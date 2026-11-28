package health

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

const defaultReadHeaderTimeout = 5 * time.Second

var (
	ErrNilListener = errors.New("health listener is nil")
	ErrNilHandler  = errors.New("health handler is nil")
)

type Server struct {
	listener net.Listener
	server   *http.Server
}

func NewServer(listener net.Listener, handler http.Handler) (*Server, error) {
	if listener == nil {
		return nil, ErrNilListener
	}

	if handler == nil {
		return nil, ErrNilHandler
	}

	return &Server{
		listener: listener,
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: defaultReadHeaderTimeout,
		},
	}, nil
}

func (s *Server) Serve() error {
	err := s.server.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("health shutdown context is nil")
	}

	return s.server.Shutdown(ctx)
}

func (s *Server) Close() error {
	return s.server.Close()
}
