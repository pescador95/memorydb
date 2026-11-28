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

	// ErrServerNotRestartable indica que o servidor já iniciou seu ciclo de vida
	// e não pode ser iniciado novamente.
	ErrServerNotRestartable = errors.New("server cannot be restarted")
)

// ServerState representa o estado do ciclo de vida do servidor TCP.
type ServerState uint8

const (
	ServerStateStarting ServerState = iota
	ServerStateRunning
	ServerStateStopping
	ServerStateStopped
	ServerStateFailed
)

// Server aceita conexões TCP e delega seu processamento ao Handler.
type Server struct {
	listener net.Listener
	handler  *Handler

	mu              sync.Mutex
	state           ServerState
	serving         bool
	serveStarted    bool
	shuttingDown    bool
	activeConns     map[net.Conn]struct{}
	connections     sync.WaitGroup
	connectionsDone chan struct{}
	waitOnce        sync.Once
	maxConnections  int
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
		listener:        listener,
		handler:         handler,
		state:           ServerStateStarting,
		maxConnections:  config.MaxConnections,
		activeConns:     make(map[net.Conn]struct{}),
		connectionsDone: make(chan struct{}),
	}, nil
}

// State retorna o estado atual do ciclo de vida do servidor.
func (s *Server) State() ServerState {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state
}

// Ready indica se o servidor está em execução e aceitando conexões.
func (s *Server) Ready() bool {
	return s.State() == ServerStateRunning
}

// updateStoppedLocked atualiza o estado para STOPPED quando não há mais
// loop de aceitação nem conexões ativas.
// O chamador deve manter s.mu bloqueado.
func (s *Server) updateStoppedLocked() {
	if s.shuttingDown &&
		!s.serving &&
		len(s.activeConns) == 0 &&
		s.state != ServerStateFailed {
		s.state = ServerStateStopped
	}
}

// Serve aceita conexões e inicia uma goroutine para cada cliente.
func (s *Server) Serve() (serveErr error) {
	s.mu.Lock()

	switch s.state {
	case ServerStateStarting:
		s.serving = true
		s.serveStarted = true
		s.state = ServerStateRunning

	case ServerStateRunning:
		s.mu.Unlock()
		return ErrServerAlreadyServing

	case ServerStateStopped:
		// Preserva o comportamento anterior quando Shutdown é chamado
		// antes da primeira execução de Serve.
		if s.shuttingDown && !s.serveStarted {
			s.mu.Unlock()
			return nil
		}

		s.mu.Unlock()
		return ErrServerNotRestartable

	case ServerStateStopping, ServerStateFailed:
		s.mu.Unlock()
		return ErrServerNotRestartable
	}

	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.serving = false

		if serveErr != nil {
			s.state = ServerStateFailed
		} else {
			s.updateStoppedLocked()
		}

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
				s.updateStoppedLocked()
				s.mu.Unlock()

				s.connections.Done()
			}()

			_ = s.handler.Handle(conn)
		}(conn)
	}
}

// Shutdown interrompe a aceitação de conexões e aguarda as ativas terminarem.
// Se o contexto expirar, fecha as conexões restantes e retorna sem aguardar
// indefinidamente o término de suas goroutines.
func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("shutdown context cannot be nil")
	}

	s.mu.Lock()
	s.shuttingDown = true

	if s.state == ServerStateStarting || s.state == ServerStateRunning {
		s.state = ServerStateStopping
	}
	s.mu.Unlock()

	closeErr := s.listener.Close()
	if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		closeErr = fmt.Errorf("close listener: %w", closeErr)
	} else {
		closeErr = nil
	}

	if closeErr == nil {
		s.mu.Lock()
		s.updateStoppedLocked()
		s.mu.Unlock()
	}

	// O encerramento impede novos Add() no WaitGroup. Portanto, a espera
	// pode ser iniciada com segurança depois de marcar shuttingDown.
	s.waitOnce.Do(func() {
		go func() {
			s.connections.Wait()
			close(s.connectionsDone)
		}()
	})

	select {
	case <-s.connectionsDone:
		s.mu.Lock()
		s.updateStoppedLocked()
		s.mu.Unlock()

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

		// O fechamento interrompe operações de I/O, mas não garante
		// o término de processamento interno não cancelável.
		// O estado permanece STOPPING até o encerramento efetivo.
		return errors.Join(closeErr, ctx.Err())
	}
}
