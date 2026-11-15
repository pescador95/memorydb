package memorydb

import (
	"errors"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"

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

	if !strings.Contains(err.Error(), "unexpected PING response") {
		t.Fatalf("erro inesperado: %v", err)
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

	if !strings.Contains(err.Error(), "request 1 failed") {
		t.Fatalf("erro inesperado: %v", err)
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
}
