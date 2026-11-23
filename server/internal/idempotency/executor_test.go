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

func TestNewExecutorAppliesDefaultConfig(t *testing.T) {
	config := &Config{}

	executor, err := NewExecutor(config)
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}

	if executor.retention != DefaultRetention {
		t.Errorf("retention = %v, want %v", executor.retention, DefaultRetention)
	}

	if executor.capacity != DefaultCapacity {
		t.Errorf("capacity = %d, want %d", executor.capacity, DefaultCapacity)
	}

	// A configuração recebida também deve refletir os valores padrão.
	if config.Retention != DefaultRetention {
		t.Errorf("config.Retention = %v, want %v", config.Retention, DefaultRetention)
	}

	if config.Capacity != DefaultCapacity {
		t.Errorf("config.Capacity = %d, want %d", config.Capacity, DefaultCapacity)
	}
}

func TestNewExecutorRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "negative retention",
			config: Config{
				Retention: -time.Second,
				Capacity:  1,
			},
		},
		{
			name: "negative capacity",
			config: Config{
				Retention: time.Hour,
				Capacity:  -1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExecutor(&tt.config)
			if err == nil {
				t.Fatal("expected error for invalid config")
			}
		})
	}
}

func TestExecuteRejectsInvalidOperationID(t *testing.T) {
	tests := []struct {
		name string
		id   []byte
	}{
		{
			name: "empty ID",
			id:   nil,
		},
		{
			name: "short ID",
			id:   make([]byte, OperationIDSize-1),
		},
		{
			name: "long ID",
			id:   make([]byte, OperationIDSize+1),
		},
	}

	executor := newTestExecutor(t, time.Hour, 10)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executor.Execute(
				tt.id,
				10,
				testFingerprint(1),
				func() *protocol.Response {
					t.Fatal("callback must not execute for invalid operation ID")
					return nil
				},
			)

			if !errors.Is(err, ErrInvalidOperationID) {
				t.Fatalf("error = %v, want ErrInvalidOperationID", err)
			}
		})
	}
}

func TestExecuteRejectsNilCallback(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 10)

	_, err := executor.Execute(
		testOperationID(1),
		10,
		testFingerprint(1),
		nil,
	)

	if !errors.Is(err, ErrNilExecution) {
		t.Fatalf("error = %v, want ErrNilExecution", err)
	}
}

func TestExecuteRetainsUnknownOutcomeAfterPanic(t *testing.T) {
	executor := newTestExecutor(t, time.Hour, 10)

	var recovered any

	func() {
		defer func() {
			recovered = recover()
		}()

		_, _ = executor.Execute(
			testOperationID(1),
			10,
			testFingerprint(1),
			func() *protocol.Response {
				panic("execution failed")
			},
		)
	}()

	if recovered != "execution failed" {
		t.Fatalf("recovered = %v, want %q", recovered, "execution failed")
	}

	_, err := executor.Execute(
		testOperationID(1),
		20,
		testFingerprint(1),
		func() *protocol.Response {
			t.Fatal("callback must not execute again after panic")
			return nil
		},
	)

	if !errors.Is(err, ErrOutcomeUnknown) {
		t.Fatalf("replay error = %v, want ErrOutcomeUnknown", err)
	}
}

func TestCloneResponseSupportsAllResultTypes(t *testing.T) {
	tests := []struct {
		name   string
		result protocol.Result
		check  func(t *testing.T, cloned protocol.Result)
	}{
		{
			name:   "pong",
			result: &protocol.PongResult{},
			check: func(t *testing.T, cloned protocol.Result) {
				if _, ok := cloned.(*protocol.PongResult); !ok {
					t.Fatalf("result type = %T, want *protocol.PongResult", cloned)
				}
			},
		},
		{
			name:   "set",
			result: &protocol.SetResult{},
			check: func(t *testing.T, cloned protocol.Result) {
				if _, ok := cloned.(*protocol.SetResult); !ok {
					t.Fatalf("result type = %T, want *protocol.SetResult", cloned)
				}
			},
		},
		{
			name: "get",
			result: &protocol.GetResult{
				Found: true,
				Value: []byte("value"),
			},
			check: func(t *testing.T, cloned protocol.Result) {
				result, ok := cloned.(*protocol.GetResult)
				if !ok {
					t.Fatalf("result type = %T, want *protocol.GetResult", cloned)
				}

				if !result.Found {
					t.Error("Found = false, want true")
				}

				if string(result.Value) != "value" {
					t.Errorf("Value = %q, want %q", result.Value, "value")
				}
			},
		},
		{
			name: "delete",
			result: &protocol.DeleteResult{
				Deleted: true,
			},
			check: func(t *testing.T, cloned protocol.Result) {
				result, ok := cloned.(*protocol.DeleteResult)
				if !ok {
					t.Fatalf("result type = %T, want *protocol.DeleteResult", cloned)
				}

				if !result.Deleted {
					t.Error("Deleted = false, want true")
				}
			},
		},
		{
			name: "error",
			result: &protocol.ErrorResult{
				Code:    "TEST_ERROR",
				Message: "test message",
			},
			check: func(t *testing.T, cloned protocol.Result) {
				result, ok := cloned.(*protocol.ErrorResult)
				if !ok {
					t.Fatalf("result type = %T, want *protocol.ErrorResult", cloned)
				}

				if result.Code != "TEST_ERROR" {
					t.Errorf("Code = %q, want %q", result.Code, "TEST_ERROR")
				}

				if result.Message != "test message" {
					t.Errorf("Message = %q, want %q", result.Message, "test message")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &protocol.Response{
				RequestID: 123,
				Success:   true,
				Result:    tt.result,
			}

			cloned := cloneResponse(original)
			if cloned == nil {
				t.Fatal("cloneResponse() returned nil")
			}

			if cloned == original {
				t.Fatal("cloneResponse() returned the original response")
			}

			if cloned.RequestID != original.RequestID {
				t.Errorf("RequestID = %d, want %d", cloned.RequestID, original.RequestID)
			}

			if cloned.Success != original.Success {
				t.Errorf("Success = %v, want %v", cloned.Success, original.Success)
			}

			tt.check(t, cloned.Result)
		})
	}
}

func TestCloneResponseCopiesGetValueIndependently(t *testing.T) {
	original := &protocol.Response{
		Result: &protocol.GetResult{
			Found: true,
			Value: []byte("original"),
		},
	}

	cloned := cloneResponse(original)
	if cloned == nil {
		t.Fatal("cloneResponse() returned nil")
	}

	originalResult := original.Result.(*protocol.GetResult)
	clonedResult := cloned.Result.(*protocol.GetResult)

	originalResult.Value[0] = 'X'

	if string(clonedResult.Value) != "original" {
		t.Errorf("cloned Value = %q, want %q", clonedResult.Value, "original")
	}
}

func TestCloneResponseRejectsNil(t *testing.T) {
	if cloned := cloneResponse(nil); cloned != nil {
		t.Fatalf("cloneResponse(nil) = %v, want nil", cloned)
	}
}
