package idempotency

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

func newTestExecutor(t *testing.T, retention time.Duration, capacity int) *Executor {
	t.Helper()

	executor, err := NewExecutor(&Config{
		Retention: retention,
		Capacity:  capacity,
	})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	return executor
}

func testOperationID(value byte) []byte {
	id := make([]byte, OperationIDSize)
	id[0] = value
	return id
}

func testFingerprint(value byte) [32]byte {
	var fingerprint [32]byte
	fingerprint[0] = value
	return fingerprint
}

func testResponse(requestID uint64) *protocol.Response {
	return &protocol.Response{
		RequestID: requestID,
		Success:   true,
		Result:    &protocol.SetResult{},
	}
}

func TestNewExecutorRejectsNilConfig(t *testing.T) {
	_, err := NewExecutor(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestExecuteRunsOperationOnce(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 10)

	var executions atomic.Int32

	response, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		func() *protocol.Response {
			executions.Add(1)
			return testResponse(10)
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if response.RequestID != 10 {
		t.Fatalf("RequestID = %d, want 10", response.RequestID)
	}

	if executions.Load() != 1 {
		t.Fatalf("executions = %d, want 1", executions.Load())
	}
}

func TestExecuteReplaysResponseWithCurrentRequestID(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 10)

	var executions atomic.Int32

	execute := func() *protocol.Response {
		executions.Add(1)
		return testResponse(10)
	}

	first, err := executor.Execute(
		testOperationID(1), 10, testFingerprint(1), execute,
	)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	second, err := executor.Execute(
		testOperationID(1), 20, testFingerprint(1), execute,
	)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}

	if first.RequestID != 10 {
		t.Fatalf("first RequestID = %d, want 10", first.RequestID)
	}

	if second.RequestID != 20 {
		t.Fatalf("second RequestID = %d, want 20", second.RequestID)
	}

	if executions.Load() != 1 {
		t.Fatalf("executions = %d, want 1", executions.Load())
	}
}

func TestExecuteRejectsDifferentFingerprint(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 10)

	_, err := executor.Execute(
		testOperationID(1), 10, testFingerprint(1),
		func() *protocol.Response {
			return testResponse(10)
		},
	)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	_, err = executor.Execute(
		testOperationID(1), 20, testFingerprint(2),
		func() *protocol.Response {
			t.Fatal("callback must not execute for conflicting fingerprint")
			return nil
		},
	)

	if !errors.Is(err, ErrFingerprintConflict) {
		t.Fatalf("error = %v, want ErrFingerprintConflict", err)
	}
}

func TestExecuteConcurrentDuplicatesRunOnce(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 100)

	const callers = 32

	var executions atomic.Int32
	var wg sync.WaitGroup

	start := make(chan struct{})
	errs := make(chan error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func(requestID uint64) {
			defer wg.Done()
			<-start

			_, err := executor.Execute(
				testOperationID(1),
				requestID,
				testFingerprint(1),
				func() *protocol.Response {
					executions.Add(1)
					time.Sleep(30 * time.Millisecond)
					return testResponse(requestID)
				},
			)

			errs <- err
		}(uint64(i + 1))
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
	}

	if executions.Load() != 1 {
		t.Fatalf("executions = %d, want 1", executions.Load())
	}
}

func TestExecuteRejectsWhenCapacityIsReached(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 1)

	_, err := executor.Execute(
		testOperationID(1), 10, testFingerprint(1),
		func() *protocol.Response {
			return testResponse(10)
		},
	)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	_, err = executor.Execute(
		testOperationID(2), 20, testFingerprint(2),
		func() *protocol.Response {
			t.Fatal("callback must not execute when capacity is reached")
			return nil
		},
	)

	if !errors.Is(err, ErrCapacityReached) {
		t.Fatalf("error = %v, want ErrCapacityReached", err)
	}
}

func TestExecuteAllowsRetryAfterExpiration(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 1)

	now := time.Now()
	executor.now = func() time.Time {
		return now
	}

	_, err := executor.Execute(
		testOperationID(1), 10, testFingerprint(1),
		func() *protocol.Response {
			return testResponse(10)
		},
	)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	now = now.Add(time.Hour)

	_, err = executor.Execute(
		testOperationID(2), 20, testFingerprint(2),
		func() *protocol.Response {
			return testResponse(20)
		},
	)
	if err != nil {
		t.Fatalf("Execute() after expiration error = %v", err)
	}
}

func TestExecuteDoesNotExpireInProgressOperation(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 1)

	now := time.Now()
	executor.now = func() time.Time {
		return now
	}

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)

	go func() {
		_, err := executor.Execute(
			testOperationID(1), 10, testFingerprint(1),
			func() *protocol.Response {
				close(started)
				<-release
				return testResponse(10)
			},
		)
		finished <- err
	}()

	<-started
	now = now.Add(24 * time.Hour)

	_, err := executor.Execute(
		testOperationID(2), 20, testFingerprint(2),
		func() *protocol.Response {
			t.Fatal("callback must not execute while capacity is occupied")
			return nil
		},
	)

	if !errors.Is(err, ErrCapacityReached) {
		t.Fatalf("error = %v, want ErrCapacityReached", err)
	}

	close(release)

	if err := <-finished; err != nil {
		t.Fatalf("original Execute() error = %v", err)
	}
}

func TestExecuteRetainsUnknownOutcome(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 10)

	_, err := executor.Execute(
		testOperationID(1), 10, testFingerprint(1),
		func() *protocol.Response {
			return nil
		},
	)

	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("error = %v, want ErrOutcomeUnknown", err)
	}

	_, err = executor.Execute(
		testOperationID(1), 20, testFingerprint(1),
		func() *protocol.Response {
			t.Fatal("callback must not execute again")
			return nil
		},
	)

	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("replay error = %v, want ErrOutcomeUnknown", err)
	}
}
