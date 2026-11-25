package config

import (
	"os"
	"time"
)

// Nomes das variáveis de ambiente utilizadas pelos componentes do MemoryDB.
const (
	EnvServerAddress        = "MEMORYDB_ADDR"
	EnvClientAddress        = "MEMORYDB_CLIENT_ADDR"
	EnvDialTimeout          = "MEMORYDB_DIAL_TIMEOUT"
	EnvServerMaxConnections = "MEMORYDB_MAX_CONNECTIONS"
)

// Valores padrão compartilhados entre os componentes do MemoryDB.
const (
	DefaultTCPPort = "6380"

	DefaultServerAddress = ":" + DefaultTCPPort
	DefaultClientAddress = "127.0.0.1:" + DefaultTCPPort

	DefaultServerReadTimeout  = 30 * time.Second
	DefaultServerWriteTimeout = 10 * time.Second

	DefaultServerMaxConnections = 1024
)

// EnvOrDefault retorna o valor da variável de ambiente ou o valor padrão
// quando a variável não está definida ou está vazia.
func EnvOrDefault(name, defaultValue string) string {
	value, exists := os.LookupEnv(name)
	if !exists || value == "" {
		return defaultValue
	}

	return value
}
