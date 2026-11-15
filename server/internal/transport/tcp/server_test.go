package tcp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/protocol"
)

func newTestServer(t *testing.T) (*Server, net.Listener) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	handler, err := NewHandler(engine.New(), protobuf.New())
	if err != nil {
		listener.Close()
		t.Fatalf("NewHandler() error = %v", err)
	}

	server, err := NewServer(listener, handler)
	if err != nil {
		listener.Close()
		t.Fatalf("NewServer() error = %v", err)
	}

	return server, listener
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

		client, err := NewConnection(conn, protobuf.New())
		if err != nil {
			conn.Close()
			t.Fatalf("NewConnection() error = %v", err)
		}

		request := &protocol.Request{
			RequestID: i,
			Command:   &protocol.PingCommand{},
		}

		if err := client.WriteRequest(request); err != nil {
			conn.Close()
			t.Fatalf("WriteRequest() error = %v", err)
		}

		response, err := client.ReadResponse()
		if err != nil {
			conn.Close()
			t.Fatalf("ReadResponse() error = %v", err)
		}

		if response.RequestID != i {
			t.Errorf("RequestID = %d, want %d", response.RequestID, i)
		}

		if !response.Success {
			t.Errorf("Success = false, want true")
		}

		if _, ok := response.Result.(*protocol.PongResult); !ok {
			t.Errorf("Result type = %T, want *protocol.PongResult", response.Result)
		}

		if err := conn.Close(); err != nil {
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

	client, err := NewConnection(conn, protobuf.New())
	if err != nil {
		t.Fatalf("NewConnection() error = %v", err)
	}

	if err := client.WriteRequest(&protocol.Request{
		RequestID: 1,
		Command:   &protocol.PingCommand{},
	}); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}

	if _, err := client.ReadResponse(); err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := server.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() error = %v, want context deadline exceeded", err)
	}

	if err := conn.Close(); err != nil {
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
