package storage

import (
	"errors"
	"testing"
)

func testStorageConfig() Config {
	return Config{
		MaxEntries:   2,
		MemoryBudget: 16,
		MaxKeySize:   8,
		MaxValueSize: 8,
	}
}

func TestNewWithConfigRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "zero max entries",
			config: Config{
				MemoryBudget: 16,
				MaxKeySize:   8,
				MaxValueSize: 8,
			},
		},
		{
			name: "zero memory budget",
			config: Config{
				MaxEntries:   2,
				MaxKeySize:   8,
				MaxValueSize: 8,
			},
		},
		{
			name: "zero max key size",
			config: Config{
				MaxEntries:   2,
				MemoryBudget: 16,
				MaxValueSize: 8,
			},
		},
		{
			name: "zero max value size",
			config: Config{
				MaxEntries:   2,
				MemoryBudget: 16,
				MaxKeySize:   8,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWithConfig(tt.config)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestSetEnforcesMaxEntries(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.Set("one", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Set("two", []byte("2")); err != nil {
		t.Fatal(err)
	}

	err = storage.Set("three", []byte("3"))
	if !errors.Is(err, ErrMaxEntriesReached) {
		t.Fatalf("expected ErrMaxEntriesReached, got %v", err)
	}

	if storage.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", storage.Len())
	}
}

func TestSetEnforcesMemoryBudget(t *testing.T) {
	config := testStorageConfig()
	config.MaxValueSize = 16

	storage, err := NewWithConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.Set("key", []byte("123456789")); err != nil {
		t.Fatal(err)
	}

	err = storage.Set("another", []byte("value"))
	if !errors.Is(err, ErrMemoryBudgetReached) {
		t.Fatalf("expected ErrMemoryBudgetReached, got %v", err)
	}

	expectedUsage := int64(len("key") + len("123456789"))
	if storage.Usage() != expectedUsage {
		t.Fatalf("expected usage %d, got %d", expectedUsage, storage.Usage())
	}
}

func TestSetEnforcesKeySize(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	err = storage.Set("123456789", []byte("value"))
	if !errors.Is(err, ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge, got %v", err)
	}

	if storage.Len() != 0 {
		t.Fatalf("expected no entries, got %d", storage.Len())
	}
}

func TestGetEnforcesKeySize(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	value, found, err := storage.Get("123456789")
	if !errors.Is(err, ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge, got %v", err)
	}

	if found {
		t.Fatal("expected key not to be found")
	}
	if value != nil {
		t.Fatalf("expected nil value, got %v", value)
	}
}

func TestDeleteEnforcesKeySize(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := storage.Delete("123456789")
	if !errors.Is(err, ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge, got %v", err)
	}

	if deleted {
		t.Fatal("expected no entry to be deleted")
	}

	if storage.Len() != 0 {
		t.Fatalf("expected no entries, got %d", storage.Len())
	}
}

func TestSetEnforcesValueSize(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	err = storage.Set("key", []byte("123456789"))
	if !errors.Is(err, ErrValueTooLarge) {
		t.Fatalf("expected ErrValueTooLarge, got %v", err)
	}

	if storage.Len() != 0 {
		t.Fatalf("expected no entries, got %d", storage.Len())
	}
}

func TestSetOverwriteRecalculatesUsage(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.Set("key", []byte("123456")); err != nil {
		t.Fatal(err)
	}

	if storage.Usage() != 9 {
		t.Fatalf("expected usage 9, got %d", storage.Usage())
	}

	if err := storage.Set("key", []byte("12")); err != nil {
		t.Fatal(err)
	}

	if storage.Usage() != 5 {
		t.Fatalf("expected usage 5 after smaller overwrite, got %d", storage.Usage())
	}

	value, found, err := storage.Get("key")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected key to exist")
	}
	if string(value) != "12" {
		t.Fatalf("expected updated value, got %q", value)
	}
}

func TestRejectedOverwritePreservesPreviousValueAndUsage(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.Set("key", []byte("original")); err != nil {
		t.Fatal(err)
	}

	previousUsage := storage.Usage()

	err = storage.Set("key", []byte("123456789"))
	if !errors.Is(err, ErrValueTooLarge) {
		t.Fatalf("expected ErrValueTooLarge, got %v", err)
	}

	value, found, err := storage.Get("key")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected original key to remain")
	}
	if string(value) != "original" {
		t.Fatalf("expected original value, got %q", value)
	}
	if storage.Usage() != previousUsage {
		t.Fatalf("expected usage %d, got %d", previousUsage, storage.Usage())
	}
}

func TestDeleteReleasesUsage(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.Set("key", []byte("value")); err != nil {
		t.Fatal(err)
	}

	if storage.Usage() != 8 {
		t.Fatalf("expected usage 8, got %d", storage.Usage())
	}

	deleted, err := storage.Delete("key")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected key to be deleted")
	}

	if storage.Usage() != 0 {
		t.Fatalf("expected usage 0 after delete, got %d", storage.Usage())
	}

	if err := storage.Set("other", []byte("value")); err != nil {
		t.Fatalf("expected capacity to be available after delete: %v", err)
	}
}

func TestStorageLimitErrorExposesCode(t *testing.T) {
	storage, err := NewWithConfig(testStorageConfig())
	if err != nil {
		t.Fatal(err)
	}

	err = storage.Set("123456789", []byte("value"))
	if err == nil {
		t.Fatal("expected key size error")
	}

	var coded interface {
		StorageErrorCode() string
	}
	if !errors.As(err, &coded) {
		t.Fatalf("expected error to expose StorageErrorCode, got %T", err)
	}

	if coded.StorageErrorCode() != "STORAGE_KEY_TOO_LARGE" {
		t.Fatalf("unexpected error code: %s", coded.StorageErrorCode())
	}
}
