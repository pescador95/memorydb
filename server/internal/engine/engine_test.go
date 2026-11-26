package engine

import (
	"testing"

	"github.com/pescador95/memorydb/server/internal/protocol"
	memorystorage "github.com/pescador95/memorydb/storage/go"
)

func newTestEngine() *Engine {
	return New(memorystorage.New())
}

func TestPing(t *testing.T) {
	engine := newTestEngine()

	response := engine.Execute(&protocol.Request{
		RequestID: 1,
		Command:   &protocol.PingCommand{},
	})

	if !response.Success {
		t.Fatal("expected successful response")
	}

	if response.RequestID != 1 {
		t.Fatal("request ID was not preserved")
	}

	if _, ok := response.Result.(*protocol.PongResult); !ok {
		t.Fatalf("expected PongResult, got %T", response.Result)
	}
}

func TestSetAndGet(t *testing.T) {
	engine := newTestEngine()

	setResponse := engine.Execute(&protocol.Request{
		RequestID: 2,
		Command: &protocol.SetCommand{
			Key:   "name",
			Value: []byte("memorydb"),
		},
	})

	if !setResponse.Success {
		t.Fatal("expected successful SET")
	}

	getResponse := engine.Execute(&protocol.Request{
		RequestID: 3,
		Command: &protocol.GetCommand{
			Key: "name",
		},
	})

	result, ok := getResponse.Result.(*protocol.GetResult)
	if !ok {
		t.Fatalf("expected GetResult, got %T", getResponse.Result)
	}

	if !result.Found {
		t.Fatal("expected key to be found")
	}

	if string(result.Value) != "memorydb" {
		t.Fatalf("unexpected value: %s", result.Value)
	}
}

func TestGetMissingKey(t *testing.T) {
	engine := newTestEngine()

	response := engine.Execute(&protocol.Request{
		Command: &protocol.GetCommand{
			Key: "missing",
		},
	})

	result, ok := response.Result.(*protocol.GetResult)
	if !ok {
		t.Fatalf("expected GetResult, got %T", response.Result)
	}

	if result.Found {
		t.Fatal("expected key not to be found")
	}
}

func TestDelete(t *testing.T) {
	engine := newTestEngine()

	engine.Execute(&protocol.Request{
		Command: &protocol.SetCommand{
			Key:   "name",
			Value: []byte("memorydb"),
		},
	})

	response := engine.Execute(&protocol.Request{
		Command: &protocol.DeleteCommand{
			Key: "name",
		},
	})

	result, ok := response.Result.(*protocol.DeleteResult)
	if !ok {
		t.Fatalf("expected DeleteResult, got %T", response.Result)
	}

	if !result.Deleted {
		t.Fatal("expected key to be deleted")
	}
}

func TestRejectsEmptyKey(t *testing.T) {
	engine := newTestEngine()

	response := engine.Execute(&protocol.Request{
		RequestID: 10,
		Command:   &protocol.SetCommand{},
	})

	if response.Success {
		t.Fatal("expected request to fail")
	}

	result, ok := response.Result.(*protocol.ErrorResult)
	if !ok {
		t.Fatalf("expected ErrorResult, got %T", response.Result)
	}

	if result.Code != ErrInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT, got %s", result.Code)
	}
}

func TestRejectsUnsupportedTTL(t *testing.T) {
	engine := newTestEngine()

	response := engine.Execute(&protocol.Request{
		Command: &protocol.SetCommand{
			Key:       "name",
			Value:     []byte("memorydb"),
			TTLMillis: 5000,
		},
	})

	if response.Success {
		t.Fatal("expected request to fail")
	}

	result, ok := response.Result.(*protocol.ErrorResult)
	if !ok {
		t.Fatalf("expected ErrorResult, got %T", response.Result)
	}

	if result.Code != ErrUnsupportedFeature {
		t.Fatalf("expected UNSUPPORTED_FEATURE, got %s", result.Code)
	}
}

func TestRejectsNilRequest(t *testing.T) {
	engine := newTestEngine()

	response := engine.Execute(nil)

	if response.Success {
		t.Fatal("expected request to fail")
	}

	result, ok := response.Result.(*protocol.ErrorResult)
	if !ok {
		t.Fatalf("expected ErrorResult, got %T", response.Result)
	}

	if result.Code != ErrInvalidRequest {
		t.Fatalf("expected INVALID_REQUEST, got %s", result.Code)
	}
}

func TestRejectsMissingCommand(t *testing.T) {
	engine := newTestEngine()

	response := engine.Execute(&protocol.Request{
		RequestID: 20,
	})

	if response.Success {
		t.Fatal("expected request to fail")
	}

	result, ok := response.Result.(*protocol.ErrorResult)
	if !ok {
		t.Fatalf("expected ErrorResult, got %T", response.Result)
	}

	if result.Code != ErrInvalidRequest {
		t.Fatalf("expected INVALID_REQUEST, got %s", result.Code)
	}
}

func TestRejectsNilCommands(t *testing.T) {
	engine := newTestEngine()

	tests := []struct {
		name    string
		command protocol.Command
	}{
		{
			name:    "PING",
			command: (*protocol.PingCommand)(nil),
		},
		{
			name:    "SET",
			command: (*protocol.SetCommand)(nil),
		},
		{
			name:    "GET",
			command: (*protocol.GetCommand)(nil),
		},
		{
			name:    "DELETE",
			command: (*protocol.DeleteCommand)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := engine.Execute(&protocol.Request{
				RequestID: 30,
				Command:   tt.command,
			})

			if response.Success {
				t.Fatal("expected request to fail")
			}

			if response.RequestID != 30 {
				t.Fatalf("expected request ID 30, got %d", response.RequestID)
			}

			result, ok := response.Result.(*protocol.ErrorResult)
			if !ok {
				t.Fatalf("expected ErrorResult, got %T", response.Result)
			}

			if result.Code != ErrInvalidRequest {
				t.Fatalf("expected INVALID_REQUEST, got %s", result.Code)
			}
		})
	}
}

func TestRejectsEmptyKeyForGetAndDelete(t *testing.T) {
	engine := newTestEngine()

	tests := []struct {
		name    string
		command protocol.Command
	}{
		{
			name:    "GET",
			command: &protocol.GetCommand{},
		},
		{
			name:    "DELETE",
			command: &protocol.DeleteCommand{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := engine.Execute(&protocol.Request{
				RequestID: 40,
				Command:   tt.command,
			})

			if response.Success {
				t.Fatal("expected request to fail")
			}

			if response.RequestID != 40 {
				t.Fatalf("expected request ID 40, got %d", response.RequestID)
			}

			result, ok := response.Result.(*protocol.ErrorResult)
			if !ok {
				t.Fatalf("expected ErrorResult, got %T", response.Result)
			}

			if result.Code != ErrInvalidArgument {
				t.Fatalf("expected INVALID_ARGUMENT, got %s", result.Code)
			}
		})
	}
}
