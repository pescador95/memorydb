package tcp

import (
	"fmt"
	"net"
	"sync"

	"github.com/pescador95/memorydb/internal/codec"
	"github.com/pescador95/memorydb/internal/protocol"
)

// Connection gerencia a comunicação de mensagens por uma conexão TCP.
type Connection struct {
	conn  net.Conn
	codec codec.Codec

	readMu  sync.Mutex
	writeMu sync.Mutex
}

// NewConnection cria uma conexão TCP associada a um codec.
func NewConnection(conn net.Conn, messageCodec codec.Codec) (*Connection, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection cannot be nil")
	}

	if messageCodec == nil {
		return nil, fmt.Errorf("codec cannot be nil")
	}

	return &Connection{
		conn:  conn,
		codec: messageCodec,
	}, nil
}

// ReadRequest lê e decodifica uma solicitação.
func (c *Connection) ReadRequest() (*protocol.Request, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	payload, err := ReadFrame(c.conn)
	if err != nil {
		return nil, fmt.Errorf("read request frame: %w", err)
	}

	request, err := c.codec.DecodeRequest(payload)
	if err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}

	return request, nil
}

// WriteRequest codifica e envia uma solicitação.
func (c *Connection) WriteRequest(request *protocol.Request) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	payload, err := c.codec.EncodeRequest(request)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	if err := WriteFrame(c.conn, payload); err != nil {
		return fmt.Errorf("write request frame: %w", err)
	}

	return nil
}

// ReadResponse lê e decodifica uma resposta.
func (c *Connection) ReadResponse() (*protocol.Response, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	payload, err := ReadFrame(c.conn)
	if err != nil {
		return nil, fmt.Errorf("read response frame: %w", err)
	}

	response, err := c.codec.DecodeResponse(payload)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return response, nil
}

// WriteResponse codifica e envia uma resposta.
func (c *Connection) WriteResponse(response *protocol.Response) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	payload, err := c.codec.EncodeResponse(response)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}

	if err := WriteFrame(c.conn, payload); err != nil {
		return fmt.Errorf("write response frame: %w", err)
	}

	return nil
}

// Close encerra a conexão TCP.
func (c *Connection) Close() error {
	return c.conn.Close()
}
