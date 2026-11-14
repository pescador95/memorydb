package tcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

var (
	// ErrServerAlreadyServing indica que o servidor já está em execução.
	ErrServerAlreadyServing = errors.New("server is already serving")
)

// Server aceita conexões TCP e delega seu processamento ao Handler.
type Server struct {
	listener net.Listener
	handler  *Handler

	mu           sync.Mutex
	serving      bool
	shuttingDown bool
	connections  sync.WaitGroup
}

// NewServer cria um servidor TCP com as dependências fornecidas.
func NewServer(listener net.Listener, handler *Handler) (*Server, error) {
	if listener == nil {
		return nil, fmt.Errorf("listener cannot be nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("handler cannot be nil")
	}

	return &Server{
		listener: listener,
		handler:  handler,
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

		s.connections.Add(1)
		s.mu.Unlock()

		go func() {
			defer s.connections.Done()
			_ = s.handler.Handle(conn)
		}()
	}
}

// Shutdown interrompe a aceitação de conexões e aguarda as ativas terminarem.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shuttingDown = true
	err := s.listener.Close()
	s.mu.Unlock()

	if err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close listener: %w", err)
	}

	done := make(chan struct{})
	go func() {
		s.connections.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
