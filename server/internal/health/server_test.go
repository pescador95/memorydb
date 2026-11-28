package health

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewServerRejectsNilDependencies(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	if _, err := NewServer(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); !errors.Is(err, ErrNilListener) {
		t.Fatalf("NewServer() error = %v, want %v", err, ErrNilListener)
	}

	if _, err := NewServer(listener, nil); !errors.Is(err, ErrNilHandler) {
		t.Fatalf("NewServer() error = %v, want %v", err, ErrNilHandler)
	}
}

func TestServerServesRequests(t *testing.T) {
	handler, err := NewHandler(func() bool {
		return true
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	server, err := NewServer(listener, handler)
	if err != nil {
		listener.Close()
		t.Fatalf("NewServer() error = %v", err)
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve()
	}()

	client := &http.Client{
		Timeout: time.Second,
	}

	response, err := client.Get("http://" + listener.Addr().String() + "/livez")
	if err != nil {
		t.Fatalf("GET /livez error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /livez status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not return after shutdown")
	}
}

func TestServerCloseStopsServing(t *testing.T) {
	handler, err := NewHandler(func() bool {
		return true
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	server, err := NewServer(listener, handler)
	if err != nil {
		listener.Close()
		t.Fatalf("NewServer() error = %v", err)
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve()
	}()

	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve() did not return after Close")
	}
}

func TestServerShutdownRejectsNilContext(t *testing.T) {
	handler, err := NewHandler(func() bool {
		return true
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	server, err := NewServer(listener, handler)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	if err := server.Shutdown(nil); err == nil {
		t.Fatal("Shutdown(nil) error = nil, want error")
	}
}
