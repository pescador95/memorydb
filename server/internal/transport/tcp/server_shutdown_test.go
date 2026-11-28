package tcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/protocol"
	memorystorage "github.com/pescador95/memorydb/storage/go"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

type shutdownBlockingCodec struct {
	decodeStarted chan struct{}
	releaseDecode chan struct{}
	releaseOnce   sync.Once
}

func newShutdownBlockingCodec() *shutdownBlockingCodec {
	return &shutdownBlockingCodec{
		decodeStarted: make(chan struct{}),
		releaseDecode: make(chan struct{}),
	}
}

func (c *shutdownBlockingCodec) EncodeRequest(*protocol.Request) ([]byte, error) {
	return []byte{1}, nil
}

func (c *shutdownBlockingCodec) DecodeRequest([]byte) (*protocol.Request, error) {
	close(c.decodeStarted)
	<-c.releaseDecode

	return &protocol.Request{
		RequestID: 1,
		Command:   &protocol.PingCommand{},
	}, nil
}

func (c *shutdownBlockingCodec) EncodeResponse(*protocol.Response) ([]byte, error) {
	return []byte{1}, nil
}

func (c *shutdownBlockingCodec) DecodeResponse([]byte) (*protocol.Response, error) {
	return nil, nil
}

func (c *shutdownBlockingCodec) release() {
	c.releaseOnce.Do(func() {
		close(c.releaseDecode)
	})
}

func TestServerShutdownReturnsAfterDeadlineWhenHandlerIsBlocked(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	messageCodec := newShutdownBlockingCodec()

	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		_ = listener.Close()
		t.Fatalf("NewExecutor() error = %v", err)
	}

	handler, err := NewHandler(
		engine.New(memorystorage.New()),
		messageCodec,
		executor,
	)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("NewHandler() error = %v", err)
	}

	server, err := NewServer(listener, handler)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("NewServer() error = %v", err)
	}

	// Garante que o teste sempre libere o codec, inclusive em caso de falha.
	defer messageCodec.release()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve()
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	waitForActiveConnections(t, server, 1)

	if err := transporttcp.WriteFrame(client, []byte{1}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	select {
	case <-messageCodec.decodeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Handler did not start decoding the request")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- server.Shutdown(ctx)
	}()

	select {
	case err := <-shutdownDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf(
				"Shutdown() error = %v, want context deadline exceeded",
				err,
			)
		}

	case <-time.After(time.Second):
		t.Fatal("Shutdown() did not return after its context expired")
	}

	// O Handler continua bloqueado mesmo depois de a conexão ser fechada.
	// Liberamos o codec para permitir que a goroutine termine naturalmente.
	messageCodec.release()

	waitForActiveConnections(t, server, 0)

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve() error = %v, want nil", err)
		}

	case <-time.After(time.Second):
		t.Fatal("Serve() did not terminate after shutdown")
	}

	// Com todas as conexões encerradas, uma nova chamada deve retornar
	// normalmente, mesmo que a chamada anterior tenha expirado.
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()

	if err := server.Shutdown(ctx2); err != nil {
		t.Fatalf("second Shutdown() error = %v, want nil", err)
	}
}
