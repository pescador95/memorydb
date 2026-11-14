package main

import (
	"bytes"
	"fmt"
	"log"
	"net"

	"github.com/pescador95/memorydb/internal/codec/protobuf"
	"github.com/pescador95/memorydb/internal/protocol"
	"github.com/pescador95/memorydb/internal/transport/tcp"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run estabelece a conexão e seleciona os cenários de execução.
func run() error {
	conn, err := net.Dial("tcp", "127.0.0.1:6380")
	if err != nil {
		return fmt.Errorf("connect to MemoryDB: %w", err)
	}
	defer conn.Close()

	client, err := tcp.NewConnection(conn, protobuf.New())
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	// Descomente os cenários que deseja executar.

	//return ping(client)
	//return set(client, "user:1", []byte("memorydb"))
	// _, _, err = get(client, "user:1")
	// return err
	//return delete(client, "user:1")

	return crud(client, "user:1", []byte("memorydb"))
}

// ping verifica a disponibilidade do MemoryDB.
func ping(client *tcp.Connection) error {
	response, err := execute(client, &protocol.PingCommand{})
	if err != nil {
		return err
	}

	if _, ok := response.Result.(*protocol.PongResult); !ok {
		return fmt.Errorf("unexpected PING response: %T", response.Result)
	}

	fmt.Println("PING: PONG")
	return nil
}

// set grava um valor associado a uma chave.
func set(client *tcp.Connection, key string, value []byte) error {
	response, err := execute(client, &protocol.SetCommand{
		Key:   key,
		Value: value,
	})
	if err != nil {
		return err
	}

	if _, ok := response.Result.(*protocol.SetResult); !ok {
		return fmt.Errorf("unexpected SET response: %T", response.Result)
	}

	fmt.Printf("SET: %s = %s\n", key, value)
	return nil
}

// get recupera o valor associado a uma chave.
func get(client *tcp.Connection, key string) ([]byte, bool, error) {
	response, err := execute(client, &protocol.GetCommand{
		Key: key,
	})
	if err != nil {
		return nil, false, err
	}

	result, ok := response.Result.(*protocol.GetResult)
	if !ok {
		return nil, false, fmt.Errorf("unexpected GET response: %T", response.Result)
	}

	fmt.Printf("GET: %s found = %t\n", key, result.Found)

	if result.Found {
		fmt.Printf("GET: %s = %s\n", key, result.Value)
	}

	return result.Value, result.Found, nil
}

// delete remove uma chave do armazenamento.
func delete(client *tcp.Connection, key string) error {
	response, err := execute(client, &protocol.DeleteCommand{
		Key: key,
	})
	if err != nil {
		return err
	}

	result, ok := response.Result.(*protocol.DeleteResult)
	if !ok {
		return fmt.Errorf("unexpected DELETE response: %T", response.Result)
	}

	fmt.Printf("DELETE: %s removed = %t\n", key, result.Deleted)
	return nil
}

// crud executa um cenário completo de criação, leitura e remoção.
func crud(client *tcp.Connection, key string, value []byte) error {
	if err := ping(client); err != nil {
		return err
	}

	if err := set(client, key, value); err != nil {
		return err
	}

	storedValue, found, err := get(client, key)
	if err != nil {
		return err
	}

	if !found {
		return fmt.Errorf("key %q was not found after SET", key)
	}

	if !bytes.Equal(storedValue, value) {
		return fmt.Errorf("unexpected GET value: got %q, want %q", storedValue, value)
	}

	if err := delete(client, key); err != nil {
		return err
	}

	_, found, err = get(client, key)
	if err != nil {
		return err
	}

	if found {
		return fmt.Errorf("key %q still exists after DELETE", key)
	}

	fmt.Println("\nCRUD validation completed successfully")
	return nil
}

// execute envia um comando e valida a resposta correspondente.
func execute(
	client *tcp.Connection,
	command protocol.Command,
) (*protocol.Response, error) {
	request := &protocol.Request{
		RequestID: nextRequestID(),
		Command:   command,
	}

	if err := client.WriteRequest(request); err != nil {
		return nil, fmt.Errorf("send request %d: %w", request.RequestID, err)
	}

	response, err := client.ReadResponse()
	if err != nil {
		return nil, fmt.Errorf("read response %d: %w", request.RequestID, err)
	}

	if response.RequestID != request.RequestID {
		return nil, fmt.Errorf(
			"request ID mismatch: got %d, want %d",
			response.RequestID,
			request.RequestID,
		)
	}

	if !response.Success {
		if result, ok := response.Result.(*protocol.ErrorResult); ok {
			return nil, fmt.Errorf(
				"MemoryDB error [%s]: %s",
				result.Code,
				result.Message,
			)
		}

		return nil, fmt.Errorf("request %d failed", request.RequestID)
	}

	return response, nil
}

var requestID uint64

// nextRequestID gera identificadores sequenciais para as solicitações.
func nextRequestID() uint64 {
	requestID++
	return requestID
}
