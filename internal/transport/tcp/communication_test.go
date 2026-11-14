package tcp

import (
	"net"
	"testing"

	"github.com/pescador95/memorydb/internal/codec/protobuf"
	"github.com/pescador95/memorydb/internal/protocol"
)

func TestRequestCommunication(t *testing.T) {
	clientSocket, serverSocket := net.Pipe()
	defer clientSocket.Close()
	defer serverSocket.Close()

	messageCodec := protobuf.New()

	client, err := NewConnection(clientSocket, messageCodec)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewConnection(serverSocket, messageCodec)
	if err != nil {
		t.Fatal(err)
	}

	original := &protocol.Request{
		RequestID: 42,
		Command:   &protocol.PingCommand{},
	}

	writeErr := make(chan error, 1)

	go func() {
		writeErr <- client.WriteRequest(original)
	}()

	received, err := server.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest() error = %v", err)
	}

	if err := <-writeErr; err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}

	if received.RequestID != original.RequestID {
		t.Errorf("request ID mismatch: got %d, want %d",
			received.RequestID, original.RequestID)
	}

	if _, ok := received.Command.(*protocol.PingCommand); !ok {
		t.Fatalf("expected PING command, got %T", received.Command)
	}
}

func TestResponseCommunication(t *testing.T) {
	clientSocket, serverSocket := net.Pipe()
	defer clientSocket.Close()
	defer serverSocket.Close()

	messageCodec := protobuf.New()

	client, err := NewConnection(clientSocket, messageCodec)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewConnection(serverSocket, messageCodec)
	if err != nil {
		t.Fatal(err)
	}

	original := &protocol.Response{
		RequestID: 42,
		Success:   true,
		Result:    &protocol.PongResult{},
	}

	writeErr := make(chan error, 1)

	go func() {
		writeErr <- server.WriteResponse(original)
	}()

	received, err := client.ReadResponse()
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}

	if err := <-writeErr; err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}

	if received.RequestID != original.RequestID {
		t.Errorf("request ID mismatch: got %d, want %d",
			received.RequestID, original.RequestID)
	}

	if !received.Success {
		t.Error("expected successful response")
	}

	if _, ok := received.Result.(*protocol.PongResult); !ok {
		t.Fatalf("expected PONG result, got %T", received.Result)
	}
}
