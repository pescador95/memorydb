package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	serverconfig "github.com/pescador95/memorydb/server/internal/config"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/transport/tcp"
)

// main inicia a aplicação MemoryDB.
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run configura as dependências e inicia o servidor.
func run() error {
	return runWith(
		serverconfig.Load,
		net.Listen,
		signal.NotifyContext,
	)
}

// runWith inicializa o servidor usando dependências externas injetáveis.
func runWith(
	loadConfig func() (serverconfig.Config, error),
	listen func(network, address string) (net.Listener, error),
	notifyContext func(
		parent context.Context,
		signals ...os.Signal,
	) (context.Context, context.CancelFunc),
) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load server configuration: %w", err)
	}

	listener, err := listen("tcp", config.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", config.Address, err)
	}
	defer listener.Close()

	messageCodec := protobuf.New()
	commandEngine := engine.New()

	executor, err := idempotency.NewExecutor(&config.Idempotency)
	if err != nil {
		return fmt.Errorf("create idempotency executor: %w", err)
	}

	handler, err := tcp.NewHandlerWithConfig(
		commandEngine,
		messageCodec,
		executor,
		tcp.HandlerConfig{
			ReadTimeout:  config.ReadTimeout,
			WriteTimeout: config.WriteTimeout,
		},
	)
	if err != nil {
		return fmt.Errorf("create handler: %w", err)
	}

	server, err := tcp.NewServerWithConfig(
		listener,
		handler,
		tcp.ServerConfig{
			MaxConnections: config.MaxConnections,
		},
	)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	ctx, stop := notifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	serveResult := make(chan error, 1)

	go func() {
		serveResult <- server.Serve()
	}()

	log.Printf("MemoryDB listening on %s", listener.Addr())

	select {
	case err := <-serveResult:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil

	case <-ctx.Done():
		log.Println("Shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			config.ShutdownTimeout,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}

		if err := <-serveResult; err != nil {
			return fmt.Errorf("stop server: %w", err)
		}

		log.Println("MemoryDB stopped")
		return nil
	}
}
