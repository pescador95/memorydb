package tcp

import (
	"errors"
	"net"
	"testing"

	"github.com/pescador95/memorydb/server/internal/codec"
	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/protocol"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

type handlerTestCodec struct {
	request           *protocol.Request
	decodeRequestErr  error
	encodeResponseErr error
}

func (c *handlerTestCodec) EncodeRequest(*protocol.Request) ([]byte, error) {
	return []byte{1}, nil
}

func (c *handlerTestCodec) DecodeRequest([]byte) (*protocol.Request, error) {
	return c.request, c.decodeRequestErr
}

func (c *handlerTestCodec) EncodeResponse(*protocol.Response) ([]byte, error) {
	if c.encodeResponseErr != nil {
		return nil, c.encodeResponseErr
	}

	return []byte{1}, nil
}

func (c *handlerTestCodec) DecodeResponse([]byte) (*protocol.Response, error) {
	return nil, nil
}

func newTestIdempotencyExecutor(t *testing.T) *idempotency.Executor {
	t.Helper()

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	return executor
}

func TestHandlerProcessesMultipleRequests(t *testing.T) {
	serverSocket, clientSocket := net.Pipe()

	messageCodec := protobuf.New()

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(engine.New(), messageCodec, executor)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(serverSocket)
	}()

	connection, err := transporttcp.NewConnection(clientSocket)
	if err != nil {
		t.Fatalf("NewConnection() error = %v", err)
	}

	defer connection.Close()

	requests := []*protocol.Request{
		{
			RequestID: 1,
			Command:   &protocol.PingCommand{},
		},
		{
			RequestID: 2,
			Command: &protocol.SetCommand{
				OperationID: []byte("set-operation-01"),
				Key:         "user:1",
				Value:       []byte("memorydb"),
			},
		},
		{
			RequestID: 3,
			Command: &protocol.GetCommand{
				Key: "user:1",
			},
		},
		{
			RequestID: 4,
			Command: &protocol.DeleteCommand{
				OperationID: []byte("del-operation-01"),
				Key:         "user:1",
			},
		},
	}

	for _, request := range requests {
		requestPayload, err := messageCodec.EncodeRequest(request)
		if err != nil {
			t.Fatalf("EncodeRequest() error = %v", err)
		}

		if err := connection.WriteFrame(requestPayload); err != nil {
			t.Fatalf("WriteFrame() error = %v", err)
		}

		responsePayload, err := connection.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame() error = %v", err)
		}

		response, err := messageCodec.DecodeResponse(responsePayload)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}

		if response.RequestID != request.RequestID {
			t.Errorf(
				"response request ID = %d, want %d",
				response.RequestID,
				request.RequestID,
			)
		}

		if !response.Success {
			t.Errorf(
				"request %d failed: %+v",
				request.RequestID,
				response.Result,
			)
		}

		switch request.RequestID {
		case 1:
			if _, ok := response.Result.(*protocol.PongResult); !ok {
				t.Errorf("expected PONG result, got %T", response.Result)
			}

		case 2:
			if _, ok := response.Result.(*protocol.SetResult); !ok {
				t.Errorf("expected SET result, got %T", response.Result)
			}

		case 3:
			result, ok := response.Result.(*protocol.GetResult)
			if !ok {
				t.Fatalf("expected GET result, got %T", response.Result)
			}

			if !result.Found {
				t.Error("expected GET to find the key")
			}

			if string(result.Value) != "memorydb" {
				t.Errorf("GET value = %q, want %q", result.Value, "memorydb")
			}

		case 4:
			result, ok := response.Result.(*protocol.DeleteResult)
			if !ok {
				t.Fatalf("expected DELETE result, got %T", response.Result)
			}

			if !result.Deleted {
				t.Error("expected DELETE to remove the key")
			}
		}
	}

	// O encerramento do cliente deve finalizar o Handler normalmente.
	if err := connection.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := <-handlerDone; err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
}

func TestNewHandlerRejectsNilDependencies(t *testing.T) {
	var messageCodec codec.Codec = protobuf.New()

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	if _, err := NewHandler(nil, messageCodec, executor); err == nil {
		t.Error("expected error for nil engine")
	}

	if _, err := NewHandler(engine.New(), nil, executor); err == nil {
		t.Error("expected error for nil codec")
	}

	if _, err := NewHandler(engine.New(), messageCodec, nil); err == nil {
		t.Error("expected error for nil idempotency executor")
	}
}

func TestHandlerExecutesIdempotentSet(t *testing.T) {
	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(engine.New(), protobuf.New(), executor)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	operationID := []byte("set-operation-01")

	firstRequest := &protocol.Request{
		RequestID: 1,
		Command: &protocol.SetCommand{
			OperationID: operationID,
			Key:         "user:1",
			Value:       []byte("original"),
		},
	}

	firstResponse := handler.executeRequest(firstRequest)

	if !firstResponse.Success {
		t.Fatalf("first SET failed: %+v", firstResponse.Result)
	}

	if firstResponse.RequestID != firstRequest.RequestID {
		t.Errorf(
			"first response request ID = %d, want %d",
			firstResponse.RequestID,
			firstRequest.RequestID,
		)
	}

	if _, ok := firstResponse.Result.(*protocol.SetResult); !ok {
		t.Fatalf("expected SET result, got %T", firstResponse.Result)
	}

	duplicateRequest := &protocol.Request{
		RequestID: 2,
		Command: &protocol.SetCommand{
			OperationID: operationID,
			Key:         "user:1",
			Value:       []byte("original"),
		},
	}

	duplicateResponse := handler.executeRequest(duplicateRequest)

	if !duplicateResponse.Success {
		t.Fatalf("duplicate SET failed: %+v", duplicateResponse.Result)
	}

	if duplicateResponse.RequestID != duplicateRequest.RequestID {
		t.Errorf(
			"duplicate response request ID = %d, want %d",
			duplicateResponse.RequestID,
			duplicateRequest.RequestID,
		)
	}

	if _, ok := duplicateResponse.Result.(*protocol.SetResult); !ok {
		t.Fatalf("expected replayed SET result, got %T", duplicateResponse.Result)
	}

	conflictingRequest := &protocol.Request{
		RequestID: 3,
		Command: &protocol.SetCommand{
			OperationID: operationID,
			Key:         "user:1",
			Value:       []byte("modified"),
		},
	}

	conflictingResponse := handler.executeRequest(conflictingRequest)

	if conflictingResponse.Success {
		t.Fatal("expected conflicting SET to fail")
	}

	errorResult, ok := conflictingResponse.Result.(*protocol.ErrorResult)
	if !ok {
		t.Fatalf("expected error result, got %T", conflictingResponse.Result)
	}

	if errorResult.Code != "OPERATION_ID_CONFLICT" {
		t.Errorf(
			"error code = %q, want %q",
			errorResult.Code,
			"OPERATION_ID_CONFLICT",
		)
	}

	if conflictingResponse.RequestID != conflictingRequest.RequestID {
		t.Errorf(
			"conflicting response request ID = %d, want %d",
			conflictingResponse.RequestID,
			conflictingRequest.RequestID,
		)
	}

	getResponse := handler.executeRequest(&protocol.Request{
		RequestID: 4,
		Command: &protocol.GetCommand{
			Key: "user:1",
		},
	})

	if !getResponse.Success {
		t.Fatalf("GET failed: %+v", getResponse.Result)
	}

	getResult, ok := getResponse.Result.(*protocol.GetResult)
	if !ok {
		t.Fatalf("expected GET result, got %T", getResponse.Result)
	}

	if !getResult.Found {
		t.Fatal("expected GET to find the key")
	}

	if string(getResult.Value) != "original" {
		t.Errorf("GET value = %q, want %q", getResult.Value, "original")
	}
}

func TestHandlerExecutesIdempotentDelete(t *testing.T) {
	commandEngine := engine.New()

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(commandEngine, protobuf.New(), executor)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	// Prepara a chave que será removida.
	setResponse := handler.executeRequest(&protocol.Request{
		RequestID: 1,
		Command: &protocol.SetCommand{
			OperationID: []byte("set-operation-01"),
			Key:         "user:1",
			Value:       []byte("memorydb"),
		},
	})

	if !setResponse.Success {
		t.Fatalf("SET failed: %+v", setResponse.Result)
	}

	operationID := []byte("del-operation-01")

	firstRequest := &protocol.Request{
		RequestID: 2,
		Command: &protocol.DeleteCommand{
			OperationID: operationID,
			Key:         "user:1",
		},
	}

	firstResponse := handler.executeRequest(firstRequest)

	if !firstResponse.Success {
		t.Fatalf("first DELETE failed: %+v", firstResponse.Result)
	}

	firstResult, ok := firstResponse.Result.(*protocol.DeleteResult)
	if !ok {
		t.Fatalf("expected DELETE result, got %T", firstResponse.Result)
	}

	if !firstResult.Deleted {
		t.Fatal("expected first DELETE to remove the key")
	}

	if firstResponse.RequestID != firstRequest.RequestID {
		t.Errorf(
			"first response request ID = %d, want %d",
			firstResponse.RequestID,
			firstRequest.RequestID,
		)
	}

	// Repete a mesma operação lógica com outro RequestID.
	duplicateRequest := &protocol.Request{
		RequestID: 3,
		Command: &protocol.DeleteCommand{
			OperationID: operationID,
			Key:         "user:1",
		},
	}

	duplicateResponse := handler.executeRequest(duplicateRequest)

	if !duplicateResponse.Success {
		t.Fatalf("duplicate DELETE failed: %+v", duplicateResponse.Result)
	}

	duplicateResult, ok := duplicateResponse.Result.(*protocol.DeleteResult)
	if !ok {
		t.Fatalf("expected replayed DELETE result, got %T", duplicateResponse.Result)
	}

	if !duplicateResult.Deleted {
		t.Error("expected duplicate DELETE to replay original result")
	}

	if duplicateResponse.RequestID != duplicateRequest.RequestID {
		t.Errorf(
			"duplicate response request ID = %d, want %d",
			duplicateResponse.RequestID,
			duplicateRequest.RequestID,
		)
	}

	// Confirma que a chave continua removida.
	getResponse := handler.executeRequest(&protocol.Request{
		RequestID: 4,
		Command: &protocol.GetCommand{
			Key: "user:1",
		},
	})

	if !getResponse.Success {
		t.Fatalf("GET failed: %+v", getResponse.Result)
	}

	getResult, ok := getResponse.Result.(*protocol.GetResult)
	if !ok {
		t.Fatalf("expected GET result, got %T", getResponse.Result)
	}

	if getResult.Found {
		t.Error("expected GET not to find the deleted key")
	}
}

func TestHandlerRejectsInvalidOperationID(t *testing.T) {
	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(engine.New(), protobuf.New(), executor)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	tests := []struct {
		name        string
		operationID []byte
	}{
		{
			name:        "missing operation ID",
			operationID: nil,
		},
		{
			name:        "empty operation ID",
			operationID: []byte{},
		},
		{
			name:        "operation ID shorter than 16 bytes",
			operationID: []byte("short"),
		},
		{
			name:        "operation ID longer than 16 bytes",
			operationID: []byte("operation-id-12345"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := handler.executeRequest(&protocol.Request{
				RequestID: 1,
				Command: &protocol.SetCommand{
					OperationID: tt.operationID,
					Key:         "user:invalid",
					Value:       []byte("must-not-be-stored"),
				},
			})

			if response.Success {
				t.Fatal("expected invalid OperationID to be rejected")
			}

			errorResult, ok := response.Result.(*protocol.ErrorResult)
			if !ok {
				t.Fatalf("expected error result, got %T", response.Result)
			}

			if errorResult.Code != "INVALID_OPERATION_ID" {
				t.Errorf(
					"error code = %q, want %q",
					errorResult.Code,
					"INVALID_OPERATION_ID",
				)
			}

			if response.RequestID != 1 {
				t.Errorf("response request ID = %d, want 1", response.RequestID)
			}

			// Confirma que a operação inválida não alterou o storage.
			getResponse := handler.executeRequest(&protocol.Request{
				RequestID: 2,
				Command: &protocol.GetCommand{
					Key: "user:invalid",
				},
			})

			if !getResponse.Success {
				t.Fatalf("GET failed: %+v", getResponse.Result)
			}

			getResult, ok := getResponse.Result.(*protocol.GetResult)
			if !ok {
				t.Fatalf("expected GET result, got %T", getResponse.Result)
			}

			if getResult.Found {
				t.Error("invalid SET unexpectedly stored the key")
			}
		})
	}
}

func TestHandlerRejectsOperationWhenCapacityReached(t *testing.T) {
	executor, err := idempotency.NewExecutor(&idempotency.Config{
		Capacity: 1,
	})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(engine.New(), protobuf.New(), executor)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	// A primeira operação ocupa a única posição disponível.
	firstResponse := handler.executeRequest(&protocol.Request{
		RequestID: 1,
		Command: &protocol.SetCommand{
			OperationID: []byte("set-operation-01"),
			Key:         "user:1",
			Value:       []byte("first"),
		},
	})

	if !firstResponse.Success {
		t.Fatalf("first SET failed: %+v", firstResponse.Result)
	}

	// A segunda operação deve ser rejeitada por falta de capacidade.
	secondResponse := handler.executeRequest(&protocol.Request{
		RequestID: 2,
		Command: &protocol.SetCommand{
			OperationID: []byte("set-operation-02"),
			Key:         "user:2",
			Value:       []byte("second"),
		},
	})

	if secondResponse.Success {
		t.Fatal("expected second SET to fail because capacity is reached")
	}

	errorResult, ok := secondResponse.Result.(*protocol.ErrorResult)
	if !ok {
		t.Fatalf("expected error result, got %T", secondResponse.Result)
	}

	if errorResult.Code != "IDEMPOTENCY_CAPACITY_REACHED" {
		t.Errorf(
			"error code = %q, want %q",
			errorResult.Code,
			"IDEMPOTENCY_CAPACITY_REACHED",
		)
	}

	if secondResponse.RequestID != 2 {
		t.Errorf("response request ID = %d, want 2", secondResponse.RequestID)
	}

	// A primeira operação deve continuar armazenada.
	firstGetResponse := handler.executeRequest(&protocol.Request{
		RequestID: 3,
		Command: &protocol.GetCommand{
			Key: "user:1",
		},
	})

	if !firstGetResponse.Success {
		t.Fatalf("GET first key failed: %+v", firstGetResponse.Result)
	}

	firstGetResult, ok := firstGetResponse.Result.(*protocol.GetResult)
	if !ok {
		t.Fatalf("expected GET result, got %T", firstGetResponse.Result)
	}

	if !firstGetResult.Found {
		t.Fatal("expected first key to remain stored")
	}

	if string(firstGetResult.Value) != "first" {
		t.Errorf("first key value = %q, want %q", firstGetResult.Value, "first")
	}

	// A segunda operação não pode ter alterado o storage.
	secondGetResponse := handler.executeRequest(&protocol.Request{
		RequestID: 4,
		Command: &protocol.GetCommand{
			Key: "user:2",
		},
	})

	if !secondGetResponse.Success {
		t.Fatalf("GET second key failed: %+v", secondGetResponse.Result)
	}

	secondGetResult, ok := secondGetResponse.Result.(*protocol.GetResult)
	if !ok {
		t.Fatalf("expected GET result, got %T", secondGetResponse.Result)
	}

	if secondGetResult.Found {
		t.Error("second key was stored despite capacity rejection")
	}
}

func TestHandlerRejectsNilRequest(t *testing.T) {
	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(engine.New(), protobuf.New(), executor)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	tests := []struct {
		name    string
		request *protocol.Request
	}{
		{
			name:    "nil request",
			request: nil,
		},
		{
			name: "nil command",
			request: &protocol.Request{
				RequestID: 1,
				Command:   nil,
			},
		},
		{
			name: "nil SET command",
			request: &protocol.Request{
				RequestID: 2,
				Command:   (*protocol.SetCommand)(nil),
			},
		},
		{
			name: "nil DELETE command",
			request: &protocol.Request{
				RequestID: 3,
				Command:   (*protocol.DeleteCommand)(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := handler.executeRequest(tt.request)

			if response == nil {
				t.Fatal("expected error response, got nil")
			}

			if response.Success {
				t.Fatal("expected invalid request to fail")
			}

			errorResult, ok := response.Result.(*protocol.ErrorResult)
			if !ok {
				t.Fatalf("expected error result, got %T", response.Result)
			}

			if errorResult.Code != "INVALID_REQUEST" {
				t.Errorf(
					"error code = %q, want %q",
					errorResult.Code,
					"INVALID_REQUEST",
				)
			}

			expectedRequestID := uint64(0)
			if tt.request != nil {
				expectedRequestID = tt.request.RequestID
			}

			if response.RequestID != expectedRequestID {
				t.Errorf(
					"response request ID = %d, want %d",
					response.RequestID,
					expectedRequestID,
				)
			}
		})
	}
}

func TestHandlerRejectsNilConnection(t *testing.T) {
	handler, err := NewHandler(
		engine.New(),
		protobuf.New(),
		newTestIdempotencyExecutor(t),
	)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	if err := handler.Handle(nil); err == nil {
		t.Fatal("expected error for nil connection")
	}

	var typedNilConnection *net.TCPConn

	if err := handler.Handle(typedNilConnection); err == nil {
		t.Fatal("expected error for typed nil connection")
	}
}

func TestHandlerReturnsReadFrameError(t *testing.T) {
	handler, err := NewHandler(
		engine.New(),
		protobuf.New(),
		newTestIdempotencyExecutor(t),
	)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	serverSocket, clientSocket := net.Pipe()

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(serverSocket)
	}()

	// Envia somente parte do cabeçalho e encerra a conexão.
	if _, err := clientSocket.Write([]byte{0, 0}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if err := clientSocket.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := <-handlerDone; err == nil {
		t.Fatal("expected read frame error")
	}
}

func TestHandlerReturnsDecodeRequestError(t *testing.T) {
	decodeErr := errors.New("decode failure")

	handler, err := NewHandler(
		engine.New(),
		&handlerTestCodec{
			decodeRequestErr: decodeErr,
		},
		newTestIdempotencyExecutor(t),
	)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	serverSocket, clientSocket := net.Pipe()

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(serverSocket)
	}()

	if err := transporttcp.WriteFrame(clientSocket, []byte{1}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	if err := <-handlerDone; !errors.Is(err, decodeErr) {
		t.Errorf("Handle() error = %v, want %v", err, decodeErr)
	}

	_ = clientSocket.Close()
}

func TestHandlerReturnsEncodeResponseError(t *testing.T) {
	encodeErr := errors.New("encode failure")

	handler, err := NewHandler(
		engine.New(),
		&handlerTestCodec{
			request: &protocol.Request{
				RequestID: 1,
				Command:   &protocol.PingCommand{},
			},
			encodeResponseErr: encodeErr,
		},
		newTestIdempotencyExecutor(t),
	)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	serverSocket, clientSocket := net.Pipe()

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(serverSocket)
	}()

	if err := transporttcp.WriteFrame(clientSocket, []byte{1}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	if err := <-handlerDone; !errors.Is(err, encodeErr) {
		t.Errorf("Handle() error = %v, want %v", err, encodeErr)
	}

	_ = clientSocket.Close()
}

func TestHandlerReturnsWriteFrameError(t *testing.T) {
	handler, err := NewHandler(
		engine.New(),
		&handlerTestCodec{
			request: &protocol.Request{
				RequestID: 1,
				Command:   &protocol.PingCommand{},
			},
		},
		newTestIdempotencyExecutor(t),
	)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	serverSocket, clientSocket := net.Pipe()

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(serverSocket)
	}()

	if err := transporttcp.WriteFrame(clientSocket, []byte{1}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	// O Handler já recebeu o frame. Fechar o cliente faz a escrita
	// da resposta falhar.
	if err := clientSocket.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := <-handlerDone; err == nil {
		t.Fatal("expected write frame error")
	}
}
