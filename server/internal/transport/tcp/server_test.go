package tcp

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/codec"
	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/protocol"
	sharedconfig "github.com/pescador95/memorydb/shared/config"
	memorystorage "github.com/pescador95/memorydb/storage/go"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

type controlledListener struct {
	conn            net.Conn
	acceptErr       error
	closeErr        error
	acceptStarted   chan struct{}
	acceptReturning chan struct{}
	releaseAccept   chan struct{}
}

func newControlledListener(conn net.Conn) *controlledListener {
	return &controlledListener{
		conn:            conn,
		acceptStarted:   make(chan struct{}),
		acceptReturning: make(chan struct{}),
		releaseAccept:   make(chan struct{}),
	}
}

func (l *controlledListener) Accept() (net.Conn, error) {
	close(l.acceptStarted)

	if l.releaseAccept != nil {
		<-l.releaseAccept
	}

	close(l.acceptReturning)
	return l.conn, l.acceptErr
}

func (l *controlledListener) Close() error {
	return l.closeErr
}

func (l *controlledListener) Addr() net.Addr {
	return &net.TCPAddr{}
}

func newTestServer(t *testing.T) (*Server, net.Listener) {
	t.Helper()

	return newTestServerWithConstructor(t, NewServer)
}

func newTestServerWithConfig(
	t *testing.T,
	maxConnections int,
) (*Server, net.Listener) {
	t.Helper()

	return newTestServerWithConstructor(
		t,
		func(listener net.Listener, handler *Handler) (*Server, error) {
			return NewServerWithConfig(
				listener,
				handler,
				ServerConfig{
					MaxConnections: maxConnections,
				},
			)
		},
	)
}

func newTestServerWithConstructor(
	t *testing.T,
	constructor func(net.Listener, *Handler) (*Server, error),
) (*Server, net.Listener) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		_ = listener.Close()
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(engine.New(memorystorage.New()), protobuf.New(), executor)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("NewHandler() error = %v", err)
	}

	server, err := constructor(listener, handler)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("NewServer() error = %v", err)
	}

	return server, listener
}

func writeTestRequest(
	t *testing.T,
	conn *transporttcp.Connection,
	messageCodec codec.Codec,
	request *protocol.Request,
) {
	t.Helper()

	payload, err := messageCodec.EncodeRequest(request)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}

	if err := conn.WriteFrame(payload); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
}

func readTestResponse(
	t *testing.T,
	conn *transporttcp.Connection,
	messageCodec codec.Codec,
) *protocol.Response {
	t.Helper()

	payload, err := conn.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	response, err := messageCodec.DecodeResponse(payload)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
	}

	return response
}

func waitForActiveConnections(
	t *testing.T,
	server *Server,
	expected int,
) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for {
		server.mu.Lock()
		active := len(server.activeConns)
		server.mu.Unlock()

		if active == expected {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf(
				"active connections = %d, want %d",
				active,
				expected,
			)
		}

		time.Sleep(time.Millisecond)
	}
}

func TestServerAcceptsMultipleClients(t *testing.T) {
	server, listener := newTestServer(t)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	for i := uint64(1); i <= 3; i++ {
		conn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatalf("Dial() error = %v", err)
		}

		client, err := transporttcp.NewConnection(conn)
		if err != nil {
			conn.Close()
			t.Fatalf("NewConnection() error = %v", err)
		}

		messageCodec := protobuf.New()

		request := &protocol.Request{
			RequestID: i,
			Command:   &protocol.PingCommand{},
		}

		writeTestRequest(t, client, messageCodec, request)

		response := readTestResponse(t, client, messageCodec)

		if response.RequestID != i {
			t.Errorf("RequestID = %d, want %d", response.RequestID, i)
		}

		if !response.Success {
			t.Errorf("Success = false, want true")
		}

		if _, ok := response.Result.(*protocol.PongResult); !ok {
			t.Errorf("Result type = %T, want *protocol.PongResult", response.Result)
		}

		if err := client.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := <-serveDone; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServerRejectsSecondServe(t *testing.T) {
	server, _ := newTestServer(t)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	deadline := time.Now().Add(time.Second)

	for {
		server.mu.Lock()
		serving := server.serving
		server.mu.Unlock()

		if serving {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("server did not start serving")
		}

		time.Sleep(time.Millisecond)
	}

	if err := server.Serve(); !errors.Is(err, ErrServerAlreadyServing) {
		t.Fatalf("Serve() error = %v, want ErrServerAlreadyServing", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := <-serveDone; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestServerShutdownWaitsForActiveConnections(t *testing.T) {
	server, listener := newTestServer(t)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	client, err := transporttcp.NewConnection(conn)
	if err != nil {
		t.Fatalf("NewConnection() error = %v", err)
	}

	messageCodec := protobuf.New()

	request := &protocol.Request{
		RequestID: 1,
		Command:   &protocol.PingCommand{},
	}

	writeTestRequest(t, client, messageCodec, request)

	readTestResponse(t, client, messageCodec)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := server.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want context deadline exceeded", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()

	if err := server.Shutdown(ctx2); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}

	if err := <-serveDone; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestNewServerRejectsNilDependencies(t *testing.T) {
	if _, err := NewServer(nil, nil); err == nil {
		t.Error("NewServer() error = nil, want error")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	if _, err := NewServer(listener, nil); err == nil {
		t.Error("NewServer() with nil handler error = nil, want error")
	}
}

func TestServerShutdownBeforeServe(t *testing.T) {
	server, _ := newTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := server.Serve(); err != nil {
		t.Fatalf("Serve() após Shutdown() error = %v, want nil", err)
	}
}

func TestServerReturnsUnexpectedAcceptError(t *testing.T) {
	server, originalListener := newTestServer(t)
	defer originalListener.Close()

	acceptErr := errors.New("accept failed")
	listener := newControlledListener(nil)
	listener.releaseAccept = nil
	listener.acceptErr = acceptErr
	server.listener = listener

	err := server.Serve()
	if !errors.Is(err, acceptErr) {
		t.Fatalf("Serve() error = %v, want wrapped accept error", err)
	}
}

func TestServerClosesAcceptedConnectionDuringShutdown(t *testing.T) {
	server, originalListener := newTestServer(t)
	defer originalListener.Close()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	if err := clientConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	listener := newControlledListener(serverConn)
	server.listener = listener

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	<-listener.acceptStarted

	// Impede Serve de verificar o estado antes de o teste simular
	// o início do desligamento.
	server.mu.Lock()

	close(listener.releaseAccept)
	<-listener.acceptReturning

	server.shuttingDown = true
	server.mu.Unlock()

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not terminate after shutdown")
	}

	if _, err := clientConn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("client Read() error = %v, want io.EOF after shutdown", err)
	}
}

func TestServerShutdownReturnsListenerCloseError(t *testing.T) {
	server, originalListener := newTestServer(t)
	defer originalListener.Close()

	closeErr := errors.New("listener close failed")
	listener := newControlledListener(nil)
	listener.closeErr = closeErr
	server.listener = listener

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if !errors.Is(err, closeErr) {
		t.Fatalf("Shutdown() error = %v, want wrapped listener close error", err)
	}
}

func TestServerShutdownForceClosesIdleConnections(t *testing.T) {
	server, listener := newTestServer(t)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	// Aguarda o servidor registrar a conexão antes de iniciar o shutdown.
	deadline := time.Now().Add(time.Second)

	for {
		server.mu.Lock()
		active := len(server.activeConns)
		server.mu.Unlock()

		if active == 1 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("server did not register the active connection")
		}

		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = server.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"Shutdown() error = %v, want context deadline exceeded",
			err,
		)
	}

	// A conexão remota deve ter sido fechada pelo servidor.
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	var buffer [1]byte
	if _, err := client.Read(buffer[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("client Read() error = %v, want io.EOF after forced shutdown", err)
	}

	// A goroutine do servidor também deve ter terminado.
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not terminate after forced shutdown")
	}

	server.mu.Lock()
	remainingConnections := len(server.activeConns)
	server.mu.Unlock()

	if remainingConnections != 0 {
		t.Errorf("active connections = %d, want 0", remainingConnections)
	}
}

func TestNewServerUsesDefaultMaxConnections(t *testing.T) {
	server, listener := newTestServer(t)
	defer listener.Close()

	if server.maxConnections != sharedconfig.DefaultServerMaxConnections {
		t.Fatalf(
			"maxConnections = %d, want %d",
			server.maxConnections,
			sharedconfig.DefaultServerMaxConnections,
		)
	}
}

func TestNewServerWithConfigRejectsInvalidMaxConnections(t *testing.T) {
	server, listener := newTestServer(t)
	defer listener.Close()

	tests := []struct {
		name           string
		maxConnections int
	}{
		{
			name:           "zero",
			maxConnections: 0,
		},
		{
			name:           "negative",
			maxConnections: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServerWithConfig(
				server.listener,
				server.handler,
				ServerConfig{
					MaxConnections: tt.maxConnections,
				},
			)

			if err == nil {
				t.Fatal("NewServerWithConfig() error = nil, want error")
			}
		})
	}
}

func TestServerRejectsConnectionsAboveLimit(t *testing.T) {
	server, listener := newTestServerWithConfig(t, 1)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	firstClient, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("first Dial() error = %v", err)
	}

	waitForActiveConnections(t, server, 1)

	rejectedClient, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		firstClient.Close()
		t.Fatalf("second Dial() error = %v", err)
	}

	if err := rejectedClient.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	var buffer [1]byte
	_, err = rejectedClient.Read(buffer[:])

	if err == nil {
		t.Fatal("excess connection remained open")
	}

	if timeoutErr, ok := err.(net.Error); ok && timeoutErr.Timeout() {
		t.Fatalf("excess connection was not closed: %v", err)
	}

	_ = rejectedClient.Close()

	waitForActiveConnections(t, server, 1)

	if err := firstClient.Close(); err != nil {
		t.Fatalf("first client Close() error = %v", err)
	}

	waitForActiveConnections(t, server, 0)

	thirdClient, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("third Dial() error = %v", err)
	}
	defer thirdClient.Close()

	waitForActiveConnections(t, server, 1)

	connection, err := transporttcp.NewConnection(thirdClient)
	if err != nil {
		t.Fatalf("NewConnection() error = %v", err)
	}

	messageCodec := protobuf.New()

	writeTestRequest(t, connection, messageCodec, &protocol.Request{
		RequestID: 1,
		Command:   &protocol.PingCommand{},
	})

	response := readTestResponse(t, connection, messageCodec)

	if !response.Success {
		t.Fatal("third client PING failed after slot release")
	}

	if err := thirdClient.Close(); err != nil {
		t.Fatalf("thirdClient.Close() error = %v", err)
	}

	waitForActiveConnections(t, server, 0)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := <-serveDone; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	waitForActiveConnections(t, server, 0)
}

func TestServerRespectsMaxConnectionsUnderConcurrency(t *testing.T) {
	const (
		maxConnections = 3
		clientCount    = 20
	)

	server, listener := newTestServerWithConfig(t, maxConnections)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	clients := make([]net.Conn, clientCount)

	dialErrors := make(chan error, clientCount)

	var dialGroup sync.WaitGroup

	for i := 0; i < clientCount; i++ {
		dialGroup.Add(1)

		go func(index int) {
			defer dialGroup.Done()

			conn, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				dialErrors <- err
				return
			}

			clients[index] = conn
		}(i)
	}

	dialGroup.Wait()
	close(dialErrors)

	for err := range dialErrors {
		t.Errorf("Dial() error = %v", err)
	}

	waitForActiveConnections(t, server, maxConnections)

	activeClients := 0

	for _, client := range clients {
		if client == nil {
			continue
		}

		if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("SetReadDeadline() error = %v", err)
		}

		var buffer [1]byte
		_, err := client.Read(buffer[:])

		if err == nil {
			t.Fatal("unexpected data received from server")
		}

		if timeoutErr, ok := err.(net.Error); ok && timeoutErr.Timeout() {
			activeClients++
		}

		if err := client.SetReadDeadline(time.Time{}); err != nil {
			t.Fatalf("clear read deadline: %v", err)
		}
	}

	if activeClients != maxConnections {
		t.Fatalf(
			"active clients = %d, want %d",
			activeClients,
			maxConnections,
		)
	}

	server.mu.Lock()
	activeConnections := len(server.activeConns)
	server.mu.Unlock()

	if activeConnections > maxConnections {
		t.Fatalf(
			"active connections = %d, exceeds limit %d",
			activeConnections,
			maxConnections,
		)
	}

	for _, client := range clients {
		if client == nil {
			continue
		}

		if err := client.Close(); err != nil {
			t.Errorf("client.Close() error = %v", err)
		}
	}

	waitForActiveConnections(t, server, 0)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if err := <-serveDone; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	waitForActiveConnections(t, server, 0)
}

func TestServerContinuesAfterConnectionHandlerError(t *testing.T) {
	server, listener := newTestServer(t)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	messageCodec := protobuf.New()

	// Primeira conexão: envia um payload Protobuf inválido.
	firstClient, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() first client error = %v", err)
	}

	waitForActiveConnections(t, server, 1)

	firstConnection, err := transporttcp.NewConnection(firstClient)
	if err != nil {
		_ = firstClient.Close()
		t.Fatalf("NewConnection() first client error = %v", err)
	}

	if err := firstConnection.WriteFrame([]byte{0xff}); err != nil {
		_ = firstClient.Close()
		t.Fatalf("WriteFrame() invalid payload error = %v", err)
	}

	// O Handler deve encerrar somente a conexão problemática.
	waitForActiveConnections(t, server, 0)
	_ = firstClient.Close()

	// Segunda conexão: comprova que o servidor continua aceitando clientes.
	secondClient, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() second client error = %v", err)
	}
	defer secondClient.Close()

	if err := secondClient.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}

	waitForActiveConnections(t, server, 1)

	secondConnection, err := transporttcp.NewConnection(secondClient)
	if err != nil {
		t.Fatalf("NewConnection() second client error = %v", err)
	}

	writeTestRequest(
		t,
		secondConnection,
		messageCodec,
		&protocol.Request{
			RequestID: 1,
			Command:   &protocol.PingCommand{},
		},
	)

	response := readTestResponse(t, secondConnection, messageCodec)
	if response == nil {
		t.Fatal("expected a response from the second client")
	}

	if response.RequestID != 1 {
		t.Errorf("response RequestID = %d, want 1", response.RequestID)
	}

	_ = secondClient.Close()
	waitForActiveConnections(t, server, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not stop after shutdown")
	}
}
