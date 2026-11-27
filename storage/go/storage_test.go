package storage

import (
	"bytes"
	"sync"
	"testing"
)

func TestSetAndGet(t *testing.T) {
	storage := New()

	if err := storage.Set("name", []byte("memorydb")); err != nil {
		t.Fatal(err)
	}

	value, found, err := storage.Get("name")
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatal("expected key to exist")
	}

	if !bytes.Equal(value, []byte("memorydb")) {
		t.Fatalf("unexpected value: %s", value)
	}
}

func TestGetMissingKey(t *testing.T) {
	storage := New()

	_, found, err := storage.Get("missing")
	if err != nil {
		t.Fatal(err)
	}

	if found {
		t.Fatal("expected key not to exist")
	}
}

func TestSetOverwritesExistingValue(t *testing.T) {
	storage := New()

	if err := storage.Set("name", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := storage.Set("name", []byte("new")); err != nil {
		t.Fatal(err)
	}

	value, found, err := storage.Get("name")
	if err != nil {
		t.Fatal(err)
	}

	if !found {
		t.Fatal("expected key to exist")
	}

	if !bytes.Equal(value, []byte("new")) {
		t.Fatalf("expected new value, got %s", value)
	}
}

func TestDelete(t *testing.T) {
	storage := New()

	if err := storage.Set("name", []byte("memorydb")); err != nil {
		t.Fatal(err)
	}

	deleted, err := storage.Delete("name")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected delete to return true")
	}

	deleted, err = storage.Delete("name")
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("expected second delete to return false")
	}
}

func TestEmptyValue(t *testing.T) {
	storage := New()

	if err := storage.Set("empty", []byte{}); err != nil {
		t.Fatal(err)
	}

	value, found, err := storage.Get("empty")
	if err != nil {
		t.Fatal(err)
	}

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
	if err := storage.Set("key", value); err != nil {
		t.Fatal(err)
	}

	value[0] = 'X'

	result, _, err := storage.Get("key")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(result, []byte("original")) {
		t.Fatalf("stored value was modified: %s", result)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	storage := New()

	if err := storage.Set("key", []byte("original")); err != nil {
		t.Fatal(err)
	}

	value, _, err := storage.Get("key")
	if err != nil {
		t.Fatal(err)
	}
	value[0] = 'X'

	result, _, err := storage.Get("key")
	if err != nil {
		t.Fatal(err)
	}

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

			if err := storage.Set(key, []byte{byte(i)}); err != nil {
				t.Errorf("Set failed: %v", err)
				return
			}

			if _, _, err := storage.Get(key); err != nil {
				t.Errorf("Get failed: %v", err)
				return
			}

			if _, err := storage.Delete(key); err != nil {
				t.Errorf("Delete failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}
