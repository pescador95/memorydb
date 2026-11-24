package main

import (
	"fmt"
	"log"

	clientconfig "github.com/pescador95/memorydb/api/client/internal/config"
	memorydb "github.com/pescador95/memorydb/drivers/go"
)

type memoryDBClient interface {
	Ping() error
	Set(operationID memorydb.OperationID, key string, value []byte) error
	Get(key string) ([]byte, bool, error)
	Delete(operationID memorydb.OperationID, key string) (bool, error)
	Close() error
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	return runWith(
		clientconfig.Load,
		dialClient,
		memorydb.NewOperationID,
	)
}

func runWith(
	loadConfig func() (clientconfig.Config, error),
	dial func(memorydb.DialConfig) (memoryDBClient, error),
	newOperationID func() (memorydb.OperationID, error),
) error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("carregar configuração do cliente: %w", err)
	}

	client, err := dial(memorydb.DialConfig{
		Address: config.Address,
		Timeout: config.DialTimeout,
	})
	if err != nil {
		return fmt.Errorf("conectar ao MemoryDB: %w", err)
	}
	defer client.Close()

	if err := executeExample(client, newOperationID); err != nil {
		return fmt.Errorf("executar demonstração: %w", err)
	}

	return nil
}

func dialClient(config memorydb.DialConfig) (memoryDBClient, error) {
	return memorydb.DialWithConfig(config)
}

func executeExample(
	client memoryDBClient,
	newOperationID func() (memorydb.OperationID, error),
) error {
	if err := client.Ping(); err != nil {
		return fmt.Errorf("executar PING: %w", err)
	}

	fmt.Println("MemoryDB conectado com sucesso!")

	key := "user:1"
	value := []byte("memorydb")

	operationID, err := newOperationID()
	if err != nil {
		return fmt.Errorf("gerar OperationID para SET: %w", err)
	}

	if err := client.Set(operationID, key, value); err != nil {
		return fmt.Errorf("executar SET: %w", err)
	}

	fmt.Printf("SET: chave %q armazenada\n", key)

	result, found, err := client.Get(key)
	if err != nil {
		return fmt.Errorf("executar GET: %w", err)
	}

	if !found {
		return fmt.Errorf("chave %q não encontrada", key)
	}

	fmt.Printf("GET: %s\n", string(result))

	operationID, err = newOperationID()
	if err != nil {
		return fmt.Errorf("gerar OperationID para DELETE: %w", err)
	}

	deleted, err := client.Delete(operationID, key)
	if err != nil {
		return fmt.Errorf("executar DELETE: %w", err)
	}

	fmt.Printf("DELETE: chave removida = %t\n", deleted)

	_, foundAfterDelete, err := client.Get(key)
	if err != nil {
		return fmt.Errorf("executar GET após DELETE: %w", err)
	}

	if foundAfterDelete {
		return fmt.Errorf("chave %q ainda existe após DELETE", key)
	}

	fmt.Printf("GET após DELETE: chave encontrada = %t\n", foundAfterDelete)

	return nil
}
