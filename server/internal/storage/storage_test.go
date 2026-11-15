package storage

import (
	"bytes"
	"sync"
	"testing"
)

func TestSetAndGet(t *testing.T) {
	storage := New()

	storage.Set("name", []byte("memorydb"))

	value, found := storage.Get("name")

	if !found {
		t.Fatal("expected key to exist")
	}

	if !bytes.Equal(value, []byte("memorydb")) {
		t.Fatalf("unexpected value: %s", value)
	}
}

func TestGetMissingKey(t *testing.T) {
	storage := New()

	_, found := storage.Get("missing")

	if found {
		t.Fatal("expected key not to exist")
	}
}

func TestSetOverwritesExistingValue(t *testing.T) {
	storage := New()

	storage.Set("name", []byte("old"))
	storage.Set("name", []byte("new"))

	value, found := storage.Get("name")

	if !found {
		t.Fatal("expected key to exist")
	}

	if !bytes.Equal(value, []byte("new")) {
		t.Fatalf("expected new value, got %s", value)
	}
}

func TestDelete(t *testing.T) {
	storage := New()

	storage.Set("name", []byte("memorydb"))

	if !storage.Delete("name") {
		t.Fatal("expected delete to return true")
	}

	if storage.Delete("name") {
		t.Fatal("expected second delete to return false")
	}
}

func TestEmptyValue(t *testing.T) {
	storage := New()

	storage.Set("empty", []byte{})

	value, found := storage.Get("empty")

	if !found {
		t.Fatal("expected empty key value to exist")
	}

	if len(value) != 0 {
		t.Fatalf("expected empty value, got %v", value)
	}
}

func TestSetCopiesInputValue(t *testing.T) {
	storage := New()

	value := []byte("original")
	storage.Set("key", value)

	value[0] = 'X'

	result, _ := storage.Get("key")

	if !bytes.Equal(result, []byte("original")) {
		t.Fatalf("stored value was modified: %s", result)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	storage := New()

	storage.Set("key", []byte("original"))

	value, _ := storage.Get("key")
	value[0] = 'X'

	result, _ := storage.Get("key")

	if !bytes.Equal(result, []byte("original")) {
		t.Fatalf("stored value was modified: %s", result)
	}
}

func TestConcurrentAccess(t *testing.T) {
	storage := New()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			key := "key"

			storage.Set(key, []byte{byte(i)})
			storage.Get(key)
			storage.Delete(key)
		}(i)
	}

	wg.Wait()
}
