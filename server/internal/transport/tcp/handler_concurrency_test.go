package tcp

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
)

func TestReserveProcessingEnforcesLimitsUnderConcurrency(t *testing.T) {
	tests := []struct {
		name                 string
		config               HandlerConfig
		expectedRejectionErr error
	}{
		{
			name: "maximum in-flight requests",
			config: HandlerConfig{
				MaxInFlightRequests: 1,
				MaxFramePayloadSize: 16,
			},
			expectedRejectionErr: ErrMaxInFlightRequestsReached,
		},
		{
			name: "processing memory budget",
			config: HandlerConfig{
				MaxInFlightRequests:    32,
				MaxFramePayloadSize:    16,
				ProcessingMemoryBudget: 33,
			},
			expectedRejectionErr: ErrProcessingMemoryBudgetReached,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newResourceTestHandler(t, tt.config)

			const goroutines = 32

			start := make(chan struct{})
			results := make(chan reservationResult, goroutines)

			var wait sync.WaitGroup

			for range goroutines {
				wait.Add(1)

				go func() {
					defer wait.Done()

					<-start

					release, err := handler.reserveProcessing(1)
					results <- reservationResult{
						release: release,
						err:     err,
					}
				}()
			}

			close(start)
			wait.Wait()
			close(results)

			successes := 0
			rejections := 0

			for result := range results {
				if result.err != nil {
					if !errors.Is(result.err, tt.expectedRejectionErr) {
						t.Errorf(
							"unexpected reservation error: got %v, want %v",
							result.err,
							tt.expectedRejectionErr,
						)
					}

					rejections++
					continue
				}

				successes++

				if result.release == nil {
					t.Error("successful reservation returned nil release function")
					continue
				}

				result.release()
			}

			if successes != 1 {
				t.Errorf("successful reservations = %d, want 1", successes)
			}

			if rejections != goroutines-1 {
				t.Errorf(
					"rejected reservations = %d, want %d",
					rejections,
					goroutines-1,
				)
			}

			if got := handler.inFlightRequestCount(); got != 0 {
				t.Errorf("in-flight requests after releases = %d, want 0", got)
			}

			if got := handler.processingMemoryUsage(); got != 0 {
				t.Errorf("processing memory after releases = %d, want 0", got)
			}
		})
	}
}

type reservationResult struct {
	release func()
	err     error
}

func TestHandlerReleasesReservationWhenPayloadReadFails(t *testing.T) {
	handler := newResourceTestHandler(t, HandlerConfig{
		MaxInFlightRequests: 1,
		MaxFramePayloadSize: 16,
	})

	serverConn, clientConn := net.Pipe()

	handlerDone := make(chan error, 1)

	go func() {
		handlerDone <- handler.Handle(serverConn)
	}()

	header := make([]byte, transporttcp.HeaderSize)
	binary.BigEndian.PutUint32(header, 8)

	if _, err := clientConn.Write(header); err != nil {
		_ = clientConn.Close()
		t.Fatalf("write frame header: %v", err)
	}

	waitForProcessingReservation(t, handler)

	if _, err := clientConn.Write([]byte{1, 2}); err != nil {
		_ = clientConn.Close()
		t.Fatalf("write partial payload: %v", err)
	}

	if err := clientConn.Close(); err != nil {
		t.Fatalf("close client connection: %v", err)
	}

	select {
	case err := <-handlerDone:
		if err == nil {
			t.Fatal("Handle() error = nil, want incomplete payload error")
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Handler did not finish after incomplete payload")
	}

	assertNoProcessingReservations(t, handler)
}

func waitForProcessingReservation(t *testing.T, handler *Handler) {
	t.Helper()

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if handler.inFlightRequestCount() == 1 &&
			handler.processingMemoryUsage() > 0 {
			return
		}

		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for processing reservation")
		case <-ticker.C:
		}
	}
}
