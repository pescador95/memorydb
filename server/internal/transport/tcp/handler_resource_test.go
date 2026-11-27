package tcp

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/codec/protobuf"
	"github.com/pescador95/memorydb/server/internal/engine"
	"github.com/pescador95/memorydb/server/internal/idempotency"
	memorystorage "github.com/pescador95/memorydb/storage/go"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

func newResourceTestHandler(
	t *testing.T,
	config HandlerConfig,
) *Handler {
	t.Helper()

	storage := memorystorage.New()
	commandEngine := engine.New(storage)
	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("create idempotency executor: %v", err)
	}

	handler, err := NewHandlerWithConfig(
		commandEngine,
		protobuf.New(),
		executor,
		HandlerConfig{
			ReadTimeout:            time.Second,
			WriteTimeout:           time.Second,
			MaxInFlightRequests:    config.MaxInFlightRequests,
			MaxFramePayloadSize:    config.MaxFramePayloadSize,
			ProcessingMemoryBudget: config.ProcessingMemoryBudget,
		},
	)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}

	return handler
}

func TestNewHandlerWithConfigAppliesResourceDefaults(t *testing.T) {
	handler := newResourceTestHandler(t, HandlerConfig{})

	if handler.maxInFlightRequests != defaultMaxInFlightRequests {
		t.Fatalf(
			"unexpected maximum in-flight requests: got %d, want %d",
			handler.maxInFlightRequests,
			defaultMaxInFlightRequests,
		)
	}

	if handler.maxFramePayloadSize != transporttcp.MaxFrameSize {
		t.Fatalf(
			"unexpected maximum frame size: got %d, want %d",
			handler.maxFramePayloadSize,
			transporttcp.MaxFrameSize,
		)
	}

	if handler.processingMemoryBudget != 0 {
		t.Fatalf(
			"unexpected processing memory budget: got %d, want 0",
			handler.processingMemoryBudget,
		)
	}
}

func TestNewHandlerWithConfigRejectsInvalidResourceLimits(t *testing.T) {
	tests := []struct {
		name   string
		config HandlerConfig
	}{
		{
			name: "negative in-flight requests",
			config: HandlerConfig{
				MaxInFlightRequests: -1,
			},
		},
		{
			name: "negative frame payload size",
			config: HandlerConfig{
				MaxFramePayloadSize: -1,
			},
		},
		{
			name: "frame payload exceeds transport maximum",
			config: HandlerConfig{
				MaxFramePayloadSize: int64(transporttcp.MaxFrameSize) + 1,
			},
		},
		{
			name: "negative processing memory budget",
			config: HandlerConfig{
				ProcessingMemoryBudget: -1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newResourceTestHandlerResult(t, test.config)
			if err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func newResourceTestHandlerResult(
	t *testing.T,
	config HandlerConfig,
) (*Handler, error) {
	t.Helper()

	storage := memorystorage.New()
	commandEngine := engine.New(storage)
	executor, err := idempotency.NewExecutor(&idempotency.Config{})
	if err != nil {
		t.Fatalf("create idempotency executor: %v", err)
	}

	return NewHandlerWithConfig(
		commandEngine,
		protobuf.New(),
		executor,
		HandlerConfig{
			ReadTimeout:            time.Second,
			WriteTimeout:           time.Second,
			MaxInFlightRequests:    config.MaxInFlightRequests,
			MaxFramePayloadSize:    config.MaxFramePayloadSize,
			ProcessingMemoryBudget: config.ProcessingMemoryBudget,
		},
	)
}

func TestNewHandlerWithConfigDoesNotAllocateByInFlightLimit(t *testing.T) {
	maxInFlightRequests := int(^uint(0) >> 1)

	handler := newResourceTestHandler(t, HandlerConfig{
		MaxInFlightRequests: maxInFlightRequests,
		MaxFramePayloadSize: 16,
	})

	if handler.maxInFlightRequests != maxInFlightRequests {
		t.Fatalf(
			"unexpected maximum in-flight requests: got %d, want %d",
			handler.maxInFlightRequests,
			maxInFlightRequests,
		)
	}
}

func TestReserveProcessingEnforcesMaxInFlightRequests(t *testing.T) {
	handler := newResourceTestHandler(t, HandlerConfig{
		MaxInFlightRequests: 1,
		MaxFramePayloadSize: 16,
	})

	release, err := handler.reserveProcessing(1)
	if err != nil {
		t.Fatalf("reserve first request: %v", err)
	}

	_, err = handler.reserveProcessing(1)
	if !errors.Is(err, ErrMaxInFlightRequestsReached) {
		t.Fatalf(
			"expected max in-flight error, got %v",
			err,
		)
	}

	release()

	if got := handler.inFlightRequestCount(); got != 0 {
		t.Fatalf(
			"expected no active requests after release, got %d",
			got,
		)
	}

	if got := handler.processingMemoryUsage(); got != 0 {
		t.Fatalf("expected zero memory usage after release, got %d", got)
	}
}

func TestReserveProcessingEnforcesMemoryBudget(t *testing.T) {
	// A reserva é frameSize + 2*maxFramePayloadSize:
	// 1 + 2*16 = 33 bytes.
	handler := newResourceTestHandler(t, HandlerConfig{
		MaxInFlightRequests:    2,
		MaxFramePayloadSize:    16,
		ProcessingMemoryBudget: 32,
	})

	_, err := handler.reserveProcessing(1)
	if !errors.Is(err, ErrProcessingMemoryBudgetReached) {
		t.Fatalf(
			"expected processing memory budget error, got %v",
			err,
		)
	}

	if got := handler.inFlightRequestCount(); got != 0 {
		t.Fatalf(
			"expected no in-flight requests after reservation failure, got %d",
			got,
		)
	}

	if got := handler.processingMemoryUsage(); got != 0 {
		t.Fatalf("expected zero memory usage after rejection, got %d", got)
	}
}

func TestReserveProcessingAccountsConcurrentReservations(t *testing.T) {
	handler := newResourceTestHandler(t, HandlerConfig{
		MaxInFlightRequests:    2,
		MaxFramePayloadSize:    16,
		ProcessingMemoryBudget: 66,
	})

	firstRelease, err := handler.reserveProcessing(1)
	if err != nil {
		t.Fatalf("reserve first request: %v", err)
	}

	secondRelease, err := handler.reserveProcessing(1)
	if err != nil {
		t.Fatalf("reserve second request: %v", err)
	}

	if got, want := handler.processingMemoryUsage(), int64(66); got != want {
		t.Fatalf("unexpected memory usage: got %d, want %d", got, want)
	}

	if got, want := handler.inFlightRequestCount(), 2; got != want {
		t.Fatalf("unexpected in-flight count: got %d, want %d", got, want)
	}

	_, err = handler.reserveProcessing(1)
	if !errors.Is(err, ErrMaxInFlightRequestsReached) {
		t.Fatalf("expected in-flight limit error, got %v", err)
	}

	firstRelease()
	secondRelease()

	if got := handler.processingMemoryUsage(); got != 0 {
		t.Fatalf("expected zero memory usage after releases, got %d", got)
	}

	if got := handler.inFlightRequestCount(); got != 0 {
		t.Fatalf("expected zero in-flight requests after releases, got %d", got)
	}
}

func TestReserveProcessingReleaseIsSafeForConcurrentCalls(t *testing.T) {
	handler := newResourceTestHandler(t, HandlerConfig{
		MaxInFlightRequests: 1,
		MaxFramePayloadSize: 16,
	})

	release, err := handler.reserveProcessing(1)
	if err != nil {
		t.Fatalf("reserve request: %v", err)
	}

	var wait sync.WaitGroup
	for range 10 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			release()
		}()
	}
	wait.Wait()

	if got := handler.inFlightRequestCount(); got != 0 {
		t.Fatalf("expected no in-flight requests after release, got %d", got)
	}

	if got := handler.processingMemoryUsage(); got != 0 {
		t.Fatalf("expected zero memory usage, got %d", got)
	}
}

func TestHandlerRejectsFrameAboveConfiguredMaximum(t *testing.T) {
	handler := newResourceTestHandler(t, HandlerConfig{
		MaxInFlightRequests: 1,
		MaxFramePayloadSize: 1,
	})

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	result := make(chan error, 1)
	go func() {
		result <- handler.Handle(serverConn)
	}()

	header := make([]byte, transporttcp.HeaderSize)
	binary.BigEndian.PutUint32(header, 2)

	if _, err := clientConn.Write(header); err != nil {
		t.Fatalf("write frame header: %v", err)
	}

	select {
	case err := <-result:
		if !errors.Is(err, transporttcp.ErrFrameTooLarge) {
			t.Fatalf("expected frame-too-large error, got %v", err)
		}

	case <-time.After(time.Second):
		t.Fatal("handler did not reject oversized frame")
	}

	if got := handler.processingMemoryUsage(); got != 0 {
		t.Fatalf("expected zero memory usage after rejection, got %d", got)
	}

	if got := handler.inFlightRequestCount(); got != 0 {
		t.Fatalf("expected no in-flight requests after rejection, got %d", got)
	}
}
