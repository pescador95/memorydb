package tcp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/codec"
	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/protocol"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

func newTestServer(t *testing.T) (*Server, net.Listener) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		listener.Close()
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(engine.New(), protobuf.New(), executor)
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
