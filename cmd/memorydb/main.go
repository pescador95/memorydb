package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pescador95/memorydb/internal/codec/protobuf"
	"github.com/pescador95/memorydb/internal/engine"
	"github.com/pescador95/memorydb/internal/transport/tcp"
)

const (
	defaultAddress  = ":6380"
	shutdownTimeout = 10 * time.Second
)

// main inicia a aplicação MemoryDB.
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run configura as dependências e inicia o servidor.
func run() error {
	address := os.Getenv("MEMORYDB_ADDR")
	if address == "" {
		address = defaultAddress
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	defer listener.Close()

	messageCodec := protobuf.New()
	commandEngine := engine.New()

	handler, err := tcp.NewHandler(commandEngine, messageCodec)
	if err != nil {
		return fmt.Errorf("create handler: %w", err)
	}

	server, err := tcp.NewServer(listener, handler)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	ctx, stop := signal.NotifyContext(
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
			shutdownTimeout,
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
