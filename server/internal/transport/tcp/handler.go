package tcp

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/pescador95/memorydb/server/internal/codec"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/protocol"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

// Handler coordena o transporte TCP, a decodificação e a execução dos comandos.
type Handler struct {
	engine      *engine.Engine
	codec       codec.Codec
	idempotency *idempotency.Executor
}

// NewHandler cria um Handler com suas dependências.
func NewHandler(
	commandEngine *engine.Engine,
	messageCodec codec.Codec,
	executor *idempotency.Executor,
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

	return &Handler{
		engine:      commandEngine,
		codec:       messageCodec,
		idempotency: executor,
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
		payload, err := connection.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("read request frame: %w", err)
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

		if err := connection.WriteFrame(responsePayload); err != nil {
			return fmt.Errorf("write response frame: %w", err)
		}
	}
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
