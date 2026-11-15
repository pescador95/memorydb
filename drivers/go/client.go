package memorydb

import (
	"fmt"
	"net"
	"sync"

	memorydbpb "github.com/pescador95/memorydb/protocol/generated/go/memorydb/v1"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
	"google.golang.org/protobuf/proto"
)

// Client representa um cliente conectado ao MemoryDB.
//
// Um Client deve ser criado por Dial ou New.
// Seu valor zero não é utilizável.
//
// Os métodos podem ser chamados concorrentemente.
// As operações são serializadas por conexão.
type Client struct {
	connection *transporttcp.Connection
	mu         sync.Mutex
	requestID  uint64
}

// ServerError representa um erro retornado pelo MemoryDB.
type ServerError struct {
	Code    string
	Message string
}

// Error retorna a descrição do erro retornado pelo servidor.
func (e *ServerError) Error() string {
	return fmt.Sprintf("MemoryDB error [%s]: %s", e.Code, e.Message)
}

// Dial estabelece uma conexão TCP com o MemoryDB.
func Dial(address string) (*Client, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect to MemoryDB: %w", err)
	}

	client, err := New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create client: %w", err)
	}

	return client, nil
}

// New cria um cliente a partir de uma conexão TCP existente.
func New(conn net.Conn) (*Client, error) {
	connection, err := transporttcp.NewConnection(conn)
	if err != nil {
		return nil, fmt.Errorf("create TCP connection: %w", err)
	}

	return &Client{
		connection: connection,
	}, nil
}

// Close encerra a conexão com o MemoryDB.
func (c *Client) Close() error {
	return c.connection.Close()
}

// Ping verifica a disponibilidade do MemoryDB.
func (c *Client) Ping() error {
	response, err := c.execute(&memorydbpb.Request{
		Command: &memorydbpb.Request_Ping{
			Ping: &memorydbpb.PingCommand{},
		},
	})
	if err != nil {
		return err
	}

	if response.GetPong() == nil {
		return fmt.Errorf("unexpected PING response")
	}

	return nil
}

// Set grava um valor associado a uma chave.
func (c *Client) Set(key string, value []byte) error {
	response, err := c.execute(&memorydbpb.Request{
		Command: &memorydbpb.Request_Set{
			Set: &memorydbpb.SetCommand{
				Key:   key,
				Value: value,
			},
		},
	})
	if err != nil {
		return err
	}

	if response.GetSet() == nil {
		return fmt.Errorf("unexpected SET response")
	}

	return nil
}

// Get recupera o valor associado a uma chave.
func (c *Client) Get(key string) ([]byte, bool, error) {
	response, err := c.execute(&memorydbpb.Request{
		Command: &memorydbpb.Request_Get{
			Get: &memorydbpb.GetCommand{
				Key: key,
			},
		},
	})
	if err != nil {
		return nil, false, err
	}

	result := response.GetGet()
	if result == nil {
		return nil, false, fmt.Errorf("unexpected GET response")
	}

	value := append([]byte(nil), result.GetValue()...)

	return value, result.GetFound(), nil
}

// Delete remove uma chave do armazenamento.
func (c *Client) Delete(key string) (bool, error) {
	response, err := c.execute(&memorydbpb.Request{
		Command: &memorydbpb.Request_Delete{
			Delete: &memorydbpb.DeleteCommand{
				Key: key,
			},
		},
	})
	if err != nil {
		return false, err
	}

	result := response.GetDelete()
	if result == nil {
		return false, fmt.Errorf("unexpected DELETE response")
	}

	return result.GetDeleted(), nil
}

// execute envia uma solicitação e aguarda sua resposta correspondente.
func (c *Client) execute(request *memorydbpb.Request) (*memorydbpb.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.requestID++
	if c.requestID == 0 {
		c.requestID++
	}

	request.RequestId = c.requestID

	payload, err := proto.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode request %d: %w", request.RequestId, err)
	}

	if err := c.connection.WriteFrame(payload); err != nil {
		return nil, fmt.Errorf("send request %d: %w", request.RequestId, err)
	}

	responsePayload, err := c.connection.ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("read response %d: %w", request.RequestId, err)
	}

	response := &memorydbpb.Response{}
	if err := proto.Unmarshal(responsePayload, response); err != nil {
		return nil, fmt.Errorf("decode response %d: %w", request.RequestId, err)
	}

	if response.GetRequestId() != request.GetRequestId() {
		return nil, fmt.Errorf(
			"request ID mismatch: got %d, want %d",
			response.GetRequestId(),
			request.GetRequestId(),
		)
	}

	if !response.GetSuccess() {
		serverError := response.GetError()
		if serverError != nil {
			return nil, &ServerError{
				Code:    serverError.GetCode(),
				Message: serverError.GetMessage(),
			}
		}

		return nil, fmt.Errorf("request %d failed", request.GetRequestId())
	}

	return response, nil
}
