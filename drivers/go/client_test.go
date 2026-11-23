package memorydb

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	memorydbpb "github.com/pescador95/memorydb/protocol/generated/go/memorydb/v1"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
	"google.golang.org/protobuf/proto"
)

type cancelOnSecondErrContext struct {
	context.Context
	errCalls int
}

func (c *cancelOnSecondErrContext) Err() error {
	c.errCalls++

	if c.errCalls == 2 {
		return context.Canceled
	}

	return nil
}

type signalingContext struct {
	context.Context
	checked    chan struct{}
	doneCalled chan struct{}
	once       sync.Once
}

func newSignalingContext(ctx context.Context) *signalingContext {
	return &signalingContext{
		Context:    ctx,
		checked:    make(chan struct{}),
		doneCalled: make(chan struct{}),
	}
}

func (c *signalingContext) Err() error {
	c.once.Do(func() {
		close(c.checked)
	})

	return c.Context.Err()
}

func (c *signalingContext) Done() <-chan struct{} {
	select {
	case <-c.doneCalled:
	default:
		close(c.doneCalled)
	}

	return c.Context.Done()
}

type cancelOnThirdErrContext struct {
	context.Context
	mu       sync.Mutex
	errCalls int
	canceled bool
	done     chan struct{}
}

func newCancelOnThirdErrContext() *cancelOnThirdErrContext {
	return &cancelOnThirdErrContext{
		Context: context.Background(),
		done:    make(chan struct{}),
	}
}

func (c *cancelOnThirdErrContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.errCalls++

	if c.errCalls >= 3 && !c.canceled {
		c.canceled = true
		close(c.done)
	}

	if c.canceled {
		return context.Canceled
	}

	return nil
}

func (c *cancelOnThirdErrContext) Done() <-chan struct{} {
	return c.done
}

type cancelAndWaitForCloseConn struct {
	net.Conn
	remaining int
	cancel    context.CancelFunc
	closed    chan struct{}
	closeOnce sync.Once
}

func (c *cancelAndWaitForCloseConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)

	if n > 0 {
		c.remaining -= n

		if c.remaining <= 0 {
			c.cancel()
			<-c.closed
		}
	}

	return n, err
}

func (c *cancelAndWaitForCloseConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() {
		close(c.closed)
	})
	return err
}

type blockingWriteConn struct {
	net.Conn

	writeStarted chan struct{}
	closed       chan struct{}

	writeOnce sync.Once
	closeOnce sync.Once
}

func newBlockingWriteConn(conn net.Conn) *blockingWriteConn {
	return &blockingWriteConn{
		Conn:         conn,
		writeStarted: make(chan struct{}),
		closed:       make(chan struct{}),
	}
}

func (c *blockingWriteConn) Write(p []byte) (int, error) {
	c.writeOnce.Do(func() {
		close(c.writeStarted)
	})

	// Simula uma escrita bloqueada até que a conexão seja encerrada.
	<-c.closed

	return 0, net.ErrClosed
}

func (c *blockingWriteConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
	})

	return c.Conn.Close()
}

type callbackStartingContext struct {
	context.Context

	mu       sync.Mutex
	done     chan struct{}
	callback func()
	canceled bool
	started  chan struct{}
}

func newCallbackStartingContext() *callbackStartingContext {
	return &callbackStartingContext{
		Context: context.Background(),
		done:    make(chan struct{}),
		started: make(chan struct{}),
	}
}

func (c *callbackStartingContext) Done() <-chan struct{} {
	return c.done
}

func (c *callbackStartingContext) Err() error {
	c.mu.Lock()

	if c.canceled {
		c.mu.Unlock()
		return context.Canceled
	}

	if c.callback == nil {
		c.mu.Unlock()
		return nil
	}

	c.canceled = true
	callback := c.callback
	close(c.done)

	c.mu.Unlock()

	go func() {
		close(c.started)
		callback()
	}()

	<-c.started

	return context.Canceled
}

func (c *callbackStartingContext) AfterFunc(f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.callback = f

	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.canceled {
			return false
		}

		c.callback = nil
		return true
	}
}

type partialWriteFailureConn struct {
	net.Conn

	writeCalls int
	writeErr   error
}

func (c *partialWriteFailureConn) Write(p []byte) (int, error) {
	c.writeCalls++

	if c.writeCalls == 1 {
		n := len(p) / 2
		if n == 0 {
			n = 1
		}

		return c.Conn.Write(p[:n])
	}

	return 0, c.writeErr
}

// startTestServer inicia um servidor simulado para testar o driver.
func startTestServer(t *testing.T, conn net.Conn) {
	t.Helper()

	connection, err := transporttcp.NewConnection(conn)
	if err != nil {
		t.Fatalf("criar conexão de teste: %v", err)
	}

	go func() {
		defer connection.Close()

		data := make(map[string][]byte)

		for {
			payload, err := connection.ReadFrame()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Errorf("ler solicitação: %v", err)
				}
				return
			}

			request := &memorydbpb.Request{}
			if err := proto.Unmarshal(payload, request); err != nil {
				t.Errorf("decodificar solicitação: %v", err)
				return
			}

			response := &memorydbpb.Response{
				RequestId: request.GetRequestId(),
				Success:   true,
			}

			switch command := request.GetCommand().(type) {
			case *memorydbpb.Request_Ping:
				response.Result = &memorydbpb.Response_Pong{
					Pong: &memorydbpb.PongResult{},
				}

			case *memorydbpb.Request_Set:
				data[command.Set.GetKey()] = append([]byte(nil), command.Set.GetValue()...)
				response.Result = &memorydbpb.Response_Set{
					Set: &memorydbpb.SetResult{},
				}

			case *memorydbpb.Request_Get:
				key := command.Get.GetKey()

				if key == "server-error" {
					response.Success = false
					response.Result = &memorydbpb.Response_Error{
						Error: &memorydbpb.ErrorResult{
							Code:    "TEST_ERROR",
							Message: "erro simulado pelo servidor",
						},
					}
					break
				}

				value, found := data[key]
				response.Result = &memorydbpb.Response_Get{
					Get: &memorydbpb.GetResult{
						Found: found,
						Value: append([]byte(nil), value...),
					},
				}

			case *memorydbpb.Request_Delete:
				key := command.Delete.GetKey()
				_, found := data[key]
				delete(data, key)

				response.Result = &memorydbpb.Response_Delete{
					Delete: &memorydbpb.DeleteResult{
						Deleted: found,
					},
				}

			default:
				response.Success = false
				response.Result = &memorydbpb.Response_Error{
					Error: &memorydbpb.ErrorResult{
						Code:    "UNKNOWN_COMMAND",
						Message: "comando desconhecido",
					},
				}
			}

			responsePayload, err := proto.Marshal(response)
			if err != nil {
				t.Errorf("codificar resposta: %v", err)
				return
			}

			if err := connection.WriteFrame(responsePayload); err != nil {
				if !errors.Is(err, io.EOF) {
					t.Errorf("escrever resposta: %v", err)
				}
				return
			}
		}
	}()
}

func newTestClient(t *testing.T) *Client {
	t.Helper()

	clientConn, serverConn := net.Pipe()

	startTestServer(t, serverConn)

	client, err := New(clientConn)
	if err != nil {
		clientConn.Close()
		serverConn.Close()
		t.Fatalf("criar cliente: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
	})

	return client
}

func TestClientPing(t *testing.T) {
	client := newTestClient(t)

	if err := client.Ping(); err != nil {
		t.Fatalf("PING retornou erro: %v", err)
	}
}

func TestClientSetGetDelete(t *testing.T) {
	client := newTestClient(t)

	key := "user:1"
	value := []byte("memorydb")

	if err := client.Set(newTestOperationID(t), key, value); err != nil {
		t.Fatalf("SET retornou erro: %v", err)
	}

	got, found, err := client.Get(key)
	if err != nil {
		t.Fatalf("GET retornou erro: %v", err)
	}

	if !found {
		t.Fatal("GET deveria encontrar a chave")
	}

	if !reflect.DeepEqual(got, value) {
		t.Fatalf("valor inesperado: obtido %q, esperado %q", got, value)
	}

	deleted, err := client.Delete(newTestOperationID(t), key)
	if err != nil {
		t.Fatalf("DELETE retornou erro: %v", err)
	}

	if !deleted {
		t.Fatal("DELETE deveria indicar que removeu a chave")
	}

	got, found, err = client.Get(key)
	if err != nil {
		t.Fatalf("GET após DELETE retornou erro: %v", err)
	}

	if found {
		t.Fatal("GET não deveria encontrar a chave removida")
	}

	if got != nil {
		t.Fatalf("valor deveria ser nil após chave ausente, obtido %v", got)
	}

	deleted, err = client.Delete(newTestOperationID(t), key)
	if err != nil {
		t.Fatalf("segundo DELETE retornou erro: %v", err)
	}

	if deleted {
		t.Fatal("segundo DELETE não deveria indicar remoção")
	}
}

func TestClientEmptyValue(t *testing.T) {
	client := newTestClient(t)

	if err := client.Set(newTestOperationID(t), "empty", []byte{}); err != nil {
		t.Fatalf("SET retornou erro: %v", err)
	}

	value, found, err := client.Get("empty")
	if err != nil {
		t.Fatalf("GET retornou erro: %v", err)
	}

	if !found {
		t.Fatal("chave com valor vazio deveria ser encontrada")
	}

	if len(value) != 0 {
		t.Fatalf("valor deveria estar vazio, obtido %v", value)
	}
}

func TestClientServerError(t *testing.T) {
	client := newTestClient(t)

	_, _, err := client.Get("server-error")
	if err == nil {
		t.Fatal("esperava erro retornado pelo servidor")
	}

	var serverError *ServerError
	if !errors.As(err, &serverError) {
		t.Fatalf("esperava *ServerError, obtido %T: %v", err, err)
	}

	if serverError.Code != "TEST_ERROR" {
		t.Fatalf("código inesperado: %s", serverError.Code)
	}

	if serverError.Message != "erro simulado pelo servidor" {
		t.Fatalf("mensagem inesperada: %s", serverError.Message)
	}

	if err := client.Ping(); err != nil {
		t.Fatalf("cliente deveria continuar utilizável após ServerError: %v", err)
	}
}

func TestClientConcurrentRequests(t *testing.T) {
	client := newTestClient(t)

	const total = 20

	var wg sync.WaitGroup
	errorsFound := make(chan error, total)

	operationIDs := make([]OperationID, total)

	for i := range operationIDs {
		operationID, err := NewOperationID()
		if err != nil {
			t.Fatalf("gerar OperationID: %v", err)
		}

		operationIDs[i] = operationID
	}

	for i := 0; i < total; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			key := string(rune('A' + i))
			value := []byte(key)

			if err := client.Set(operationIDs[i], key, value); err != nil {
				errorsFound <- err
				return
			}

			got, found, err := client.Get(key)
			if err != nil {
				errorsFound <- err
				return
			}

			if !found {
				errorsFound <- errors.New("chave não encontrada")
				return
			}

			if !reflect.DeepEqual(got, value) {
				errorsFound <- errors.New("valor inesperado")
			}
		}(i)
	}

	wg.Wait()
	close(errorsFound)

	for err := range errorsFound {
		t.Error(err)
	}
}

func TestClientRejectsMismatchedRequestID(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}

		defer connection.Close()

		_, err = connection.ReadFrame()
		if err != nil {
			return
		}

		response := &memorydbpb.Response{
			RequestId: 999,
			Success:   true,
			Result: &memorydbpb.Response_Pong{
				Pong: &memorydbpb.PongResult{},
			},
		}

		payload, err := proto.Marshal(response)
		if err != nil {
			return
		}

		_ = connection.WriteFrame(payload)
	}()

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro de request_id divergente")
	}

	if !strings.Contains(err.Error(), "request ID mismatch") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestClientRejectsInvalidProtobufResponse(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}

		defer connection.Close()

		_, err = connection.ReadFrame()
		if err != nil {
			return
		}

		_ = connection.WriteFrame([]byte{0xFF, 0xFF, 0xFF})
	}()

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro ao decodificar resposta inválida")
	}

	if !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestClientRejectsUnexpectedResponseType(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}

		defer connection.Close()

		_, err = connection.ReadFrame()
		if err != nil {
			return
		}

		response := &memorydbpb.Response{
			RequestId: 1,
			Success:   true,
			Result: &memorydbpb.Response_Get{
				Get: &memorydbpb.GetResult{
					Found: true,
					Value: []byte("unexpected"),
				},
			},
		}

		payload, err := proto.Marshal(response)
		if err != nil {
			return
		}

		_ = connection.WriteFrame(payload)
	}()

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro para tipo de resposta inesperado")
	}

	if !strings.Contains(err.Error(), "unexpected response") {
		t.Fatalf("erro inesperado: %v", err)
	}

	if err := client.checkState(); err == nil {
		t.Fatal("esperava cliente em estado inválido")
	}
}

func TestClientRejectsErrorResponseWithoutDetails(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}

		defer connection.Close()

		_, err = connection.ReadFrame()
		if err != nil {
			return
		}

		response := &memorydbpb.Response{
			RequestId: 1,
			Success:   false,
		}

		payload, err := proto.Marshal(response)
		if err != nil {
			return
		}

		_ = connection.WriteFrame(payload)
	}()

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro para resposta sem detalhes")
	}

	if !strings.Contains(err.Error(), "missing error details") {
		t.Fatalf("erro inesperado: %v", err)
	}

	if err := client.checkState(); err == nil {
		t.Fatal("esperava cliente em estado inválido")
	}
}

func TestClientPingAfterClose(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("fechar cliente: %v", err)
	}

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro ao executar Ping após fechar o cliente")
	}
}

func TestClientReadFailureAfterServerDisconnect(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}

		defer connection.Close()

		// Aguarda o recebimento completo da requisição.
		_, err = connection.ReadFrame()
		if err != nil {
			return
		}

		// Encerra a conexão sem enviar uma resposta.
	}()

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro após desconexão do servidor")
	}

	if !strings.Contains(err.Error(), "read response") {
		t.Fatalf("esperava falha de leitura, obtido: %v", err)
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf("esperava cliente no estado failed, obtido: %v", err)
	}
}

func TestClientWriteFailureAfterServerDisconnect(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	if err := serverConn.Close(); err != nil {
		t.Fatalf("desconectar servidor: %v", err)
	}

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro ao escrever em conexão encerrada")
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf("esperava cliente no estado failed, obtido: %v", err)
	}
}

func TestClientCloseInterruptsBlockedOperation(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	releaseServer := make(chan struct{})
	defer close(releaseServer)
	defer serverConn.Close()

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("erro ao criar cliente: %v", err)
	}
	defer client.Close()

	requestReceived := make(chan struct{})
	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}
		defer connection.Close()

		if _, err := connection.ReadFrame(); err != nil {
			return
		}

		close(requestReceived)

		// Mantém a conexão aberta sem responder à requisição.
		<-releaseServer
	}()

	operationDone := make(chan error, 1)
	go func() {
		operationDone <- client.Ping()
	}()

	select {
	case <-requestReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("servidor não recebeu a requisição dentro do prazo")
	}

	// O cliente está aguardando uma resposta que não será enviada.
	if err := client.Close(); err != nil {
		t.Fatalf("erro ao fechar cliente: %v", err)
	}

	select {
	case err := <-operationDone:
		if err == nil {
			t.Fatal("esperava erro após fechar cliente durante operação")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close não interrompeu a operação bloqueada")
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "closed") {
		t.Fatalf("esperava cliente fechado, obtido: %v", err)
	}
}

func TestClientDialContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, err := DialContext(ctx, "127.0.0.1:6380")
	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Fatal("esperava erro para contexto cancelado")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, obtido: %v", err)
	}
}

func TestClientDialContextUnavailableAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("criar listener: %v", err)
	}

	address := listener.Addr().String()

	if err := listener.Close(); err != nil {
		t.Fatalf("fechar listener: %v", err)
	}

	client, err := DialContext(context.Background(), address)
	if err == nil {
		client.Close()
		t.Fatal("esperava erro ao conectar em endereço indisponível")
	}
}

func TestClientNewWithNilConnection(t *testing.T) {
	client, err := New(nil)

	if err == nil {
		if client != nil {
			client.Close()
		}
		t.Fatal("esperava erro ao criar cliente com conexão nil")
	}

	if client != nil {
		t.Fatal("cliente deveria ser nil")
	}
}

func TestClientRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name    string
		request *memorydbpb.Request
		wantErr string
	}{
		{
			name:    "requisição nula",
			request: nil,
			wantErr: "request cannot be nil",
		},
		{
			name:    "comando ausente",
			request: &memorydbpb.Request{},
			wantErr: "request command is missing or unsupported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			defer serverConn.Close()

			client, err := New(clientConn)
			if err != nil {
				t.Fatalf("criar cliente: %v", err)
			}
			defer client.Close()

			_, err = client.execute(context.Background(), tt.request)
			if err == nil {
				t.Fatal("esperava erro para requisição inválida")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("erro inesperado: %v", err)
			}

			if err := client.checkState(); err != nil {
				t.Fatalf("cliente não deveria ser invalidado: %v", err)
			}
		})
	}
}

func TestClientContextMethods(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	if err := client.PingContext(ctx); err != nil {
		t.Fatalf("PING com contexto retornou erro: %v", err)
	}

	if err := client.SetContext(ctx, newTestOperationID(t), "user:context", []byte("memorydb")); err != nil {
		t.Fatalf("SET com contexto retornou erro: %v", err)
	}

	value, found, err := client.GetContext(ctx, "user:context")
	if err != nil {
		t.Fatalf("GET com contexto retornou erro: %v", err)
	}

	if !found {
		t.Fatal("GET deveria encontrar a chave")
	}

	if string(value) != "memorydb" {
		t.Fatalf("valor inesperado: %q", value)
	}

	deleted, err := client.DeleteContext(ctx, newTestOperationID(t), "user:context")
	if err != nil {
		t.Fatalf("DELETE com contexto retornou erro: %v", err)
	}

	if !deleted {
		t.Fatal("DELETE deveria indicar que removeu a chave")
	}
}

func TestClientOperationWithCanceledContext(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.PingContext(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, obtido: %v", err)
	}

	if err := client.checkState(); err != nil {
		t.Fatalf("cliente não deveria ser invalidado: %v", err)
	}

	if err := client.Ping(); err != nil {
		t.Fatalf("cliente deveria continuar utilizável: %v", err)
	}
}

func TestClientOperationCanceledWhileWaiting(t *testing.T) {
	client := newTestClient(t)

	// Ocupa o semáforo para impedir outra operação de começar.
	if err := client.acquireOperation(context.Background()); err != nil {
		t.Fatalf("adquirir semáforo: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalingCtx := newSignalingContext(ctx)

	operationDone := make(chan error, 1)

	go func() {
		operationDone <- client.PingContext(signalingCtx)
	}()

	// Garante que acquireOperation já verificou o contexto
	// e está prestes a aguardar o semáforo.
	select {
	case <-signalingCtx.doneCalled:
	case <-time.After(2 * time.Second):
		client.releaseOperation()
		t.Fatal("operação não começou a aguardar o semáforo")
	}

	cancel()

	select {
	case err := <-operationDone:
		if !errors.Is(err, context.Canceled) {
			client.releaseOperation()
			t.Fatalf("esperava context.Canceled, obtido: %v", err)
		}

	case <-time.After(2 * time.Second):
		client.releaseOperation()
		t.Fatal("operação não foi cancelada enquanto aguardava o semáforo")
	}

	client.releaseOperation()

	if err := client.checkState(); err != nil {
		t.Fatalf("cliente não deveria ser invalidado: %v", err)
	}

	if err := client.Ping(); err != nil {
		t.Fatalf("cliente deveria continuar utilizável: %v", err)
	}
}

func TestClientContextCancellationInterruptsBlockedOperation(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	defer serverConn.Close()

	releaseServer := make(chan struct{})
	defer close(releaseServer)

	requestReceived := make(chan struct{})

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}
		defer connection.Close()

		if _, err := connection.ReadFrame(); err != nil {
			return
		}

		close(requestReceived)

		// Mantém a conexão aberta sem enviar resposta.
		<-releaseServer
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	operationDone := make(chan error, 1)

	go func() {
		operationDone <- client.PingContext(ctx)
	}()

	select {
	case <-requestReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("servidor não recebeu a requisição dentro do prazo")
	}

	// Cancela enquanto o cliente aguarda a resposta.
	cancel()

	select {
	case err := <-operationDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("esperava context.Canceled, obtido: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("cancelamento não interrompeu a operação bloqueada")
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf("esperava cliente no estado failed, obtido: %v", err)
	}

	if err := client.Ping(); err == nil {
		t.Fatal("cliente não deveria aceitar operações após o cancelamento")
	}
}

func TestClientContextDeadlineInterruptsBlockedOperation(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	defer serverConn.Close()

	releaseServer := make(chan struct{})
	defer close(releaseServer)

	requestReceived := make(chan struct{})

	client, err := New(clientConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			return
		}
		defer connection.Close()

		if _, err := connection.ReadFrame(); err != nil {
			return
		}

		close(requestReceived)

		// Mantém a conexão aberta sem enviar resposta.
		<-releaseServer
	}()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		500*time.Millisecond,
	)
	defer cancel()

	operationDone := make(chan error, 1)

	go func() {
		operationDone <- client.PingContext(ctx)
	}()

	select {
	case <-requestReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("servidor não recebeu a requisição dentro do prazo")
	}

	select {
	case err := <-operationDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("esperava context.DeadlineExceeded, obtido: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("deadline não interrompeu a operação bloqueada")
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf("esperava cliente no estado failed, obtido: %v", err)
	}
}

func TestClientDialContextRejectsNilContext(t *testing.T) {
	_, err := DialContext(nil, "127.0.0.1:6380")

	if err == nil {
		t.Fatal("DialContext(nil) expected error, got nil")
	}
}

func TestClientContextCancellationInterruptsBlockedWrite(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	blockingConn := newBlockingWriteConn(clientConn)

	client, err := New(blockingConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	operationDone := make(chan error, 1)

	go func() {
		operationDone <- client.PingContext(ctx)
	}()

	// Aguarda a operação chegar à escrita da requisição.
	select {
	case <-blockingConn.writeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("cliente não iniciou a escrita dentro do prazo")
	}

	// A escrita está bloqueada. O cancelamento deve fechar a conexão.
	cancel()

	select {
	case err := <-operationDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("esperava context.Canceled, obtido: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("cancelamento não interrompeu a escrita bloqueada")
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf("esperava cliente no estado failed, obtido: %v", err)
	}

	if err := client.Ping(); err == nil {
		t.Fatal("cliente não deveria aceitar operações após o cancelamento")
	}
}

func TestClientCancellationDuringFinalResponseRead(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	response := &memorydbpb.Response{
		RequestId: 1,
		Success:   true,
		Result: &memorydbpb.Response_Pong{
			Pong: &memorydbpb.PongResult{},
		},
	}

	responsePayload, err := proto.Marshal(response)
	if err != nil {
		t.Fatalf("codificar resposta: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrappedConn := &cancelAndWaitForCloseConn{
		Conn:      clientConn,
		remaining: 4 + len(responsePayload),
		cancel:    cancel,
		closed:    make(chan struct{}),
	}

	client, err := New(wrappedConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	serverConnection, err := transporttcp.NewConnection(serverConn)
	if err != nil {
		t.Fatalf("criar conexão do servidor: %v", err)
	}
	defer serverConnection.Close()

	go func() {
		defer serverConnection.Close()

		if _, err := serverConnection.ReadFrame(); err != nil {
			return
		}

		if err := serverConnection.WriteFrame(responsePayload); err != nil {
			t.Errorf("enviar resposta: %v", err)
		}
	}()
	err = client.PingContext(ctx)

	stateErr := client.checkState()

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"esperava OperationOutcomeUnknownError com context.Canceled, obtido: %T: %v",
			err,
			err,
		)
	}

	var outcomeErr *OperationOutcomeUnknownError
	if !errors.As(err, &outcomeErr) {
		t.Fatalf(
			"esperava OperationOutcomeUnknownError, obtido: %T: %v",
			err,
			err,
		)
	}

	if outcomeErr.RequestID != 1 {
		t.Fatalf(
			"request ID inesperado: obtido %d, esperado 1",
			outcomeErr.RequestID,
		)
	}

	if stateErr == nil || !strings.Contains(stateErr.Error(), "failed") {
		t.Fatalf(
			"cliente deveria estar em estado failed, obtido: %v",
			stateErr,
		)
	}
}

func TestClientPartialWriteReturnsUnknownOutcome(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	writeErr := errors.New("simulated partial write failure")

	wrappedConn := &partialWriteFailureConn{
		Conn:     clientConn,
		writeErr: writeErr,
	}

	client, err := New(wrappedConn)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	receivedPartialFrame := make(chan []byte, 1)

	go func() {
		header := make([]byte, 2)

		if _, err := io.ReadFull(serverConn, header); err != nil {
			return
		}

		receivedPartialFrame <- header
	}()

	err = client.Ping()
	if err == nil {
		t.Fatal("esperava erro durante escrita parcial")
	}

	var outcomeErr *OperationOutcomeUnknownError
	if !errors.As(err, &outcomeErr) {
		t.Fatalf(
			"esperava OperationOutcomeUnknownError, obtido %T: %v",
			err,
			err,
		)
	}

	if outcomeErr.RequestID != 1 {
		t.Fatalf(
			"request ID inesperado: obtido %d, esperado 1",
			outcomeErr.RequestID,
		)
	}

	if !errors.Is(err, writeErr) {
		t.Fatalf("erro original de escrita não foi preservado: %v", err)
	}

	select {
	case header := <-receivedPartialFrame:
		if len(header) != 2 {
			t.Fatalf("esperava 2 bytes transmitidos, obtido %d", len(header))
		}

	case <-time.After(2 * time.Second):
		t.Fatal("servidor não recebeu os bytes parciais do frame")
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf("esperava cliente em estado failed, obtido: %v", err)
	}

	if err := client.Ping(); err == nil {
		t.Fatal("cliente não deveria aceitar operações após escrita parcial")
	}
}

func TestClientSetReturnsUnknownOutcomeAfterServerExecution(t *testing.T) {
	clientConn, serverConn := net.Pipe()

	client, err := New(clientConn)
	if err != nil {
		serverConn.Close()
		t.Fatalf("criar cliente: %v", err)
	}
	defer client.Close()

	type receivedSet struct {
		key         string
		value       []byte
		operationID []byte
	}

	executedSet := make(chan receivedSet, 1)
	serverErr := make(chan error, 1)

	go func() {
		connection, err := transporttcp.NewConnection(serverConn)
		if err != nil {
			serverErr <- err
			return
		}
		defer connection.Close()

		payload, err := connection.ReadFrame()
		if err != nil {
			serverErr <- err
			return
		}

		request := &memorydbpb.Request{}
		if err := proto.Unmarshal(payload, request); err != nil {
			serverErr <- err
			return
		}

		command, ok := request.GetCommand().(*memorydbpb.Request_Set)
		if !ok {
			serverErr <- errors.New("expected SET command")
			return
		}

		// Simula a execução da mutação antes da perda da resposta.
		executedSet <- receivedSet{
			key:         command.Set.GetKey(),
			value:       append([]byte(nil), command.Set.GetValue()...),
			operationID: append([]byte(nil), command.Set.GetOperationId()...),
		}

		// Encerra a conexão sem enviar a resposta.
	}()

	expectedKey := "user:1"
	expectedValue := []byte("memorydb")
	operationID := newTestOperationID(t)

	err = client.Set(operationID, expectedKey, expectedValue)
	if err == nil {
		t.Fatal("esperava resultado indeterminado após perda da resposta")
	}

	var outcomeErr *OperationOutcomeUnknownError
	if !errors.As(err, &outcomeErr) {
		t.Fatalf(
			"esperava OperationOutcomeUnknownError, obtido %T: %v",
			err,
			err,
		)
	}

	if outcomeErr.RequestID != 1 {
		t.Fatalf(
			"request ID inesperado: obtido %d, esperado 1",
			outcomeErr.RequestID,
		)
	}

	select {
	case got := <-executedSet:
		if got.key != expectedKey {
			t.Errorf(
				"chave inesperada: obtida %q, esperada %q",
				got.key,
				expectedKey,
			)
		}

		if !reflect.DeepEqual(got.value, expectedValue) {
			t.Errorf(
				"valor inesperado: obtido %q, esperado %q",
				got.value,
				expectedValue,
			)
		}

		if !reflect.DeepEqual(got.operationID, operationID[:]) {
			t.Errorf(
				"OperationID inesperado: obtido %x, esperado %x",
				got.operationID,
				operationID[:],
			)
		}

	case err := <-serverErr:
		t.Fatalf("servidor simulado não conseguiu executar SET: %v", err)

	case <-time.After(2 * time.Second):
		t.Fatal("servidor não executou SET dentro do prazo")
	}

	if err := client.checkState(); err == nil ||
		!strings.Contains(err.Error(), "failed") {
		t.Fatalf(
			"esperava cliente em estado failed, obtido: %v",
			err,
		)
	}

	if err := client.Ping(); err == nil {
		t.Fatal("cliente não deveria aceitar operações após perda da resposta")
	}
}

func newTestOperationID(t *testing.T) OperationID {
	t.Helper()

	operationID, err := NewOperationID()
	if err != nil {
		t.Fatalf("gerar OperationID: %v", err)
	}

	return operationID
}

func TestNewOperationID(t *testing.T) {
	first, err := NewOperationID()
	if err != nil {
		t.Fatalf("gerar primeiro OperationID: %v", err)
	}

	second, err := NewOperationID()
	if err != nil {
		t.Fatalf("gerar segundo OperationID: %v", err)
	}

	if first == (OperationID{}) {
		t.Fatal("OperationID não deveria ser zero")
	}

	if first == second {
		t.Fatal("identificadores gerados deveriam ser diferentes")
	}
}

func TestServerErrorError(t *testing.T) {
	err := &ServerError{
		Code:    "TEST_ERROR",
		Message: "mensagem de teste",
	}

	expected := "MemoryDB error [TEST_ERROR]: mensagem de teste"

	if got := err.Error(); got != expected {
		t.Fatalf("mensagem inesperada: obtida %q, esperada %q", got, expected)
	}
}

func TestClientDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("criar listener: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)

	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client, err := Dial(listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial retornou erro: %v", err)
	}
	defer client.Close()

	select {
	case conn := <-accepted:
		defer conn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("servidor não aceitou a conexão")
	}
}

func TestClientAcquireOperationRejectsNilContext(t *testing.T) {
	client := newTestClient(t)

	if err := client.acquireOperation(nil); err == nil {
		t.Fatal("esperava erro para contexto nil")
	}
}

func TestClientCheckStateRejectsInvalidState(t *testing.T) {
	client := newTestClient(t)

	client.stateMu.Lock()
	client.state = clientState(255)
	client.stateMu.Unlock()

	err := client.checkState()
	if err == nil {
		t.Fatal("esperava erro para estado inválido")
	}

	if !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestClientDeleteWithCanceledContext(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deleted, err := client.DeleteContext(
		ctx,
		newTestOperationID(t),
		"user:1",
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, obtido: %v", err)
	}

	if deleted {
		t.Fatal("DELETE não deveria indicar remoção")
	}

	if err := client.checkState(); err != nil {
		t.Fatalf("cliente não deveria ser invalidado: %v", err)
	}
}

func TestClientRequestIDOverflow(t *testing.T) {
	client := newTestClient(t)

	client.requestID = ^uint64(0)

	if err := client.Ping(); err != nil {
		t.Fatalf("Ping retornou erro após overflow: %v", err)
	}

	if client.requestID != 1 {
		t.Fatalf("requestID após overflow = %d, esperado 1", client.requestID)
	}
}

func TestClientRejectsInvalidUTF8Request(t *testing.T) {
	client := newTestClient(t)

	invalidKey := string([]byte{0xff})

	_, _, err := client.Get(invalidKey)
	if err == nil {
		t.Fatal("esperava erro ao serializar chave com UTF-8 inválido")
	}

	if !strings.Contains(err.Error(), "encode request") {
		t.Fatalf("erro inesperado: %v", err)
	}

	if err := client.checkState(); err != nil {
		t.Fatalf("cliente não deveria ser invalidado: %v", err)
	}

	if err := client.Ping(); err != nil {
		t.Fatalf("cliente deveria continuar utilizável: %v", err)
	}
}

func TestClientAcquireOperationReleasesGateAfterCancellation(t *testing.T) {
	client := newTestClient(t)

	ctx := &cancelOnSecondErrContext{
		Context: context.Background(),
	}

	err := client.acquireOperation(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("esperava context.Canceled, obtido: %v", err)
	}

	if ctx.errCalls != 2 {
		t.Fatalf("ctx.Err() chamado %d vezes, esperado 2", ctx.errCalls)
	}

	// Confirma que o semáforo foi liberado após o cancelamento.
	if err := client.acquireOperation(context.Background()); err != nil {
		t.Fatalf("semáforo deveria estar disponível: %v", err)
	}

	client.releaseOperation()
}

func TestClientCancellationBeforeCommunication(t *testing.T) {
	client := newTestClient(t)
	ctx := newCancelOnThirdErrContext()

	err := client.PingContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PingContext() error = %v, want %v", err, context.Canceled)
	}

	if err := client.checkState(); err != nil {
		t.Fatalf("client state after cancellation = %v, want open", err)
	}

	if err := client.Ping(); err != nil {
		t.Fatalf("Ping() after cancellation error = %v", err)
	}
}

func TestClientCancellationWaitsForCallbackCompletion(t *testing.T) {
	client := newTestClient(t)
	ctx := newCallbackStartingContext()

	err := client.PingContext(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PingContext() error = %v, want %v", err, context.Canceled)
	}

	if err := client.checkState(); err != nil {
		t.Fatalf("client state after cancellation = %v, want open", err)
	}

	if err := client.Ping(); err != nil {
		t.Fatalf("Ping() after cancellation error = %v", err)
	}
}
