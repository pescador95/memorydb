package main

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	serverconfig "github.com/pescador95/memorydb/server/internal/config"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

type failingListener struct {
	err        error
	closeCalls int
}

func (l *failingListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (l *failingListener) Close() error {
	l.closeCalls++
	return nil
}

func (l *failingListener) Addr() net.Addr {
	return &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 6380,
	}
}

type shutdownFailingListener struct {
	closeErr error
	closed   chan struct{}
	started  chan struct{}
}

func (l *shutdownFailingListener) Accept() (net.Conn, error) {
	close(l.started)
	<-l.closed
	return nil, net.ErrClosed
}

func (l *shutdownFailingListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}

	return l.closeErr
}

func (l *shutdownFailingListener) Addr() net.Addr {
	return &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 6380,
	}
}

func TestRunRejectsInvalidConfigurationBeforeListening(t *testing.T) {
	t.Setenv("MEMORYDB_SHUTDOWN_TIMEOUT", "invalid-duration")

	err := run()
	if err == nil {
		t.Fatal("expected configuration error")
	}

	if !strings.Contains(err.Error(), "load server configuration") {
		t.Fatalf("expected configuration loading error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "MEMORYDB_SHUTDOWN_TIMEOUT") {
		t.Fatalf("expected shutdown timeout error, got: %v", err)
	}

	if strings.Contains(err.Error(), "listen on") {
		t.Fatalf("expected configuration validation before listening, got: %v", err)
	}
}

func TestRunWithConfigurationFailure(t *testing.T) {
	sentinel := errors.New("configuration failure")

	listenCalls := 0
	notifyCalls := 0

	err := runWith(
		func() (serverconfig.Config, error) {
			return serverconfig.Config{}, sentinel
		},
		func(network, address string) (net.Listener, error) {
			listenCalls++
			return nil, nil
		},
		func(
			parent context.Context,
			signals ...os.Signal,
		) (context.Context, context.CancelFunc) {
			notifyCalls++
			return context.WithCancel(parent)
		},
	)

	if !errors.Is(err, sentinel) {
		t.Fatalf("esperava erro %v, recebido %v", sentinel, err)
	}

	if listenCalls != 0 {
		t.Errorf("listen foi chamado %d vezes; esperado 0", listenCalls)
	}

	if notifyCalls != 0 {
		t.Errorf("notifyContext foi chamado %d vezes; esperado 0", notifyCalls)
	}
}

func TestRunWithListenFailure(t *testing.T) {
	sentinel := errors.New("listen failure")

	listenCalls := 0
	notifyCalls := 0

	config := serverconfig.Config{
		Address: ":6380",
	}

	err := runWith(
		func() (serverconfig.Config, error) {
			return config, nil
		},
		func(network, address string) (net.Listener, error) {
			listenCalls++

			if network != "tcp" {
				t.Errorf("network recebido: %q, esperado tcp", network)
			}

			if address != config.Address {
				t.Errorf("address recebido: %q, esperado %q", address, config.Address)
			}

			return nil, sentinel
		},
		func(
			parent context.Context,
			signals ...os.Signal,
		) (context.Context, context.CancelFunc) {
			notifyCalls++
			return context.WithCancel(parent)
		},
	)

	if !errors.Is(err, sentinel) {
		t.Fatalf("esperava erro %v, recebido %v", sentinel, err)
	}

	if listenCalls != 1 {
		t.Errorf("listen foi chamado %d vezes; esperado 1", listenCalls)
	}

	if notifyCalls != 0 {
		t.Errorf("notifyContext foi chamado %d vezes; esperado 0", notifyCalls)
	}
}

func TestRunWithGracefulShutdown(t *testing.T) {
	config := serverconfig.Config{
		Address:         "127.0.0.1:0",
		ShutdownTimeout: 2 * time.Second,
		ReadTimeout:     sharedconfig.DefaultServerReadTimeout,
		WriteTimeout:    sharedconfig.DefaultServerWriteTimeout,
		MaxConnections:  sharedconfig.DefaultServerMaxConnections,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listenerCreated := make(chan net.Addr, 1)
	notifyCalls := 0

	done := make(chan error, 1)

	go func() {
		done <- runWith(
			func() (serverconfig.Config, error) {
				return config, nil
			},
			func(network, address string) (net.Listener, error) {
				listener, err := net.Listen(network, address)
				if err == nil {
					listenerCreated <- listener.Addr()
				}
				return listener, err
			},
			func(
				parent context.Context,
				signals ...os.Signal,
			) (context.Context, context.CancelFunc) {
				notifyCalls++
				return ctx, func() {}
			},
		)
	}()

	var address net.Addr

	select {
	case address = <-listenerCreated:
	case err := <-done:
		t.Fatalf("runWith terminou antes de criar o listener: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando criação do listener")
	}

	conn, err := net.DialTimeout("tcp", address.String(), time.Second)
	if err != nil {
		t.Fatalf("conectar ao servidor: %v", err)
	}
	_ = conn.Close()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWith retornou erro durante shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout aguardando encerramento do servidor")
	}

	if notifyCalls != 1 {
		t.Errorf("notifyContext foi chamado %d vezes; esperado 1", notifyCalls)
	}
}

func TestRunWithServeFailure(t *testing.T) {
	sentinel := errors.New("unexpected accept failure")

	listener := &failingListener{
		err: sentinel,
	}

	notifyCalls := 0

	err := runWith(
		func() (serverconfig.Config, error) {
			return serverconfig.Config{
				Address:         ":6380",
				ShutdownTimeout: time.Second,
				ReadTimeout:     sharedconfig.DefaultServerReadTimeout,
				WriteTimeout:    sharedconfig.DefaultServerWriteTimeout,
				MaxConnections:  sharedconfig.DefaultServerMaxConnections,
			}, nil
		},
		func(network, address string) (net.Listener, error) {
			return listener, nil
		},
		func(
			parent context.Context,
			signals ...os.Signal,
		) (context.Context, context.CancelFunc) {
			notifyCalls++
			return context.WithCancel(parent)
		},
	)

	if !errors.Is(err, sentinel) {
		t.Fatalf("esperava erro %v, recebido %v", sentinel, err)
	}

	if listener.closeCalls != 1 {
		t.Errorf(
			"listener.Close foi chamado %d vezes; esperado 1",
			listener.closeCalls,
		)
	}

	if notifyCalls != 1 {
		t.Errorf(
			"notifyContext foi chamado %d vezes; esperado 1",
			notifyCalls,
		)
	}
}

func TestRunWithInvalidIdempotencyConfiguration(t *testing.T) {
	listener := &failingListener{
		err: errors.New("listener should not be used"),
	}

	err := runWith(
		func() (serverconfig.Config, error) {
			return serverconfig.Config{
				Address:        ":6380",
				ReadTimeout:    sharedconfig.DefaultServerReadTimeout,
				WriteTimeout:   sharedconfig.DefaultServerWriteTimeout,
				MaxConnections: sharedconfig.DefaultServerMaxConnections,
				Idempotency: idempotency.Config{
					Retention: -time.Second,
					Capacity:  100,
				},
			}, nil
		},
		func(network, address string) (net.Listener, error) {
			return listener, nil
		},
		func(
			parent context.Context,
			signals ...os.Signal,
		) (context.Context, context.CancelFunc) {
			return context.WithCancel(parent)
		},
	)

	if err == nil {
		t.Fatal("expected idempotency executor error")
	}

	if !strings.Contains(err.Error(), "create idempotency executor") {
		t.Fatalf("unexpected error: %v", err)
	}

	if listener.closeCalls != 1 {
		t.Errorf(
			"expected listener to be closed once, got %d",
			listener.closeCalls,
		)
	}
}

func TestRunWithShutdownFailure(t *testing.T) {
	sentinel := errors.New("listener close failure")

	listener := &shutdownFailingListener{
		closeErr: sentinel,
		closed:   make(chan struct{}),
		started:  make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)

	go func() {
		done <- runWith(
			func() (serverconfig.Config, error) {
				return serverconfig.Config{
					Address:         ":6380",
					ShutdownTimeout: time.Second,
					ReadTimeout:     sharedconfig.DefaultServerReadTimeout,
					WriteTimeout:    sharedconfig.DefaultServerWriteTimeout,
					MaxConnections:  sharedconfig.DefaultServerMaxConnections,
				}, nil
			},
			func(network, address string) (net.Listener, error) {
				return listener, nil
			},
			func(
				parent context.Context,
				signals ...os.Signal,
			) (context.Context, context.CancelFunc) {
				return ctx, func() {}
			},
		)
	}()

	// Aguarda o servidor iniciar a chamada Accept.
	select {
	case <-listener.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando o servidor iniciar Accept")
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected shutdown error %v, got %v", sentinel, err)
		}

		if !strings.Contains(err.Error(), "shutdown server") {
			t.Fatalf("unexpected error: %v", err)
		}

	case <-time.After(3 * time.Second):
		t.Fatal("timeout aguardando falha no shutdown")
	}
}
