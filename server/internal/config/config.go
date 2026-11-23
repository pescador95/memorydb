package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/pescador95/memorydb/server/internal/idempotency"
	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

const (
	envShutdownTimeout      = "MEMORYDB_SHUTDOWN_TIMEOUT"
	envIdempotencyRetention = "MEMORYDB_IDEMPOTENCY_RETENTION"
	envIdempotencyCapacity  = "MEMORYDB_IDEMPOTENCY_CAPACITY"

	defaultShutdownTimeout = 10 * time.Second
)

// Config representa as configurações operacionais do servidor.
type Config struct {
	Address         string
	ShutdownTimeout time.Duration
	Idempotency     idempotency.Config
}

// Load carrega as configurações do servidor a partir do ambiente.
//
// Variáveis ausentes ou vazias utilizam seus respectivos valores padrão.
// Valores explicitamente informados, mas inválidos, retornam erro.
func Load() (Config, error) {
	address := sharedconfig.EnvOrDefault(
		sharedconfig.EnvServerAddress,
		sharedconfig.DefaultServerAddress,
	)

	shutdownTimeout, err := loadDuration(
		envShutdownTimeout,
		defaultShutdownTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	retention, err := loadDuration(
		envIdempotencyRetention,
		idempotency.DefaultRetention,
	)
	if err != nil {
		return Config{}, err
	}

	capacity, err := loadPositiveInt(
		envIdempotencyCapacity,
		idempotency.DefaultCapacity,
	)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Address:         address,
		ShutdownTimeout: shutdownTimeout,
		Idempotency: idempotency.Config{
			Retention: retention,
			Capacity:  capacity,
		},
	}, nil
}

// loadDuration carrega uma duração positiva a partir do ambiente.
func loadDuration(name string, defaultValue time.Duration) (time.Duration, error) {
	value, exists := os.LookupEnv(name)
	if !exists || value == "" {
		return defaultValue, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}

	return duration, nil
}

// loadPositiveInt carrega um inteiro positivo a partir do ambiente.
func loadPositiveInt(name string, defaultValue int) (int, error) {
	value, exists := os.LookupEnv(name)
	if !exists || value == "" {
		return defaultValue, nil
	}

	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}

	if number < 1 {
		return 0, fmt.Errorf("%s must be positive", name)
	}

	return number, nil
}
