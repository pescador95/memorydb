package tcp

import (
	"net"
	"testing"

	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/protocol"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

func TestRequestCommunication(t *testing.T) {
	clientSocket, serverSocket := net.Pipe()
	defer clientSocket.Close()
	defer serverSocket.Close()

	messageCodec := protobuf.New()

	client, err := transporttcp.NewConnection(clientSocket)
	if err != nil {
		t.Fatal(err)
	}

	server, err := transporttcp.NewConnection(serverSocket)
	if err != nil {
		t.Fatal(err)
	}

	original := &protocol.Request{
		RequestID: 42,
		Command:   &protocol.PingCommand{},
	}

	payload, err := messageCodec.EncodeRequest(original)
	if err != nil {
		t.Fatalf("EncodeRequest() error = %v", err)
	}

	writeErr := make(chan error, 1)

	go func() {
		writeErr <- client.WriteFrame(payload)
	}()

	receivedPayload, err := server.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	if err := <-writeErr; err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	received, err := messageCodec.DecodeRequest(receivedPayload)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
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

	client, err := transporttcp.NewConnection(clientSocket)
	if err != nil {
		t.Fatal(err)
	}

	server, err := transporttcp.NewConnection(serverSocket)
	if err != nil {
		t.Fatal(err)
	}

	original := &protocol.Response{
		RequestID: 42,
		Success:   true,
		Result:    &protocol.PongResult{},
	}

	payload, err := messageCodec.EncodeResponse(original)
	if err != nil {
		t.Fatalf("EncodeResponse() error = %v", err)
	}

	writeErr := make(chan error, 1)

	go func() {
		writeErr <- server.WriteFrame(payload)
	}()

	receivedPayload, err := client.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	if err := <-writeErr; err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	received, err := messageCodec.DecodeResponse(receivedPayload)
	if err != nil {
		t.Fatalf("DecodeResponse() error = %v", err)
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
