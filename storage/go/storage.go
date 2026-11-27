// Package storage fornece um mecanismo de armazenamento de dados
// em memória no formato chave-valor, com controle de concorrência
// e limites configuráveis de utilização lógica.
package storage

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidConfig       = errors.New("invalid storage configuration")
	ErrMaxEntriesReached   = errors.New("storage maximum entries reached")
	ErrMemoryBudgetReached = errors.New("storage memory budget reached")
	ErrKeyTooLarge         = errors.New("storage key too large")
	ErrValueTooLarge       = errors.New("storage value too large")
)

// LimitError representa uma rejeição causada por um limite do Storage.
type LimitError struct {
	Code    string
	Message string
	Cause   error
}

func (e *LimitError) Error() string {
	return e.Message
}

// Unwrap permite identificar a causa com errors.Is e errors.As.
func (e *LimitError) Unwrap() error {
	return e.Cause
}

// StorageErrorCode permite que consumidores identifiquem o código
// do erro sem importar o pacote concreto de Storage.
func (e *LimitError) StorageErrorCode() string {
	return e.Code
}

// Config define os limites lógicos do armazenamento.
//
// Todos os campos devem ser positivos ao utilizar NewWithConfig.
// O orçamento contabiliza somente len(key) + len(value) por entrada.
// Estruturas internas e overhead de memória do runtime não são incluídos.
type Config struct {
	MaxEntries   int64
	MemoryBudget int64
	MaxKeySize   int64
	MaxValueSize int64
}

// Storage mantém entradas chave-valor em memória.
type Storage struct {
	mu      sync.RWMutex
	entries map[string]Entry

	maxEntries   int64
	memoryBudget int64
	maxKeySize   int64
	maxValueSize int64

	usedMemory int64
}

// New cria um Storage sem limites explícitos.
//
// É mantido para compatibilidade com consumidores existentes.
// Para aplicações que precisam de limites, utilize NewWithConfig.
func New() *Storage {
	return &Storage{
		entries: make(map[string]Entry),
	}
}

// NewWithConfig cria um Storage com limites configurados.
func NewWithConfig(config Config) (*Storage, error) {
	if config.MaxEntries <= 0 {
		return nil, fmt.Errorf("%w: MaxEntries must be positive", ErrInvalidConfig)
	}
	if config.MemoryBudget <= 0 {
		return nil, fmt.Errorf("%w: MemoryBudget must be positive", ErrInvalidConfig)
	}
	if config.MaxKeySize <= 0 {
		return nil, fmt.Errorf("%w: MaxKeySize must be positive", ErrInvalidConfig)
	}
	if config.MaxValueSize <= 0 {
		return nil, fmt.Errorf("%w: MaxValueSize must be positive", ErrInvalidConfig)
	}

	return &Storage{
		entries:      make(map[string]Entry),
		maxEntries:   config.MaxEntries,
		memoryBudget: config.MemoryBudget,
		maxKeySize:   config.MaxKeySize,
		maxValueSize: config.MaxValueSize,
	}, nil
}

// validateKey aplica o limite de tamanho de chave a todas as operações.
func (s *Storage) validateKey(key string) error {
	if s.maxKeySize > 0 && int64(len(key)) > s.maxKeySize {
		return &LimitError{
			Code:    "STORAGE_KEY_TOO_LARGE",
			Message: ErrKeyTooLarge.Error(),
			Cause:   ErrKeyTooLarge,
		}
	}

	return nil
}

// Set armazena uma cópia do valor associado à chave.
//
// Uma operação rejeitada não modifica o estado atual.
func (s *Storage) Set(key string, value []byte) error {
	if err := s.validateKey(key); err != nil {
		return err
	}

	keySize := int64(len(key))
	valueSize := int64(len(value))

	if s.maxValueSize > 0 && valueSize > s.maxValueSize {
		return &LimitError{
			Code:    "STORAGE_VALUE_TOO_LARGE",
			Message: ErrValueTooLarge.Error(),
			Cause:   ErrValueTooLarge,
		}
	}

	entrySize := keySize + valueSize

	s.mu.Lock()
	defer s.mu.Unlock()

	current, exists := s.entries[key]

	if !exists && s.maxEntries > 0 && int64(len(s.entries)) >= s.maxEntries {
		return &LimitError{
			Code:    "STORAGE_MAX_ENTRIES_REACHED",
			Message: ErrMaxEntriesReached.Error(),
			Cause:   ErrMaxEntriesReached,
		}
	}

	currentSize := int64(0)
	if exists {
		currentSize = keySize + int64(len(current.value))
	}

	baseUsage := s.usedMemory - currentSize
	if baseUsage < 0 {
		return errors.New("storage memory accounting is inconsistent")
	}

	if s.memoryBudget > 0 && entrySize > s.memoryBudget-baseUsage {
		return &LimitError{
			Code:    "STORAGE_MEMORY_BUDGET_REACHED",
			Message: ErrMemoryBudgetReached.Error(),
			Cause:   ErrMemoryBudgetReached,
		}
	}

	copiedValue := append([]byte(nil), value...)

	s.entries[key] = Entry{
		value: copiedValue,
	}
	s.usedMemory = baseUsage + entrySize

	return nil
}

// Get retorna uma cópia do valor associado à chave.
//
// O segundo retorno indica se a chave existe. O terceiro comunica
// eventuais erros de validação ou acesso ao armazenamento.
func (s *Storage) Get(key string) ([]byte, bool, error) {
	if err := s.validateKey(key); err != nil {
		return nil, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, found := s.entries[key]
	if !found {
		return nil, false, nil
	}

	return append([]byte(nil), entry.value...), true, nil
}

// Delete remove a entrada e libera seu consumo lógico.
//
// O retorno indica se uma entrada foi removida.
func (s *Storage) Delete(key string) (bool, error) {
	if err := s.validateKey(key); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, found := s.entries[key]
	if !found {
		return false, nil
	}

	s.usedMemory -= int64(len(key)) + int64(len(entry.value))
	delete(s.entries, key)

	return true, nil
}

// Usage retorna o número de bytes lógicos atualmente contabilizados.
// O valor não representa o consumo real de heap ou RSS do processo.
func (s *Storage) Usage() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.usedMemory
}

// Len retorna a quantidade atual de entradas.
func (s *Storage) Len() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return int64(len(s.entries))
}
