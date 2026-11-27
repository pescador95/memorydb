package tcp

import (
	"fmt"
	"net"
	"reflect"
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

	value := reflect.ValueOf(conn)

	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface,
		reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, fmt.Errorf("connection cannot be nil")
		}
	}

	return &Connection{
		conn: conn,
	}, nil
}

// ReadFrame lê um quadro completo da conexão.
func (c *Connection) ReadFrame() ([]byte, error) {
	payload, release, err := c.ReadFrameWithReservation(
		MaxFrameSize,
		nil,
	)
	if err != nil {
		return nil, err
	}

	if release != nil {
		release()
	}

	return payload, nil
}

// ReadFrameWithReservation lê um quadro obtendo uma reserva opcional
// antes de alocar o payload.
//
// A função release retornada deve ser chamada pelo consumidor depois
// de terminar o processamento do quadro.
func (c *Connection) ReadFrameWithReservation(
	maxSize uint32,
	reserve func(size uint32) (release func(), err error),
) ([]byte, func(), error) {
	if c == nil {
		return nil, nil, fmt.Errorf("connection cannot be nil")
	}

	c.readMu.Lock()
	defer c.readMu.Unlock()

	payload, release, err := ReadFrameWithReservation(
		c.conn,
		maxSize,
		reserve,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("read frame: %w", err)
	}

	return payload, release, nil
}

// WriteFrame escreve um quadro completo na conexão.
func (c *Connection) WriteFrame(payload []byte) error {
	if c == nil {
		return fmt.Errorf("connection cannot be nil")
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := WriteFrame(c.conn, payload); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

// Close encerra a conexão TCP.
func (c *Connection) Close() error {
	if c == nil {
		return fmt.Errorf("connection cannot be nil")
	}

	return c.conn.Close()
}
