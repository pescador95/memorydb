package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	serverconfig "github.com/pescador95/memorydb/server/internal/config"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/health"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	"github.com/pescador95/memorydb/server/internal/transport/tcp"
	memorystorage "github.com/pescador95/memorydb/storage/go"
)

var errServerStoppedUnexpectedly = errors.New("server stopped unexpectedly")

type serveResult struct {
	name string
	err  error
}

// main inicia a aplicação MemoryDB.
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run configura as dependências e inicia os servidores.
func run() error {
	return runWith(
		serverconfig.Load,
		net.Listen,
		signal.NotifyContext,
	)
}

// runWith inicializa os servidores TCP e HTTP usando dependências externas
// injetáveis.
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
		return fmt.Errorf("create TCP handler: %w", err)
	}

	tcpListener, err := listen("tcp", config.Address)
	if err != nil {
		return fmt.Errorf("listen on TCP address %s: %w", config.Address, err)
	}

	healthListener, err := listen("tcp", config.HealthAddress)
	if err != nil {
		return errors.Join(
			fmt.Errorf("listen on health address %s: %w", config.HealthAddress, err),
			closeListeners(tcpListener),
		)
	}

	tcpServer, err := tcp.NewServerWithConfig(
		tcpListener,
		handler,
		tcp.ServerConfig{
			MaxConnections: config.Resources.Network.MaxConnections,
		},
	)
	if err != nil {
		return errors.Join(
			fmt.Errorf("create TCP server: %w", err),
			closeListeners(tcpListener, healthListener),
		)
	}

	healthHandler, err := health.NewHandler(tcpServer.Ready)
	if err != nil {
		return errors.Join(
			fmt.Errorf("create health handler: %w", err),
			closeListeners(tcpListener, healthListener),
		)
	}

	healthServer, err := health.NewServer(healthListener, healthHandler)
	if err != nil {
		return errors.Join(
			fmt.Errorf("create health server: %w", err),
			closeListeners(tcpListener, healthListener),
		)
	}

	ctx, stop := notifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	serveResults := make(chan serveResult, 2)

	go func() {
		serveResults <- serveResult{
			name: "TCP",
			err:  tcpServer.Serve(),
		}
	}()

	go func() {
		serveResults <- serveResult{
			name: "health HTTP",
			err:  healthServer.Serve(),
		}
	}()

	log.Printf("MemoryDB TCP listening on %s", tcpListener.Addr())
	log.Printf("MemoryDB health HTTP listening on %s", healthListener.Addr())

	select {
	case result := <-serveResults:
		serveErr := unexpectedServeError(result)

		shutdownErr := shutdownServers(
			tcpServer,
			healthServer,
			config.ShutdownTimeout,
		)

		remainingErrors := collectServeErrors(serveResults, 1)

		return errors.Join(
			serveErr,
			wrapShutdownError(shutdownErr),
			errors.Join(remainingErrors...),
		)

	case <-ctx.Done():
		log.Println("Shutdown signal received")

		shutdownErr := shutdownServers(
			tcpServer,
			healthServer,
			config.ShutdownTimeout,
		)

		serveErrors := collectServeErrors(serveResults, 2)

		if shutdownErr == nil && len(serveErrors) == 0 {
			log.Println("MemoryDB stopped")
			return nil
		}

		return errors.Join(
			wrapShutdownError(shutdownErr),
			errors.Join(serveErrors...),
		)
	}
}

// shutdownServers encerra os servidores usando um único prazo global.
//
// O servidor TCP é encerrado primeiro para interromper a aceitação de novas
// conexões e permitir que o readiness indique indisponibilidade enquanto
// o servidor HTTP permanece ativo durante a drenagem.
//
// Se o encerramento HTTP falhar, Close força o fechamento das conexões.
func shutdownServers(
	tcpServer *tcp.Server,
	healthServer *health.Server,
	timeout time.Duration,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var shutdownErrors []error

	if err := tcpServer.Shutdown(ctx); err != nil {
		shutdownErrors = append(
			shutdownErrors,
			fmt.Errorf("shutdown TCP server: %w", err),
		)
	}

	if err := healthServer.Shutdown(ctx); err != nil {
		shutdownErrors = append(
			shutdownErrors,
			fmt.Errorf("shutdown health server: %w", err),
		)

		if closeErr := healthServer.Close(); closeErr != nil {
			shutdownErrors = append(
				shutdownErrors,
				fmt.Errorf("force close health server: %w", closeErr),
			)
		}
	}

	return errors.Join(shutdownErrors...)
}

// unexpectedServeError converte o retorno de Serve em erro quando o servidor
// termina antes de um encerramento solicitado.
func unexpectedServeError(result serveResult) error {
	if result.err == nil {
		return fmt.Errorf(
			"%s serve: %w",
			result.name,
			errServerStoppedUnexpectedly,
		)
	}

	return fmt.Errorf("%s serve: %w", result.name, result.err)
}

// collectServeErrors aguarda a quantidade indicada de resultados e preserva
// erros inesperados de encerramento.
func collectServeErrors(
	results <-chan serveResult,
	count int,
) []error {
	var serveErrors []error

	for range count {
		result := <-results
		if result.err != nil {
			serveErrors = append(
				serveErrors,
				fmt.Errorf("%s serve: %w", result.name, result.err),
			)
		}
	}

	return serveErrors
}

// wrapShutdownError adiciona contexto ao erro de encerramento.
func wrapShutdownError(err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("shutdown servers: %w", err)
}

// closeListeners fecha todos os listeners recebidos e preserva os erros.
func closeListeners(listeners ...net.Listener) error {
	var closeErrors []error

	for _, listener := range listeners {
		if err := listener.Close(); err != nil &&
			!errors.Is(err, net.ErrClosed) {
			closeErrors = append(
				closeErrors,
				fmt.Errorf("close listener %s: %w", listener.Addr(), err),
			)
		}
	}

	return errors.Join(closeErrors...)
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
