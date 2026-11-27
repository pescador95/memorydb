package engine

import (
	"testing"

	"github.com/pescador95/memorydb/server/internal/protocol"
	memorystorage "github.com/pescador95/memorydb/storage/go"
)

func newLimitedTestEngine(t *testing.T, config memorystorage.Config) *Engine {
	t.Helper()

	storage, err := memorystorage.NewWithConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	return New(storage)
}

func assertStorageErrorResponse(
	t *testing.T,
	response *protocol.Response,
	requestID uint64,
	expectedCode string,
) {
	t.Helper()

	if response.Success {
		t.Fatal("expected operation to fail")
	}

	if response.RequestID != requestID {
		t.Fatalf("expected request ID %d, got %d", requestID, response.RequestID)
	}

	result, ok := response.Result.(*protocol.ErrorResult)
	if !ok {
		t.Fatalf("expected ErrorResult, got %T", response.Result)
	}

	if result.Code != expectedCode {
		t.Fatalf("expected error code %s, got %s", expectedCode, result.Code)
	}

	if result.Message == "" {
		t.Fatal("expected error message")
	}
}

func TestSetReturnsStorageLimitError(t *testing.T) {
	engine := newLimitedTestEngine(t, memorystorage.Config{
		MaxEntries:   1,
		MemoryBudget: 16,
		MaxKeySize:   4,
		MaxValueSize: 8,
	})

	first := engine.Execute(&protocol.Request{
		RequestID: 1,
		Command: &protocol.SetCommand{
			Key:   "one",
			Value: []byte("value"),
		},
	})
	if !first.Success {
		t.Fatalf("expected first SET to succeed, got %#v", first.Result)
	}

	second := engine.Execute(&protocol.Request{
		RequestID: 2,
		Command: &protocol.SetCommand{
			Key:   "two",
			Value: []byte("value"),
		},
	})

	assertStorageErrorResponse(
		t,
		second,
		2,
		"STORAGE_MAX_ENTRIES_REACHED",
	)
}

func TestSetReturnsStorageKeySizeError(t *testing.T) {
	engine := newLimitedTestEngine(t, memorystorage.Config{
		MaxEntries:   2,
		MemoryBudget: 16,
		MaxKeySize:   3,
		MaxValueSize: 8,
	})

	response := engine.Execute(&protocol.Request{
		RequestID: 3,
		Command: &protocol.SetCommand{
			Key:   "long",
			Value: []byte("value"),
		},
	})

	assertStorageErrorResponse(
		t,
		response,
		3,
		"STORAGE_KEY_TOO_LARGE",
	)
}

func TestSetReturnsStorageValueSizeError(t *testing.T) {
	engine := newLimitedTestEngine(t, memorystorage.Config{
		MaxEntries:   2,
		MemoryBudget: 16,
		MaxKeySize:   8,
		MaxValueSize: 3,
	})

	response := engine.Execute(&protocol.Request{
		RequestID: 4,
		Command: &protocol.SetCommand{
			Key:   "key",
			Value: []byte("large"),
		},
	})

	assertStorageErrorResponse(
		t,
		response,
		4,
		"STORAGE_VALUE_TOO_LARGE",
	)
}

func TestGetReturnsStorageKeySizeError(t *testing.T) {
	engine := newLimitedTestEngine(t, memorystorage.Config{
		MaxEntries:   2,
		MemoryBudget: 16,
		MaxKeySize:   3,
		MaxValueSize: 8,
	})

	response := engine.Execute(&protocol.Request{
		RequestID: 5,
		Command: &protocol.GetCommand{
			Key: "long",
		},
	})

	assertStorageErrorResponse(
		t,
		response,
		5,
		"STORAGE_KEY_TOO_LARGE",
	)
}

func TestDeleteReturnsStorageKeySizeError(t *testing.T) {
	engine := newLimitedTestEngine(t, memorystorage.Config{
		MaxEntries:   2,
		MemoryBudget: 16,
		MaxKeySize:   3,
		MaxValueSize: 8,
	})

	response := engine.Execute(&protocol.Request{
		RequestID: 6,
		Command: &protocol.DeleteCommand{
			Key: "long",
		},
	})

	assertStorageErrorResponse(
		t,
		response,
		6,
		"STORAGE_KEY_TOO_LARGE",
	)
}
