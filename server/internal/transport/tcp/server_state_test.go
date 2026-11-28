package tcp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type stateTestListener struct {
	acceptErr error
}

func (l *stateTestListener) Accept() (net.Conn, error) {
	return nil, l.acceptErr
}

func (l *stateTestListener) Close() error {
	return nil
}

func (l *stateTestListener) Addr() net.Addr {
	return &net.TCPAddr{}
}

func TestServerStartsInStartingState(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server, err := NewServer(listener, &Handler{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	if got := server.State(); got != ServerStateStarting {
		t.Fatalf("expected STARTING, got %v", got)
	}

	if server.Ready() {
		t.Fatal("server must not be ready before Serve")
	}

	_ = listener.Close()
}

func TestServerTransitionsFromRunningToStopped(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server, err := NewServer(listener, &Handler{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve()
	}()

	waitForServerState(t, server, ServerStateRunning)

	if !server.Ready() {
		t.Fatal("server must be ready while running")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after shutdown")
	}

	if got := server.State(); got != ServerStateStopped {
		t.Fatalf("expected STOPPED, got %v", got)
	}

	if server.Ready() {
		t.Fatal("stopped server must not be ready")
	}
}

func TestServerShutdownBeforeServeTransitionsToStopped(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server, err := NewServer(listener, &Handler{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if got := server.State(); got != ServerStateStopped {
		t.Fatalf("expected STOPPED, got %v", got)
	}

	if err := server.Serve(); err != nil {
		t.Fatalf("Serve after shutdown: %v", err)
	}
}

func TestServerUnexpectedAcceptErrorTransitionsToFailed(t *testing.T) {
	acceptErr := errors.New("unexpected accept failure")
	listener := &stateTestListener{acceptErr: acceptErr}

	server, err := NewServer(listener, &Handler{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	err = server.Serve()
	if !errors.Is(err, acceptErr) {
		t.Fatalf("expected accept error %v, got %v", acceptErr, err)
	}

	if got := server.State(); got != ServerStateFailed {
		t.Fatalf("expected FAILED, got %v", got)
	}

	if server.Ready() {
		t.Fatal("failed server must not be ready")
	}
}

func TestServerCannotRestartAfterFailure(t *testing.T) {
	listener := &stateTestListener{
		acceptErr: errors.New("unexpected accept failure"),
	}

	server, err := NewServer(listener, &Handler{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	if err := server.Serve(); err == nil {
		t.Fatal("expected Serve to fail")
	}

	if err := server.Serve(); !errors.Is(err, ErrServerNotRestartable) {
		t.Fatalf("expected ErrServerNotRestartable, got %v", err)
	}
}

func TestServerCannotRestartAfterShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server, err := NewServer(listener, &Handler{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve()
	}()

	waitForServerState(t, server, ServerStateRunning)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after shutdown")
	}

	if err := server.Serve(); !errors.Is(err, ErrServerNotRestartable) {
		t.Fatalf("expected ErrServerNotRestartable, got %v", err)
	}
}

func waitForServerState(t *testing.T, server *Server, expected ServerState) {
	t.Helper()

	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if got := server.State(); got == expected {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("server did not reach state %v; current state: %v",
				expected, server.State())
		case <-ticker.C:
		}
	}
}
