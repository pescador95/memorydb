package tcp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pescador95/memorydb/server/internal/codec"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/protocol"
	sharedconfig "github.com/pescador95/memorydb/shared/config"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

const (
	defaultMaxInFlightRequests = 256
)

var (
	ErrMaxInFlightRequestsReached    = errors.New("maximum in-flight requests reached")
	ErrProcessingMemoryBudgetReached = errors.New("processing memory budget reached")
	ErrResponseFrameTooLarge         = errors.New("response frame exceeds configured maximum")
)

// Handler coordena o transporte TCP, a decodificação e a execução dos comandos.
type Handler struct {
	engine      *engine.Engine
	codec       codec.Codec
	idempotency *idempotency.Executor

	readTimeout  time.Duration
	writeTimeout time.Duration

	maxInFlightRequests    int
	maxFramePayloadSize    uint32
	processingMemoryBudget int64

	processingMu         sync.Mutex
	inFlightRequests     int
	processingMemoryUsed int64
}

// HandlerConfig representa os limites operacionais e de recursos das operações TCP.
type HandlerConfig struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	MaxInFlightRequests    int
	MaxFramePayloadSize    int64
	ProcessingMemoryBudget int64
}

// NewHandler cria um Handler com suas dependências.
func NewHandler(
	commandEngine *engine.Engine,
	messageCodec codec.Codec,
	executor *idempotency.Executor,
) (*Handler, error) {
	return NewHandlerWithConfig(
		commandEngine,
		messageCodec,
		executor,
		HandlerConfig{
			ReadTimeout:  sharedconfig.DefaultServerReadTimeout,
			WriteTimeout: sharedconfig.DefaultServerWriteTimeout,
		},
	)
}

// NewHandlerWithConfig cria um Handler com limites temporais e de recursos configuráveis.
func NewHandlerWithConfig(
	commandEngine *engine.Engine,
	messageCodec codec.Codec,
	executor *idempotency.Executor,
	config HandlerConfig,
) (*Handler, error) {
	if commandEngine == nil {
		return nil, fmt.Errorf("engine cannot be nil")
	}

	if messageCodec == nil {
		return nil, fmt.Errorf("codec cannot be nil")
	}

	if executor == nil {
		return nil, fmt.Errorf("idempotency executor cannot be nil")
	}

	if config.ReadTimeout <= 0 {
		return nil, fmt.Errorf("read timeout must be positive")
	}

	if config.WriteTimeout <= 0 {
		return nil, fmt.Errorf("write timeout must be positive")
	}

	if config.MaxInFlightRequests < 0 {
		return nil, fmt.Errorf("maximum in-flight requests cannot be negative")
	}

	if config.MaxFramePayloadSize < 0 {
		return nil, fmt.Errorf("maximum frame payload size cannot be negative")
	}

	if config.ProcessingMemoryBudget < 0 {
		return nil, fmt.Errorf("processing memory budget cannot be negative")
	}

	maxInFlightRequests := config.MaxInFlightRequests
	if maxInFlightRequests == 0 {
		maxInFlightRequests = defaultMaxInFlightRequests
	}

	maxFramePayloadSize := config.MaxFramePayloadSize
	if maxFramePayloadSize == 0 {
		maxFramePayloadSize = int64(transporttcp.MaxFrameSize)
	}

	if maxFramePayloadSize > int64(transporttcp.MaxFrameSize) {
		return nil, fmt.Errorf(
			"maximum frame payload size %d exceeds transport maximum %d",
			maxFramePayloadSize,
			transporttcp.MaxFrameSize,
		)
	}

	return &Handler{
		engine:                 commandEngine,
		codec:                  messageCodec,
		idempotency:            executor,
		readTimeout:            config.ReadTimeout,
		writeTimeout:           config.WriteTimeout,
		maxInFlightRequests:    maxInFlightRequests,
		maxFramePayloadSize:    uint32(maxFramePayloadSize),
		processingMemoryBudget: config.ProcessingMemoryBudget,
	}, nil
}

// Handle processa solicitações enquanto a conexão permanecer aberta.
func (h *Handler) Handle(conn net.Conn) error {
	if conn == nil {
		return fmt.Errorf("connection cannot be nil")
	}

	connection, err := transporttcp.NewConnection(conn)
	if err != nil {
		return fmt.Errorf("create connection: %w", err)
	}
	defer connection.Close()

	for {
		if err := conn.SetReadDeadline(time.Now().Add(h.readTimeout)); err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
				return nil
			}

			return fmt.Errorf("set read deadline: %w", err)
		}

		payload, release, err := connection.ReadFrameWithReservation(
			h.maxFramePayloadSize,
			h.reserveProcessing,
		)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("read request frame: %w", err)
		}

		if err := h.processFrame(conn, connection, payload, release); err != nil {
			return err
		}
	}
}

// reserveProcessing adquire uma vaga de processamento e reserva memória lógica.
// A reserva acontece antes da alocação do payload do frame.
func (h *Handler) reserveProcessing(frameSize uint32) (func(), error) {
	reservation := int64(frameSize) + 2*int64(h.maxFramePayloadSize)

	h.processingMu.Lock()

	if h.inFlightRequests >= h.maxInFlightRequests {
		h.processingMu.Unlock()
		return nil, ErrMaxInFlightRequestsReached
	}

	if h.processingMemoryBudget > 0 &&
		(reservation > h.processingMemoryBudget ||
			h.processingMemoryUsed > h.processingMemoryBudget-reservation) {
		h.processingMu.Unlock()
		return nil, ErrProcessingMemoryBudgetReached
	}

	h.inFlightRequests++
	h.processingMemoryUsed += reservation
	h.processingMu.Unlock()

	var once sync.Once

	release := func() {
		once.Do(func() {
			h.processingMu.Lock()
			h.inFlightRequests--
			h.processingMemoryUsed -= reservation
			h.processingMu.Unlock()
		})
	}

	return release, nil
}

// processFrame mantém a reserva até a resposta ser escrita ou ocorrer um erro.
func (h *Handler) processFrame(
	conn net.Conn,
	connection *transporttcp.Connection,
	payload []byte,
	release func(),
) error {
	if release != nil {
		defer release()
	}

	request, err := h.codec.DecodeRequest(payload)
	if err != nil {
		return fmt.Errorf("decode request: %w", err)
	}

	response := h.executeRequest(request)

	responsePayload, err := h.codec.EncodeResponse(response)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}

	if uint64(len(responsePayload)) > uint64(h.maxFramePayloadSize) {
		return fmt.Errorf(
			"%w: encoded size %d, configured maximum %d",
			ErrResponseFrameTooLarge,
			len(responsePayload),
			h.maxFramePayloadSize,
		)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(h.writeTimeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}

	if err := connection.WriteFrame(responsePayload); err != nil {
		return fmt.Errorf("write response frame: %w", err)
	}

	return nil
}

// executeRequest encaminha operações mutáveis pelo executor de idempotência.
func (h *Handler) executeRequest(request *protocol.Request) *protocol.Response {
	if request == nil {
		return idempotencyErrorResponse(
			0,
			"INVALID_REQUEST",
			"request cannot be nil",
		)
	}

	if request.Command == nil {
		return idempotencyErrorResponse(
			request.RequestID,
			"INVALID_REQUEST",
			"command cannot be nil",
		)
	}

	switch command := request.Command.(type) {
	case *protocol.SetCommand:
		if command == nil {
			return idempotencyErrorResponse(
				request.RequestID,
				"INVALID_REQUEST",
				"SET command cannot be nil",
			)
		}

		return h.executeIdempotent(request, command.OperationID)

	case *protocol.DeleteCommand:
		if command == nil {
			return idempotencyErrorResponse(
				request.RequestID,
				"INVALID_REQUEST",
				"DELETE command cannot be nil",
			)
		}

		return h.executeIdempotent(request, command.OperationID)

	default:
		return h.engine.Execute(request)
	}
}

// executeIdempotent calcula o fingerprint e executa ou reproduz o resultado.
func (h *Handler) executeIdempotent(
	request *protocol.Request,
	operationID []byte,
) *protocol.Response {
	fingerprint, err := idempotency.Fingerprint(request.Command)
	if err != nil {
		return idempotencyErrorResponse(
			request.RequestID,
			"INVALID_REQUEST",
			err.Error(),
		)
	}

	response, err := h.idempotency.Execute(
		operationID,
		request.RequestID,
		fingerprint,
		func() *protocol.Response {
			return h.engine.Execute(request)
		},
	)
	if err != nil {
		switch {
		case errors.Is(err, idempotency.ErrInvalidOperationID):
			return idempotencyErrorResponse(
				request.RequestID,
				"INVALID_OPERATION_ID",
				err.Error(),
			)

		case errors.Is(err, idempotency.ErrFingerprintConflict):
			return idempotencyErrorResponse(
				request.RequestID,
				"OPERATION_ID_CONFLICT",
				err.Error(),
			)

		case errors.Is(err, idempotency.ErrCapacityReached):
			return idempotencyErrorResponse(
				request.RequestID,
				"IDEMPOTENCY_CAPACITY_REACHED",
				err.Error(),
			)

		case errors.Is(err, idempotency.ErrMemoryBudgetReached):
			return idempotencyErrorResponse(
				request.RequestID,
				"IDEMPOTENCY_MEMORY_BUDGET_REACHED",
				err.Error(),
			)

		case errors.Is(err, idempotency.ErrOutcomeUnknown):
			return idempotencyErrorResponse(
				request.RequestID,
				"OPERATION_OUTCOME_UNKNOWN",
				err.Error(),
			)

		default:
			return idempotencyErrorResponse(
				request.RequestID,
				"INTERNAL_ERROR",
				err.Error(),
			)
		}
	}

	return response
}

// idempotencyErrorResponse cria uma resposta de erro do protocolo.
func idempotencyErrorResponse(
	requestID uint64,
	code string,
	message string,
) *protocol.Response {
	return &protocol.Response{
		RequestID: requestID,
		Success:   false,
		Result: &protocol.ErrorResult{
			Code:    code,
			Message: message,
		},
	}
}

// processingMemoryUsage retorna o total de memória lógica reservado.
func (h *Handler) processingMemoryUsage() int64 {
	h.processingMu.Lock()
	defer h.processingMu.Unlock()

	return h.processingMemoryUsed
}

// inFlightRequestCount retorna a quantidade de requisições em processamento.
func (h *Handler) inFlightRequestCount() int {
	h.processingMu.Lock()
	defer h.processingMu.Unlock()

	return h.inFlightRequests
}
