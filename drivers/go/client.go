package memorydb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	memorydbpb "github.com/pescador95/memorydb/protocol/generated/go/memorydb/v1"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
	"google.golang.org/protobuf/proto"
)

type clientState uint8

const (
	clientStateOpen clientState = iota
	clientStateClosed
	clientStateFailed
)

type operationPhase uint8

const (
	operationPhaseReady operationPhase = iota
	operationPhaseInFlight
	operationPhaseCompleted
	operationPhaseInterrupted
)

// Client representa um cliente conectado ao MemoryDB.
//
// Um Client deve ser criado por Dial, DialContext ou New.
// Seu valor zero não é utilizável.
//
// Os métodos podem ser chamados concorrentemente.
// As operações são serializadas por conexão.
type Client struct {
	connection *transporttcp.Connection

	// Serializa as operações e permite cancelamento durante a espera.
	operationGate chan struct{}

	// Protege o estado do cliente.
	stateMu sync.Mutex
	state   clientState

	// Garante que a conexão seja encerrada uma única vez.
	closeOnce sync.Once
	closeErr  error

	requestID uint64
}

// ServerError representa um erro retornado pelo MemoryDB.
type ServerError struct {
	Code    string
	Message string
}

// Error retorna a descrição do erro retornado pelo servidor.
func (e *ServerError) Error() string {
	return fmt.Sprintf("MemoryDB error [%s]: %s", e.Code, e.Message)
}

// OperationOutcomeUnknownError indica que uma operação pode ter sido
// executada pelo servidor, mas seu resultado não pôde ser confirmado.
type OperationOutcomeUnknownError struct {
	RequestID uint64
	Cause     error
}

// Error retorna a descrição do resultado indeterminado.
func (e *OperationOutcomeUnknownError) Error() string {
	return fmt.Sprintf(
		"MemoryDB operation outcome unknown for request %d: %v",
		e.RequestID,
		e.Cause,
	)
}

// Unwrap permite acessar a causa original do erro.
func (e *OperationOutcomeUnknownError) Unwrap() error {
	return e.Cause
}

// Dial estabelece uma conexão TCP com o MemoryDB.
//
// Utiliza um contexto sem cancelamento para estabelecer a conexão.
func Dial(address string) (*Client, error) {
	return DialContext(context.Background(), address)
}

// DialContext estabelece uma conexão TCP com o MemoryDB.
//
// O contexto permite cancelar ou limitar o tempo de estabelecimento
// da conexão. Após a conexão ser estabelecida, esse contexto não é
// associado ao cliente. Cada operação pode receber seu próprio contexto.
func DialContext(ctx context.Context, address string) (*Client, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect to MemoryDB: %w", err)
	}

	client, err := New(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create client: %w", err)
	}

	return client, nil
}

// New cria um cliente a partir de uma conexão TCP existente.
//
// O cliente assume a propriedade da conexão e a encerra quando
// Client.Close é chamado.
func New(conn net.Conn) (*Client, error) {
	connection, err := transporttcp.NewConnection(conn)
	if err != nil {
		return nil, fmt.Errorf("create TCP connection: %w", err)
	}

	return &Client{
		connection:    connection,
		operationGate: make(chan struct{}, 1),
	}, nil
}

// acquireOperation aguarda a disponibilidade da conexão ou o cancelamento.
func (c *Client) acquireOperation(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context cannot be nil")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case c.operationGate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			c.releaseOperation()
			return err
		}

		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseOperation libera a conexão para a próxima operação.
func (c *Client) releaseOperation() {
	<-c.operationGate
}

// Close encerra a conexão com o MemoryDB.
//
// Pode ser chamado concorrentemente com outras operações.
// O fechamento interrompe operações de rede que estejam bloqueadas.
func (c *Client) Close() error {
	c.stateMu.Lock()

	if c.state == clientStateOpen {
		c.state = clientStateClosed
	}

	c.stateMu.Unlock()

	return c.closeConnection()
}

// closeConnection encerra a conexão uma única vez.
func (c *Client) closeConnection() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.connection.Close()
	})

	return c.closeErr
}

// fail invalida o cliente após uma falha de comunicação ou protocolo.
func (c *Client) fail() {
	c.stateMu.Lock()

	if c.state == clientStateOpen {
		c.state = clientStateFailed
	}

	c.stateMu.Unlock()

	_ = c.closeConnection()
}

// checkState verifica se o cliente ainda pode executar operações.
func (c *Client) checkState() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	switch c.state {
	case clientStateOpen:
		return nil
	case clientStateClosed:
		return fmt.Errorf("MemoryDB client is closed")
	case clientStateFailed:
		return fmt.Errorf("MemoryDB client is in a failed state")
	default:
		return fmt.Errorf("MemoryDB client has an invalid state")
	}
}

// Ping verifica a disponibilidade do MemoryDB.
func (c *Client) Ping() error {
	return c.PingContext(context.Background())
}

// PingContext verifica a disponibilidade do MemoryDB.
func (c *Client) PingContext(ctx context.Context) error {
	_, err := c.execute(ctx, &memorydbpb.Request{
		Command: &memorydbpb.Request_Ping{
			Ping: &memorydbpb.PingCommand{},
		},
	})

	return err
}

// Set grava um valor associado a uma chave usando o identificador
// da operação lógica.
func (c *Client) Set(
	operationID OperationID,
	key string,
	value []byte,
) error {
	return c.SetContext(
		context.Background(),
		operationID,
		key,
		value,
	)
}

// SetContext grava um valor associado a uma chave usando o identificador
// da operação lógica.
func (c *Client) SetContext(
	ctx context.Context,
	operationID OperationID,
	key string,
	value []byte,
) error {
	_, err := c.execute(ctx, &memorydbpb.Request{
		Command: &memorydbpb.Request_Set{
			Set: &memorydbpb.SetCommand{
				OperationId: append([]byte(nil), operationID[:]...),
				Key:         key,
				Value:       value,
			},
		},
	})

	return err
}

// Get recupera o valor associado a uma chave.
func (c *Client) Get(key string) ([]byte, bool, error) {
	return c.GetContext(context.Background(), key)
}

// GetContext recupera o valor associado a uma chave.
func (c *Client) GetContext(
	ctx context.Context,
	key string,
) ([]byte, bool, error) {
	response, err := c.execute(ctx, &memorydbpb.Request{
		Command: &memorydbpb.Request_Get{
			Get: &memorydbpb.GetCommand{
				Key: key,
			},
		},
	})
	if err != nil {
		return nil, false, err
	}

	result := response.GetGet()
	value := append([]byte(nil), result.GetValue()...)

	return value, result.GetFound(), nil
}

// Delete remove uma chave do armazenamento usando o identificador
// da operação lógica.
func (c *Client) Delete(
	operationID OperationID,
	key string,
) (bool, error) {
	return c.DeleteContext(
		context.Background(),
		operationID,
		key,
	)
}

// DeleteContext remove uma chave do armazenamento usando o identificador
// da operação lógica.
func (c *Client) DeleteContext(
	ctx context.Context,
	operationID OperationID,
	key string,
) (bool, error) {
	response, err := c.execute(ctx, &memorydbpb.Request{
		Command: &memorydbpb.Request_Delete{
			Delete: &memorydbpb.DeleteCommand{
				OperationId: append([]byte(nil), operationID[:]...),
				Key:         key,
			},
		},
	})
	if err != nil {
		return false, err
	}

	return response.GetDelete().GetDeleted(), nil
}

// execute envia uma solicitação e aguarda sua resposta correspondente.

func (c *Client) execute(
	ctx context.Context,
	request *memorydbpb.Request,
) (*memorydbpb.Response, error) {
	if err := c.acquireOperation(ctx); err != nil {
		return nil, err
	}
	defer c.releaseOperation()

	if err := c.checkState(); err != nil {
		return nil, err
	}

	if request == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	switch request.GetCommand().(type) {
	case *memorydbpb.Request_Ping,
		*memorydbpb.Request_Set,
		*memorydbpb.Request_Get,
		*memorydbpb.Request_Delete:
	default:
		return nil, fmt.Errorf("request command is missing or unsupported")
	}

	c.requestID++
	if c.requestID == 0 {
		c.requestID++
	}

	request.RequestId = c.requestID

	payload, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf(
			"encode request %d: %w",
			request.RequestId,
			err,
		)
	}

	var operationMu sync.Mutex
	phase := operationPhaseReady

	callbackDone := make(chan struct{})

	stopCancellation := context.AfterFunc(ctx, func() {
		operationMu.Lock()

		shouldFail := phase == operationPhaseInFlight

		if phase == operationPhaseReady ||
			phase == operationPhaseInFlight {
			phase = operationPhaseInterrupted
		}

		operationMu.Unlock()

		if shouldFail {
			c.fail()
		}

		close(callbackDone)
	})

	// Decide atomicamente se a operação pode iniciar a comunicação.
	operationMu.Lock()

	if err := ctx.Err(); err != nil {
		phase = operationPhaseInterrupted
		operationMu.Unlock()

		if !stopCancellation() {
			<-callbackDone
		}

		return nil, err
	}

	if phase != operationPhaseReady {
		operationMu.Unlock()

		if !stopCancellation() {
			<-callbackDone
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		return nil, fmt.Errorf(
			"operation interrupted before communication",
		)
	}

	phase = operationPhaseInFlight
	operationMu.Unlock()

	// Executa a comunicação com o servidor.
	responsePayload, communicationErr := func() ([]byte, error) {
		if err := c.connection.WriteFrame(payload); err != nil {
			return nil, fmt.Errorf(
				"send request %d: %w",
				request.RequestId,
				err,
			)
		}

		response, err := c.connection.ReadFrame()
		if err != nil {
			return nil, fmt.Errorf(
				"read response %d: %w",
				request.RequestId,
				err,
			)
		}

		return response, nil
	}()

	// Registra a conclusão ou interrupção da operação.
	operationMu.Lock()

	if phase == operationPhaseInFlight {
		if communicationErr == nil {
			phase = operationPhaseCompleted
		} else {
			phase = operationPhaseInterrupted
		}
	}

	operationMu.Unlock()

	// Impede que o callback continue executando após a liberação
	// do operationGate.
	if !stopCancellation() {
		<-callbackDone
	}

	operationMu.Lock()
	completed := phase == operationPhaseCompleted
	operationMu.Unlock()

	// A comunicação foi interrompida antes da confirmação da resposta.
	if !completed {
		c.fail()

		cause := communicationErr

		if ctxErr := ctx.Err(); ctxErr != nil {
			if cause != nil {
				cause = errors.Join(ctxErr, cause)
			} else {
				cause = ctxErr
			}
		}

		if cause == nil {
			cause = errors.New(
				"operation interrupted before response confirmation",
			)
		}

		return nil, &OperationOutcomeUnknownError{
			RequestID: request.RequestId,
			Cause:     cause,
		}
	}

	// A partir daqui, o frame de resposta foi recebido integralmente.
	response := &memorydbpb.Response{}

	if err := proto.Unmarshal(responsePayload, response); err != nil {
		c.fail()

		return nil, &OperationOutcomeUnknownError{
			RequestID: request.RequestId,
			Cause: fmt.Errorf(
				"decode response: %w",
				err,
			),
		}
	}

	if response.GetRequestId() != request.GetRequestId() {
		c.fail()

		return nil, &OperationOutcomeUnknownError{
			RequestID: request.RequestId,
			Cause: fmt.Errorf(
				"request ID mismatch: got %d, want %d",
				response.GetRequestId(),
				request.GetRequestId(),
			),
		}
	}

	if !response.GetSuccess() {
		serverError := response.GetError()

		if serverError == nil {
			c.fail()

			return nil, &OperationOutcomeUnknownError{
				RequestID: request.RequestId,
				Cause:     errors.New("missing error details"),
			}
		}

		return nil, &ServerError{
			Code:    serverError.GetCode(),
			Message: serverError.GetMessage(),
		}
	}

	// Verifica se a resposta corresponde ao comando solicitado.
	validResponse := false

	switch request.GetCommand().(type) {
	case *memorydbpb.Request_Ping:
		validResponse = response.GetPong() != nil

	case *memorydbpb.Request_Set:
		validResponse = response.GetSet() != nil

	case *memorydbpb.Request_Get:
		validResponse = response.GetGet() != nil

	case *memorydbpb.Request_Delete:
		validResponse = response.GetDelete() != nil
	}

	if !validResponse {
		c.fail()

		return nil, &OperationOutcomeUnknownError{
			RequestID: request.RequestId,
			Cause: errors.New(
				"unexpected response type",
			),
		}
	}

	return response, nil
}
