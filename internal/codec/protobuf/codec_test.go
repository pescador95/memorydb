package protobuf

import (
	"bytes"
	"testing"

	"github.com/pescador95/memorydb/internal/protocol"
)

func TestRequestRoundTrip(t *testing.T) {
	codec := New()

	tests := []struct {
		name    string
		command protocol.Command
	}{
		{
			name:    "PING",
			command: &protocol.PingCommand{},
		},
		{
			name: "SET",
			command: &protocol.SetCommand{
				Key:       "user:1",
				Value:     []byte{0x00, 0x01, 0xFF, 0xFE},
				TTLMillis: 5000,
			},
		},
		{
			name: "GET",
			command: &protocol.GetCommand{
				Key: "user:1",
			},
		},
		{
			name: "DELETE",
			command: &protocol.DeleteCommand{
				Key: "user:1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &protocol.Request{
				RequestID: 42,
				Command:   tt.command,
			}

			data, err := codec.EncodeRequest(request)
			if err != nil {
				t.Fatalf("EncodeRequest() error = %v", err)
			}

			got, err := codec.DecodeRequest(data)
			if err != nil {
				t.Fatalf("DecodeRequest() error = %v", err)
			}

			if got.RequestID != request.RequestID {
				t.Errorf("RequestID = %d, want %d", got.RequestID, request.RequestID)
			}

			switch want := tt.command.(type) {
			case *protocol.PingCommand:
				if _, ok := got.Command.(*protocol.PingCommand); !ok {
					t.Errorf("Command type = %T, want *protocol.PingCommand", got.Command)
				}
			case *protocol.SetCommand:
				gotCommand, ok := got.Command.(*protocol.SetCommand)
				if !ok {
					t.Fatalf("Command type = %T, want *protocol.SetCommand", got.Command)
				}
				if gotCommand.Key != want.Key ||
					gotCommand.TTLMillis != want.TTLMillis ||
					!bytes.Equal(gotCommand.Value, want.Value) {
					t.Errorf("SET command mismatch: got %+v, want %+v", gotCommand, want)
				}
			case *protocol.GetCommand:
				gotCommand, ok := got.Command.(*protocol.GetCommand)
				if !ok || gotCommand.Key != want.Key {
					t.Errorf("GET command mismatch: got %+v, want %+v", got.Command, want)
				}
			case *protocol.DeleteCommand:
				gotCommand, ok := got.Command.(*protocol.DeleteCommand)
				if !ok || gotCommand.Key != want.Key {
					t.Errorf("DELETE command mismatch: got %+v, want %+v", got.Command, want)
				}
			}
		})
	}
}

func TestResponseRoundTrip(t *testing.T) {
	codec := New()

	tests := []struct {
		name   string
		result protocol.Result
	}{
		{name: "PONG", result: &protocol.PongResult{}},
		{name: "SET", result: &protocol.SetResult{}},
		{
			name: "GET",
			result: &protocol.GetResult{
				Found: true,
				Value: []byte{0x00, 0x01, 0xFF},
			},
		},
		{
			name: "GET missing",
			result: &protocol.GetResult{
				Found: false,
				Value: nil,
			},
		},
		{
			name:   "DELETE",
			result: &protocol.DeleteResult{Deleted: true},
		},
		{
			name: "ERROR",
			result: &protocol.ErrorResult{
				Code:    "INVALID_ARGUMENT",
				Message: "key cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &protocol.Response{
				RequestID: 42,
				Success:   true,
				Result:    tt.result,
			}

			if _, ok := tt.result.(*protocol.ErrorResult); ok {
				response.Success = false
			}

			data, err := codec.EncodeResponse(response)
			if err != nil {
				t.Fatalf("EncodeResponse() error = %v", err)
			}

			got, err := codec.DecodeResponse(data)
			if err != nil {
				t.Fatalf("DecodeResponse() error = %v", err)
			}

			if got.RequestID != response.RequestID {
				t.Errorf("RequestID = %d, want %d", got.RequestID, response.RequestID)
			}

			if got.Success != response.Success {
				t.Errorf("Success = %t, want %t", got.Success, response.Success)
			}

			switch want := tt.result.(type) {
			case *protocol.PongResult:
				if _, ok := got.Result.(*protocol.PongResult); !ok {
					t.Errorf("Result type = %T, want *protocol.PongResult", got.Result)
				}
			case *protocol.SetResult:
				if _, ok := got.Result.(*protocol.SetResult); !ok {
					t.Errorf("Result type = %T, want *protocol.SetResult", got.Result)
				}
			case *protocol.GetResult:
				gotResult, ok := got.Result.(*protocol.GetResult)
				if !ok {
					t.Fatalf("Result type = %T, want *protocol.GetResult", got.Result)
				}
				if gotResult.Found != want.Found || !bytes.Equal(gotResult.Value, want.Value) {
					t.Errorf("GET result mismatch: got %+v, want %+v", gotResult, want)
				}
			case *protocol.DeleteResult:
				gotResult, ok := got.Result.(*protocol.DeleteResult)
				if !ok || gotResult.Deleted != want.Deleted {
					t.Errorf("DELETE result mismatch: got %+v, want %+v", got.Result, want)
				}
			case *protocol.ErrorResult:
				gotResult, ok := got.Result.(*protocol.ErrorResult)
				if !ok {
					t.Fatalf("Result type = %T, want *protocol.ErrorResult", got.Result)
				}
				if gotResult.Code != want.Code || gotResult.Message != want.Message {
					t.Errorf("ERROR result mismatch: got %+v, want %+v", gotResult, want)
				}
			}
		})
	}
}

func TestEncodeRejectsNilMessages(t *testing.T) {
	codec := New()

	if _, err := codec.EncodeRequest(nil); err == nil {
		t.Error("EncodeRequest() expected error for nil request")
	}

	if _, err := codec.EncodeResponse(nil); err == nil {
		t.Error("EncodeResponse() expected error for nil response")
	}
}

func TestEncodeRejectsMissingCommandOrResult(t *testing.T) {
	codec := New()

	if _, err := codec.EncodeRequest(&protocol.Request{}); err == nil {
		t.Error("EncodeRequest() expected error for missing command")
	}

	if _, err := codec.EncodeResponse(&protocol.Response{}); err == nil {
		t.Error("EncodeResponse() expected error for missing result")
	}
}

func TestDecodeRejectsInvalidProtobuf(t *testing.T) {
	codec := New()

	invalidData := []byte{0xFF, 0xFF, 0xFF}

	if _, err := codec.DecodeRequest(invalidData); err == nil {
		t.Error("DecodeRequest() expected error for invalid Protobuf")
	}

	if _, err := codec.DecodeResponse(invalidData); err == nil {
		t.Error("DecodeResponse() expected error for invalid Protobuf")
	}
}
