// Package storage fornece um mecanismo de armazenamento de dados
// em memória no formato chave-valor, com controle de concorrência.

package storage

import "sync"

type Storage struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

func New() *Storage {
	return &Storage{
		entries: make(map[string]Entry),
	}
}

func (s *Storage) Set(key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[key] = Entry{
		value: append([]byte(nil), value...),
	}
}

func (s *Storage) Get(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, found := s.entries[key]

	if !found {
		return nil, false
	}
	return append([]byte(nil), entry.value...), true
}

func (s *Storage) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, found := s.entries[key]; !found {
		return false
	}

	delete(s.entries, key)
	return true
}
