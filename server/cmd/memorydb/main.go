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
	memorystorage "github.com/pescador95/memorydb/storage/go"
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

	if err := config.Resources.Validate(); err != nil {
		return fmt.Errorf("validate resource configuration: %w", err)
	}

	if err := validateProtocolFrameLimits(config.Resources); err != nil {
		return fmt.Errorf("validate protocol frame limits: %w", err)
	}

	storage, err := memorystorage.NewWithConfig(memorystorage.Config{
		MaxEntries:   config.Resources.Storage.MaxEntries,
		MemoryBudget: config.Resources.Storage.MemoryBudget,
		MaxKeySize:   config.Resources.Storage.MaxKeySize,
		MaxValueSize: config.Resources.Storage.MaxValueSize,
	})
	if err != nil {
		return fmt.Errorf("create storage: %w", err)
	}

	idempotencyCapacity, err := resourceEntriesToInt(
		config.Resources.Idempotency.MaxEntries,
	)
	if err != nil {
		return fmt.Errorf("configure idempotency executor: %w", err)
	}

	idempotencyConfig := idempotency.Config{
		Retention:    config.Resources.Idempotency.Retention,
		Capacity:     idempotencyCapacity,
		MemoryBudget: config.Resources.Idempotency.MemoryBudget,
	}

	executor, err := idempotency.NewExecutor(&idempotencyConfig)
	if err != nil {
		return fmt.Errorf("create idempotency executor: %w", err)
	}

	messageCodec := protobuf.New()
	commandEngine := engine.New(storage)

	handler, err := tcp.NewHandlerWithConfig(
		commandEngine,
		messageCodec,
		executor,
		tcp.HandlerConfig{
			ReadTimeout:            config.ReadTimeout,
			WriteTimeout:           config.WriteTimeout,
			MaxInFlightRequests:    config.Resources.Network.MaxInFlightRequests,
			MaxFramePayloadSize:    config.Resources.Network.MaxFramePayloadSize,
			ProcessingMemoryBudget: config.Resources.Processing.MemoryBudget,
		},
	)
	if err != nil {
		return fmt.Errorf("create handler: %w", err)
	}

	listener, err := listen("tcp", config.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", config.Address, err)
	}
	defer listener.Close()

	server, err := tcp.NewServerWithConfig(
		listener,
		handler,
		tcp.ServerConfig{
			MaxConnections: config.Resources.Network.MaxConnections,
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

// resourceEntriesToInt converte um limite de entradas para int,
// respeitando a capacidade da arquitetura em execução.
func resourceEntriesToInt(entries int64) (int, error) {
	maxInt := int64(^uint(0) >> 1)

	if entries <= 0 || entries > maxInt {
		return 0, fmt.Errorf(
			"idempotency maximum entries %d exceeds supported int range",
			entries,
		)
	}

	return int(entries), nil
}
