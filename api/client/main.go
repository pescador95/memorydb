package main

import (
	"fmt"
	"log"

	memorydb "github.com/pescador95/memorydb/drivers/go"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	client, err := memorydb.Dial("127.0.0.1:6380")
	if err != nil {
		return fmt.Errorf("conectar ao MemoryDB: %w", err)
	}
	defer client.Close()

	if err := client.Ping(); err != nil {
		return fmt.Errorf("executar PING: %w", err)
	}

	fmt.Println("MemoryDB conectado com sucesso!")

	key := "user:1"
	value := []byte("memorydb")

	if err := client.Set(key, value); err != nil {
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

	deleted, err := client.Delete(key)
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
