package tcp

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/protocol"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

const (
	lifecycleTestMaxFrameSize = 1024
	lifecycleTestMemoryBudget = 4096
)

type handlerWriteSignalConn struct {
	net.Conn
	writeStarted chan struct{}
	writeOnce    sync.Once
}

func (c *handlerWriteSignalConn) Write(payload []byte) (int, error) {
	c.writeOnce.Do(func() {
		close(c.writeStarted)
	})

	return c.Conn.Write(payload)
}

func newLifecycleTestHandler(
	t *testing.T,
	messageCodec *handlerTestCodec,
) *Handler {
	t.Helper()

	handler, err := NewHandlerWithConfig(
		newTestEngine(),
		messageCodec,
		newTestIdempotencyExecutor(t),
		HandlerConfig{
			ReadTimeout:            time.Second,
			WriteTimeout:           time.Second,
			MaxInFlightRequests:    1,
			MaxFramePayloadSize:    lifecycleTestMaxFrameSize,
			ProcessingMemoryBudget: lifecycleTestMemoryBudget,
		},
	)
	if err != nil {
		t.Fatalf("NewHandlerWithConfig() error = %v", err)
	}

	return handler
}

func assertNoProcessingReservations(t *testing.T, handler *Handler) {
	t.Helper()

	if got := handler.inFlightRequestCount(); got != 0 {
		t.Errorf("in-flight requests = %d, want 0", got)
	}

	if got := handler.processingMemoryUsage(); got != 0 {
		t.Errorf("processing memory usage = %d, want 0", got)
	}
}

func TestHandlerReleasesProcessingReservationAfterProcessingErrors(t *testing.T) {
	decodeErr := errors.New("decode failure")
	encodeErr := errors.New("encode failure")

	tests := []struct {
		name                      string
		messageCodec              *handlerTestCodec
		closeClientBeforeResponse bool
		wantError                 error
	}{
		{
			name: "decode request error",
			messageCodec: &handlerTestCodec{
				decodeRequestErr: decodeErr,
			},
			wantError: decodeErr,
		},
		{
			name: "encode response error",
			messageCodec: &handlerTestCodec{
				request: &protocol.Request{
					RequestID: 1,
					Command:   &protocol.PingCommand{},
				},
				encodeResponseErr: encodeErr,
			},
			wantError: encodeErr,
		},
		{
			name: "write response error",
			messageCodec: &handlerTestCodec{
				request: &protocol.Request{
					RequestID: 1,
					Command:   &protocol.PingCommand{},
				},
			},
			closeClientBeforeResponse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newLifecycleTestHandler(t, tt.messageCodec)

			serverSocket, clientSocket := net.Pipe()

			handlerDone := make(chan error, 1)

			go func() {
				handlerDone <- handler.Handle(serverSocket)
			}()

			if err := transporttcp.WriteFrame(clientSocket, []byte{1}); err != nil {
				_ = clientSocket.Close()
				t.Fatalf("WriteFrame() error = %v", err)
			}

			if tt.closeClientBeforeResponse {
				if err := clientSocket.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			} else {
				_ = clientSocket.Close()
			}

			select {
			case err := <-handlerDone:
				if tt.wantError != nil {
					if !errors.Is(err, tt.wantError) {
						t.Errorf("Handle() error = %v, want %v", err, tt.wantError)
					}
				} else if err == nil {
					t.Error("Handle() error = nil, want write failure")
				}

			case <-time.After(2 * time.Second):
				t.Fatal("Handler did not finish after processing error")
			}

			assertNoProcessingReservations(t, handler)
		})
	}
}

func TestHandlerKeepsProcessingReservationUntilResponseWriteCompletes(t *testing.T) {
	handler := newLifecycleTestHandler(
		t,
		&handlerTestCodec{
			request: &protocol.Request{
				RequestID: 1,
				Command:   &protocol.PingCommand{},
			},
		},
	)

	serverSocket, clientSocket := net.Pipe()

	writeStarted := make(chan struct{})

	signaledSocket := &handlerWriteSignalConn{
		Conn:         serverSocket,
		writeStarted: writeStarted,
	}

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(signaledSocket)
	}()

	if err := transporttcp.WriteFrame(clientSocket, []byte{1}); err != nil {
		_ = clientSocket.Close()
		t.Fatalf("WriteFrame() error = %v", err)
	}

	// Aguarda o Handler entrar efetivamente na escrita da resposta.
	// Como o cliente ainda não está lendo, net.Pipe mantém a escrita bloqueada.
	select {
	case <-writeStarted:
	case <-time.After(2 * time.Second):
		_ = clientSocket.Close()
		t.Fatal("Handler did not start writing the response")
	}

	if got := handler.inFlightRequestCount(); got != 1 {
		_ = clientSocket.Close()
		t.Fatalf("in-flight requests during blocked write = %d, want 1", got)
	}

	if got := handler.processingMemoryUsage(); got <= 0 {
		_ = clientSocket.Close()
		t.Fatalf("processing memory usage during blocked write = %d, want > 0", got)
	}

	// A leitura da resposta permite que a escrita do Handler termine.
	responsePayload, err := transporttcp.ReadFrame(clientSocket)
	if err != nil {
		_ = clientSocket.Close()
		t.Fatalf("ReadFrame() error = %v", err)
	}

	if len(responsePayload) == 0 {
		_ = clientSocket.Close()
		t.Fatal("response payload is empty")
	}

	// Fechar o cliente faz o Handler receber EOF na próxima leitura.
	if err := clientSocket.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-handlerDone:
		if err != nil {
			t.Fatalf("Handle() error = %v, want nil", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Handler did not finish after client close")
	}

	assertNoProcessingReservations(t, handler)
}
