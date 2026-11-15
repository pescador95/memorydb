package tcp

import (
	"fmt"
	"net"
	"sync"
)

// Connection gerencia a comunicação por uma conexão TCP.
type Connection struct {
	conn    net.Conn
	readMu  sync.Mutex
	writeMu sync.Mutex
}

// NewConnection cria uma conexão TCP a partir de uma conexão existente.
func NewConnection(conn net.Conn) (*Connection, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection cannot be nil")
	}

	return &Connection{
		conn: conn,
	}, nil
}

// ReadFrame lê um quadro completo da conexão.
func (c *Connection) ReadFrame() ([]byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	payload, err := ReadFrame(c.conn)
	if err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	return payload, nil
}

// WriteFrame escreve um quadro completo na conexão.
func (c *Connection) WriteFrame(payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := WriteFrame(c.conn, payload); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

// Close encerra a conexão TCP.
func (c *Connection) Close() error {
	return c.conn.Close()
}
