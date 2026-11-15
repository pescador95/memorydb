package tcp

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/pescador95/memorydb/server/internal/codec"
	"github.com/pescador95/memorydb/server/internal/engine"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

// Handler coordena o transporte TCP, a decodificação e a execução dos comandos.
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

	connection, err := transporttcp.NewConnection(conn)
	if err != nil {
		return fmt.Errorf("create connection: %w", err)
	}
	defer connection.Close()

	for {
		payload, err := connection.ReadFrame()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return fmt.Errorf("read request frame: %w", err)
		}

		request, err := h.codec.DecodeRequest(payload)
		if err != nil {
			return fmt.Errorf("decode request: %w", err)
		}

		response := h.engine.Execute(request)

		responsePayload, err := h.codec.EncodeResponse(response)
		if err != nil {
			return fmt.Errorf("encode response: %w", err)
		}

		if err := connection.WriteFrame(responsePayload); err != nil {
			return fmt.Errorf("write response frame: %w", err)
		}
	}
}
