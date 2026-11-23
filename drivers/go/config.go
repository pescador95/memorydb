package memorydb

import (
	"fmt"
	"time"
)

// DialConfig define as opções utilizadas para estabelecer
// uma conexão com o MemoryDB.
type DialConfig struct {
	Address string
	Timeout time.Duration
}

// validate verifica se a configuração de conexão é válida.
func (c DialConfig) validate() error {
	if c.Address == "" {
		return fmt.Errorf("address cannot be empty")
	}

	if c.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}

	return nil
}
