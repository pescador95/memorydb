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

	if err := client.Set(key, value); err != nil {
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

	deleted, err := client.Delete(key)
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

	deleted, err = client.Delete(key)
	if err != nil {
		t.Fatalf("segundo DELETE retornou erro: %v", err)
	}

	if deleted {
		t.Fatal("segundo DELETE não deveria indicar remoção")
	}
}

func TestClientEmptyValue(t *testing.T) {
	client := newTestClient(t)

	if err := client.Set("empty", []byte{}); err != nil {
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

	for i := 0; i < total; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			key := string(rune('A' + i))
			value := []byte(key)

			if err := client.Set(key, value); err != nil {
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

	if err := client.SetContext(ctx, "user:context", []byte("memorydb")); err != nil {
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

	deleted, err := client.DeleteContext(ctx, "user:context")
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

	operationDone := make(chan error, 1)

	go func() {
		operationDone <- client.PingContext(ctx)
	}()

	cancel()

	select {
	case err := <-operationDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("esperava context.Canceled, obtido: %v", err)
		}

	case <-time.After(2 * time.Second):
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

type cancelAfterReadConn struct {
	net.Conn
	remaining int
	cancel    context.CancelFunc
	once      sync.Once
}

func (c *cancelAfterReadConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)

	if n > 0 {
		c.remaining -= n

		if c.remaining <= 0 {
			c.once.Do(c.cancel)
		}
	}

	return n, err
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

	wrappedConn := &cancelAfterReadConn{
		Conn:      clientConn,
		remaining: 4 + len(responsePayload),
		cancel:    cancel,
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

	switch {
	case err == nil:
		// A conclusão da operação venceu a sincronização.
		if stateErr != nil {
			t.Fatalf(
				"cliente deveria permanecer aberto após resposta válida: %v",
				stateErr,
			)
		}

	case errors.Is(err, context.Canceled):
		// O cancelamento venceu a sincronização.
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

		if stateErr == nil ||
			!strings.Contains(stateErr.Error(), "failed") {
			t.Fatalf(
				"cliente deveria estar em estado failed, obtido: %v",
				stateErr,
			)
		}

	default:
		t.Fatalf(
			"esperava sucesso ou OperationOutcomeUnknownError com context.Canceled, obtido: %v",
			err,
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

	executedValue := make(chan []byte, 1)
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

		// Simula a execução bem-sucedida no servidor.
		value := append([]byte(nil), command.Set.GetValue()...)
		executedValue <- value

		// A conexão é encerrada sem enviar a resposta.
	}()

	expectedValue := []byte("memorydb")

	err = client.Set("user:1", expectedValue)
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
	case value := <-executedValue:
		if !reflect.DeepEqual(value, expectedValue) {
			t.Fatalf(
				"valor executado inesperado: obtido %q, esperado %q",
				value,
				expectedValue,
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
