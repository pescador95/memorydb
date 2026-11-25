package tcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

var (
	// ErrServerAlreadyServing indica que o servidor já está em execução.
	ErrServerAlreadyServing = errors.New("server is already serving")
)

// Server aceita conexões TCP e delega seu processamento ao Handler.
type Server struct {
	listener net.Listener
	handler  *Handler

	mu             sync.Mutex
	serving        bool
	shuttingDown   bool
	activeConns    map[net.Conn]struct{}
	connections    sync.WaitGroup
	maxConnections int
}

// ServerConfig define os limites operacionais do servidor TCP.
type ServerConfig struct {
	MaxConnections int
}

func NewServer(listener net.Listener, handler *Handler) (*Server, error) {
	return NewServerWithConfig(listener, handler, ServerConfig{
		MaxConnections: sharedconfig.DefaultServerMaxConnections,
	})
}

func NewServerWithConfig(
	listener net.Listener,
	handler *Handler,
	config ServerConfig,
) (*Server, error) {
	if listener == nil {
		return nil, fmt.Errorf("listener cannot be nil")
	}

	if handler == nil {
		return nil, fmt.Errorf("handler cannot be nil")
	}

	if config.MaxConnections < 1 {
		return nil, fmt.Errorf("max connections must be positive")
	}

	return &Server{
		listener:       listener,
		handler:        handler,
		maxConnections: config.MaxConnections,
		activeConns:    make(map[net.Conn]struct{}),
	}, nil
}

// Serve aceita conexões e inicia uma goroutine para cada cliente.
func (s *Server) Serve() error {
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return nil
	}
	if s.serving {
		s.mu.Unlock()
		return ErrServerAlreadyServing
	}
	s.serving = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.serving = false
		s.mu.Unlock()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			shuttingDown := s.shuttingDown
			s.mu.Unlock()

			if shuttingDown {
				return nil
			}

			return fmt.Errorf("accept connection: %w", err)
		}

		s.mu.Lock()
		if s.shuttingDown {
			s.mu.Unlock()
			_ = conn.Close()
			return nil
		}

		if len(s.activeConns) >= s.maxConnections {
			s.mu.Unlock()
			_ = conn.Close()
			continue
		}

		s.activeConns[conn] = struct{}{}
		s.connections.Add(1)

		s.mu.Unlock()

		go func(conn net.Conn) {
			defer func() {
				s.mu.Lock()
				delete(s.activeConns, conn)
				s.mu.Unlock()

				s.connections.Done()
			}()

			_ = s.handler.Handle(conn)
		}(conn)
	}
}

// Shutdown interrompe a aceitação de conexões e aguarda as ativas terminarem.
// Se o contexto expirar, fecha as conexões restantes e aguarda suas goroutines.
func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("shutdown context cannot be nil")
	}

	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()

	closeErr := s.listener.Close()
	if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		closeErr = fmt.Errorf("close listener: %w", closeErr)
	} else {
		closeErr = nil
	}

	done := make(chan struct{})

	go func() {
		s.connections.Wait()
		close(done)
	}()

	select {
	case <-done:
		return closeErr

	case <-ctx.Done():
		s.mu.Lock()
		connections := make([]net.Conn, 0, len(s.activeConns))
		for conn := range s.activeConns {
			connections = append(connections, conn)
		}
		s.mu.Unlock()

		for _, conn := range connections {
			_ = conn.Close()
		}

		// O fechamento das conexões deve desbloquear as operações de I/O.
		// Aguardamos as goroutines para não retornar com processamento pendente.
		<-done

		return errors.Join(closeErr, ctx.Err())
	}
}
