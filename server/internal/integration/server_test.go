package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	memorydb "github.com/pescador95/memorydb/drivers/go"
	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	servertcp "github.com/pescador95/memorydb/server/internal/transport/tcp"
)

const shutdownTimeout = 5 * time.Second

type testServer struct {
	server    *servertcp.Server
	address   string
	serveDone chan error
}

func startTestServer(t *testing.T) *testServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("iniciar listener TCP: %v", err)
	}

	messageCodec := protobuf.New()

	commandEngine := engine.New()

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := servertcp.NewHandler(commandEngine, messageCodec, executor)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	if err != nil {
		_ = listener.Close()
		t.Fatalf("criar handler: %v", err)
	}

	server, err := servertcp.NewServer(listener, handler)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("criar servidor: %v", err)
	}

	instance := &testServer{
		server:    server,
		address:   listener.Addr().String(),
		serveDone: make(chan error, 1),
	}

	go func() {
		instance.serveDone <- server.Serve()
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("encerrar servidor: %v", err)
		}

		select {
		case err := <-instance.serveDone:
			if err != nil {
				t.Errorf("execução do servidor: %v", err)
			}
		case <-ctx.Done():
			t.Errorf("tempo limite aguardando encerramento do servidor")
		}
	})

	return instance
}

func (s *testServer) newClient(t *testing.T) *memorydb.Client {
	t.Helper()

	client, err := memorydb.Dial(s.address)
	if err != nil {
		t.Fatalf("conectar ao servidor: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return client
}

func TestServerPing(t *testing.T) {
	server := startTestServer(t)
	client := server.newClient(t)

	if err := client.Ping(); err != nil {
		t.Fatalf("executar PING: %v", err)
	}
}

func TestServerSetGetDelete(t *testing.T) {
	server := startTestServer(t)
	client := server.newClient(t)

	key := "usuario:1"
	value := []byte("MemoryDB")

	if err := client.Set(newOperationID(t), key, value); err != nil {
		t.Fatalf("executar SET: %v", err)
	}

	got, found, err := client.Get(key)
	if err != nil {
		t.Fatalf("executar GET: %v", err)
	}

	if !found {
		t.Fatal("GET deveria encontrar a chave")
	}

	if string(got) != string(value) {
		t.Fatalf("valor incorreto: esperado %q, recebido %q", value, got)
	}

	deleted, err := client.Delete(newOperationID(t), key)
	if err != nil {
		t.Fatalf("executar DELETE: %v", err)
	}

	if !deleted {
		t.Fatal("DELETE deveria indicar que a chave foi removida")
	}

	_, found, err = client.Get(key)
	if err != nil {
		t.Fatalf("executar GET após DELETE: %v", err)
	}

	if found {
		t.Fatal("GET não deveria encontrar uma chave removida")
	}
}

func TestServerEmptyValueAndMissingKey(t *testing.T) {
	server := startTestServer(t)
	client := server.newClient(t)

	if err := client.Set(newOperationID(t), "valor-vazio", []byte{}); err != nil {
		t.Fatalf("armazenar valor vazio: %v", err)
	}

	value, found, err := client.Get("valor-vazio")
	if err != nil {
		t.Fatalf("recuperar valor vazio: %v", err)
	}

	if !found {
		t.Fatal("a chave com valor vazio deveria existir")
	}

	if len(value) != 0 {
		t.Fatalf("esperava valor vazio, recebeu %v", value)
	}

	_, found, err = client.Get("chave-inexistente")
	if err != nil {
		t.Fatalf("consultar chave inexistente: %v", err)
	}

	if found {
		t.Fatal("uma chave inexistente não deveria ser encontrada")
	}
}

func TestServerMultipleClientsShareStorage(t *testing.T) {
	server := startTestServer(t)

	writer := server.newClient(t)
	reader := server.newClient(t)

	key := "compartilhada"
	value := []byte("dados compartilhados")

	if err := writer.Set(newOperationID(t), key, value); err != nil {
		t.Fatalf("cliente escritor executar SET: %v", err)
	}

	got, found, err := reader.Get(key)
	if err != nil {
		t.Fatalf("cliente leitor executar GET: %v", err)
	}

	if !found {
		t.Fatal("o segundo cliente deveria encontrar a chave")
	}

	if string(got) != string(value) {
		t.Fatalf("valor incorreto: esperado %q, recebido %q", value, got)
	}
}

func TestServerConcurrentClients(t *testing.T) {
	server := startTestServer(t)

	const clientCount = 10

	clients := make([]*memorydb.Client, clientCount)

	for i := range clients {
		clients[i] = server.newClient(t)
	}

	operationIDs := make([]memorydb.OperationID, clientCount)

	for i := range operationIDs {
		operationIDs[i] = newOperationID(t)
	}

	var wg sync.WaitGroup

	for i, client := range clients {
		wg.Add(1)

		go func(
			index int,
			client *memorydb.Client,
			operationID memorydb.OperationID,
		) {
			defer wg.Done()

			key := fmt.Sprintf("cliente:%d", index)
			value := []byte(fmt.Sprintf("valor:%d", index))

			if err := client.Set(operationID, key, value); err != nil {
				t.Errorf("cliente %d executar SET: %v", index, err)
				return
			}

			got, found, err := client.Get(key)
			if err != nil {
				t.Errorf("cliente %d executar GET: %v", index, err)
				return
			}

			if !found {
				t.Errorf("cliente %d não encontrou sua chave", index)
				return
			}

			if string(got) != string(value) {
				t.Errorf(
					"cliente %d recebeu valor incorreto: esperado %q, recebido %q",
					index,
					value,
					got,
				)
			}
		}(i, client, operationIDs[i])
	}

	wg.Wait()
}

func TestServerClientDisconnectDoesNotStopServer(t *testing.T) {
	server := startTestServer(t)

	firstClient := server.newClient(t)

	if err := firstClient.Ping(); err != nil {
		t.Fatalf("primeiro cliente executar PING: %v", err)
	}

	if err := firstClient.Close(); err != nil {
		t.Fatalf("fechar primeiro cliente: %v", err)
	}

	secondClient := server.newClient(t)

	if err := secondClient.Ping(); err != nil {
		t.Fatalf("servidor deveria continuar atendendo após desconexão: %v", err)
	}
}

func TestServerShutdownWaitsForActiveConnections(t *testing.T) {
	server := startTestServer(t)
	client := server.newClient(t)

	// Confirma que o servidor aceitou e processou a conexão.
	if err := client.Ping(); err != nil {
		t.Fatalf("executar PING antes do encerramento: %v", err)
	}

	// Mantém a conexão ativa enquanto solicita o encerramento.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := server.server.Shutdown(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"esperava timeout enquanto a conexão permanecia ativa, recebido: %v",
			err,
		)
	}

	// Após fechar o cliente, o servidor deve conseguir concluir o encerramento.
	if err := client.Close(); err != nil {
		t.Fatalf("fechar conexão ativa: %v", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer shutdownCancel()

	if err := server.server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("concluir encerramento do servidor: %v", err)
	}
}

func newOperationID(t *testing.T) memorydb.OperationID {
	t.Helper()

	operationID, err := memorydb.NewOperationID()
	if err != nil {
		t.Fatalf("gerar OperationID: %v", err)
	}

	return operationID
}
