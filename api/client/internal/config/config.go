package config

import (
	"fmt"
	"os"
	"time"

	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

const (
	defaultDialTimeout = 5 * time.Second
)

// Config representa as configurações de conexão do cliente.
type Config struct {
	Address     string
	DialTimeout time.Duration
}

// Load carrega as configurações do cliente a partir do ambiente.
//
// Variáveis ausentes ou vazias utilizam seus respectivos valores padrão.
// Valores explicitamente informados, mas inválidos, retornam erro.
func Load() (Config, error) {
	address := sharedconfig.EnvOrDefault(
		sharedconfig.EnvClientAddress,
		sharedconfig.DefaultClientAddress,
	)

	timeout, err := loadDialTimeout()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Address:     address,
		DialTimeout: timeout,
	}, nil
}

func loadDialTimeout() (time.Duration, error) {
	value, exists := os.LookupEnv(sharedconfig.EnvDialTimeout)
	if !exists || value == "" {
		return defaultDialTimeout, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf(
			"invalid %s: %w",
			sharedconfig.EnvDialTimeout,
			err,
		)
	}

	if timeout <= 0 {
		return 0, fmt.Errorf(
			"%s must be positive",
			sharedconfig.EnvDialTimeout,
		)
	}

	return timeout, nil
}
