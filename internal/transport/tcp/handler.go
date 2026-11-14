package tcp

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/pescador95/memorydb/internal/codec"
	"github.com/pescador95/memorydb/internal/engine"
)

// Handler coordena a comunicação TCP e a execução dos comandos.
type Handler struct {
	engine *engine.Engine
	codec  codec.Codec
}

// NewHandler cria um Handler com suas dependências.
func NewHandler(commandEngine *engine.Engine, messageCodec codec.Codec) (*Handler, error) {
	if commandEngine == nil {
		return nil, fmt.Errorf("engine cannot be nil")
	}

	if messageCodec == nil {
		return nil, fmt.Errorf("codec cannot be nil")
	}

	return &Handler{
		engine: commandEngine,
		codec:  messageCodec,
	}, nil
}

// Handle processa solicitações enquanto a conexão permanecer aberta.
func (h *Handler) Handle(conn net.Conn) error {
	if conn == nil {
		return fmt.Errorf("connection cannot be nil")
	}

	connection, err := NewConnection(conn, h.codec)
	if err != nil {
		return fmt.Errorf("create connection: %w", err)
	}
	defer connection.Close()

	for {
		request, err := connection.ReadRequest()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("read request: %w", err)
		}

		response := h.engine.Execute(request)

		if err := connection.WriteResponse(response); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}
}
