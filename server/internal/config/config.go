package config

import (
	"fmt"
	"os"
	"time"

	"github.com/pescador95/memorydb/server/resources"
	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

const (
	envShutdownTimeout = "MEMORYDB_SHUTDOWN_TIMEOUT"
	envReadTimeout     = "MEMORYDB_READ_TIMEOUT"
	envWriteTimeout    = "MEMORYDB_WRITE_TIMEOUT"
	envHealthAddress   = "MEMORYDB_HEALTH_ADDR"

	defaultHealthAddress   = ":8081"
	defaultShutdownTimeout = 10 * time.Second
)

// Config reúne as configurações operacionais e os recursos calculados
// para inicializar o servidor.
//
// Resources é a fonte única de verdade para os limites de recursos.
// Os componentes recebem somente as configurações de que necessitam.
type Config struct {
	Address         string
	ShutdownTimeout time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	HealthAddress   string
	Resources       resources.ResourceConfig
}

// Load carrega as configurações operacionais e detecta os recursos
// disponíveis no ambiente.
//
// A detecção é realizada uma única vez por chamada.
func Load() (Config, error) {
	return loadWithResourceConfig(resources.LoadResourceConfigFromEnv)
}

// loadWithResourceConfig permite testar o carregamento da configuração
// sem depender da detecção real de recursos da máquina.
func loadWithResourceConfig(
	loadResources func() (resources.ResourceConfig, error),
) (Config, error) {
	if loadResources == nil {
		return Config{}, fmt.Errorf("resource config loader cannot be nil")
	}

	resourceConfig, err := loadResources()
	if err != nil {
		return Config{}, fmt.Errorf("load resource configuration: %w", err)
	}

	if err := resourceConfig.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate resource configuration: %w", err)
	}

	address := environmentValue(
		sharedconfig.EnvServerAddress,
		sharedconfig.DefaultServerAddress,
	)

	healthAddress := environmentValue(
		envHealthAddress,
		defaultHealthAddress,
	)

	shutdownTimeout, err := loadDuration(
		envShutdownTimeout,
		defaultShutdownTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := loadDuration(
		envReadTimeout,
		sharedconfig.DefaultServerReadTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := loadDuration(
		envWriteTimeout,
		sharedconfig.DefaultServerWriteTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Address:         address,
		ShutdownTimeout: shutdownTimeout,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		HealthAddress:   healthAddress,
		Resources:       resourceConfig,
	}, nil
}

// environmentValue retorna o valor da variável de ambiente ou o fallback.
func environmentValue(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

// loadDuration carrega uma duração positiva ou retorna seu valor padrão.
func loadDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("invalid %s: duration must be positive", name)
	}

	return duration, nil
}
