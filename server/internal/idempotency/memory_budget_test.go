package idempotency

import (
	"errors"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

func newBudgetTestExecutor(t *testing.T, memoryBudget int64, capacity int) *Executor {
	t.Helper()

	executor, err := NewExecutor(&Config{
		Retention:    time.Hour,
		Capacity:     capacity,
		MemoryBudget: memoryBudget,
	})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	return executor
}

func TestNewExecutorRejectsNegativeMemoryBudget(t *testing.T) {
	_, err := NewExecutor(&Config{
		Retention:    time.Hour,
		Capacity:     10,
		MemoryBudget: -1,
	})

	if err == nil {
		t.Fatal("expected error for negative memory budget")
	}
}

func TestExecuteRejectsWhenBaseReservationExceedsMemoryBudget(t *testing.T) {
	executor := newBudgetTestExecutor(
		t,
		entryBaseMemorySize()-1,
		10,
	)

	executed := false

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		func() *protocol.Response {
			executed = true
			return testResponse(10)
		},
	)

	if !errors.Is(err, ErrMemoryBudgetReached) {
		t.Fatalf("error = %v, want ErrMemoryBudgetReached", err)
	}

	if executed {
		t.Fatal("callback executed despite insufficient memory budget")
	}

	if executor.Len() != 0 {
		t.Errorf("entries = %d, want 0", executor.Len())
	}

	if executor.MemoryUsage() != 0 {
		t.Errorf("memory usage = %d, want 0", executor.MemoryUsage())
	}
}

func TestExecuteAccountsForBaseReservation(t *testing.T) {
	base := entryBaseMemorySize()
	executor := newBudgetTestExecutor(t, base, 10)

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		func() *protocol.Response {
			return testResponse(10)
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := executor.MemoryUsage(); got != base {
		t.Errorf("memory usage = %d, want %d", got, base)
	}

	if got := executor.Len(); got != 1 {
		t.Errorf("entries = %d, want 1", got)
	}
}

func TestExecuteAccountsForVariableResponseMemory(t *testing.T) {
	const (
		code    = "TEST_ERROR"
		message = "operation rejected"
	)

	base := entryBaseMemorySize()
	responseBytes := int64(len(code) + len(message))

	executor := newBudgetTestExecutor(
		t,
		base+responseBytes,
		10,
	)

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		func() *protocol.Response {
			return &protocol.Response{
				RequestID: 10,
				Success:   false,
				Result: &protocol.ErrorResult{
					Code:    code,
					Message: message,
				},
			}
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := base + responseBytes
	if got := executor.MemoryUsage(); got != want {
		t.Errorf("memory usage = %d, want %d", got, want)
	}
}

func TestExecuteReturnsUnknownWhenResponseExceedsMemoryBudget(t *testing.T) {
	base := entryBaseMemorySize()

	executor := newBudgetTestExecutor(
		t,
		base,
		10,
	)

	callbackCalls := 0

	execute := func() *protocol.Response {
		callbackCalls++

		return &protocol.Response{
			RequestID: 10,
			Success:   false,
			Result: &protocol.ErrorResult{
				Code:    "ERROR",
				Message: "response does not fit",
			},
		}
	}

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		execute,
	)

	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("error = %v, want ErrOutcomeUnknown", err)
	}

	if callbackCalls != 1 {
		t.Fatalf("callback calls = %d, want 1", callbackCalls)
	}

	// A reserva base permanece retida para impedir uma segunda execução.
	if got := executor.MemoryUsage(); got != base {
		t.Errorf("memory usage = %d, want %d", got, base)
	}

	_, err = executor.Execute(
		testOperationID(1),
		20,
		testFingerprint(1),
		execute,
	)

	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("replay error = %v, want ErrOutcomeUnknown", err)
	}

	if callbackCalls != 1 {
		t.Errorf("callback calls after replay = %d, want 1", callbackCalls)
	}
}

func TestExecuteDuplicateDoesNotIncreaseMemoryUsage(t *testing.T) {
	executor := newBudgetTestExecutor(t, 0, 10)

	execute := func() *protocol.Response {
		return testResponse(10)
	}

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		execute,
	)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	usageAfterFirst := executor.MemoryUsage()

	_, err = executor.Execute(
		testOperationID(1),
		20,
		testFingerprint(1),
		execute,
	)
	if err != nil {
		t.Fatalf("duplicate Execute() error = %v", err)
	}

	if got := executor.MemoryUsage(); got != usageAfterFirst {
		t.Errorf("memory usage after duplicate = %d, want %d", got, usageAfterFirst)
	}
}

func TestExecuteExpirationReleasesMemory(t *testing.T) {
	executor := newBudgetTestExecutor(t, 0, 1)

	now := time.Now()
	executor.now = func() time.Time {
		return now
	}

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		func() *protocol.Response {
			return &protocol.Response{
				RequestID: 10,
				Success:   false,
				Result: &protocol.ErrorResult{
					Code:    "ERROR",
					Message: "retained response",
				},
			}
		},
	)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	if executor.MemoryUsage() == 0 {
		t.Fatal("expected non-zero memory usage before expiration")
	}

	now = now.Add(time.Hour)

	_, err = executor.Execute(
		testOperationID(2),
		20,
		testFingerprint(2),
		func() *protocol.Response {
			return testResponse(20)
		},
	)
	if err != nil {
		t.Fatalf("Execute() after expiration error = %v", err)
	}

	want := entryBaseMemorySize()
	if got := executor.MemoryUsage(); got != want {
		t.Errorf("memory usage after expiration = %d, want %d", got, want)
	}

	if got := executor.Len(); got != 1 {
		t.Errorf("entries after expiration = %d, want 1", got)
	}
}

func TestExecuteUnknownOutcomeRetainsBaseReservation(t *testing.T) {
	base := entryBaseMemorySize()
	executor := newBudgetTestExecutor(t, base, 10)

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		func() *protocol.Response {
			return nil
		},
	)

	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("error = %v, want ErrOutcomeUnknown", err)
	}

	if got := executor.MemoryUsage(); got != base {
		t.Errorf("memory usage = %d, want %d", got, base)
	}
}
