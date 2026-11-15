package tcp

import (
	"net"
	"testing"

	"github.com/pescador95/memorydb/server/internal/codec"
	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/protocol"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

func TestHandlerProcessesMultipleRequests(t *testing.T) {
	serverSocket, clientSocket := net.Pipe()

	messageCodec := protobuf.New()

	handler, err := NewHandler(engine.New(), messageCodec)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(serverSocket)
	}()

	connection, err := transporttcp.NewConnection(clientSocket)
	if err != nil {
		t.Fatalf("NewConnection() error = %v", err)
	}

	defer connection.Close()

	requests := []*protocol.Request{
		{
			RequestID: 1,
			Command:   &protocol.PingCommand{},
		},
		{
			RequestID: 2,
			Command: &protocol.SetCommand{
				Key:   "user:1",
				Value: []byte("memorydb"),
			},
		},
		{
			RequestID: 3,
			Command: &protocol.GetCommand{
				Key: "user:1",
			},
		},
		{
			RequestID: 4,
			Command: &protocol.DeleteCommand{
				Key: "user:1",
			},
		},
	}

	for _, request := range requests {
		requestPayload, err := messageCodec.EncodeRequest(request)
		if err != nil {
			t.Fatalf("EncodeRequest() error = %v", err)
		}

		if err := connection.WriteFrame(requestPayload); err != nil {
			t.Fatalf("WriteFrame() error = %v", err)
		}

		responsePayload, err := connection.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame() error = %v", err)
		}

		response, err := messageCodec.DecodeResponse(responsePayload)
		if err != nil {
			t.Fatalf("DecodeResponse() error = %v", err)
		}

		if response.RequestID != request.RequestID {
			t.Errorf(
				"response request ID = %d, want %d",
				response.RequestID,
				request.RequestID,
			)
		}

		if !response.Success {
			t.Errorf(
				"request %d failed: %+v",
				request.RequestID,
				response.Result,
			)
		}

		switch request.RequestID {
		case 1:
			if _, ok := response.Result.(*protocol.PongResult); !ok {
				t.Errorf("expected PONG result, got %T", response.Result)
			}

		case 2:
			if _, ok := response.Result.(*protocol.SetResult); !ok {
				t.Errorf("expected SET result, got %T", response.Result)
			}

		case 3:
			result, ok := response.Result.(*protocol.GetResult)
			if !ok {
				t.Fatalf("expected GET result, got %T", response.Result)
			}

			if !result.Found {
				t.Error("expected GET to find the key")
			}

			if string(result.Value) != "memorydb" {
				t.Errorf("GET value = %q, want %q", result.Value, "memorydb")
			}

		case 4:
			result, ok := response.Result.(*protocol.DeleteResult)
			if !ok {
				t.Fatalf("expected DELETE result, got %T", response.Result)
			}

			if !result.Deleted {
				t.Error("expected DELETE to remove the key")
			}
		}
	}

	// O encerramento do cliente deve finalizar o Handler normalmente.
	if err := connection.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := <-handlerDone; err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}
}

func TestNewHandlerRejectsNilDependencies(t *testing.T) {
	var messageCodec codec.Codec = protobuf.New()

	if _, err := NewHandler(nil, messageCodec); err == nil {
		t.Error("expected error for nil engine")
	}

	if _, err := NewHandler(engine.New(), nil); err == nil {
		t.Error("expected error for nil codec")
	}
}
