package idempotency

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

const (
	OperationIDSize  = 16
	DefaultRetention = 24 * time.Hour
	DefaultCapacity  = 100_000
)

var (
	ErrInvalidOperationID  = errors.New("invalid operation ID")
	ErrFingerprintConflict = errors.New("operation ID already used with a different fingerprint")
	ErrCapacityReached     = errors.New("idempotency capacity reached")
	ErrOutcomeUnknown      = errors.New("operation outcome is unknown")
	ErrNilExecution        = errors.New("execution callback cannot be nil")
)

// Config define os limites do executor de idempotência.
type Config struct {
	Retention time.Duration
	Capacity  int
}

type entry struct {
	fingerprint [32]byte
	done        chan struct{}

	response *protocol.Response
	err      error

	completedAt time.Time
	completed   bool
}

// Executor controla a execução idempotente de operações.
type Executor struct {
	mu sync.Mutex

	entries   map[string]*entry
	retention time.Duration
	capacity  int

	now func() time.Time
}

// NewExecutor cria um executor de idempotência.
func NewExecutor(config *Config) (*Executor, error) {
	if config == nil {
		return nil, errors.New("idempotency config cannot be nil")
	}

	if config.Retention == 0 {
		config.Retention = DefaultRetention
	}
	if config.Capacity == 0 {
		config.Capacity = DefaultCapacity
	}

	if config.Retention < 0 {
		return nil, fmt.Errorf("retention must be positive")
	}
	if config.Capacity < 1 {
		return nil, fmt.Errorf("capacity must be positive")
	}

	return &Executor{
		entries:   make(map[string]*entry),
		retention: config.Retention,
		capacity:  config.Capacity,
		now:       time.Now,
	}, nil
}

// Execute executa uma operação uma única vez por OperationID retido.
//
// Requisições duplicadas com o mesmo fingerprint aguardam a execução
// original e recebem seu resultado. O RequestID da resposta reproduzida
// corresponde sempre à requisição atual.
func (e *Executor) Execute(
	operationID []byte,
	requestID uint64,
	fingerprint [32]byte,
	execute func() *protocol.Response,
) (*protocol.Response, error) {
	if len(operationID) != OperationIDSize {
		return nil, fmt.Errorf(
			"%w: expected %d bytes, got %d",
			ErrInvalidOperationID,
			OperationIDSize,
			len(operationID),
		)
	}

	if execute == nil {
		return nil, ErrNilExecution
	}

	key := string(operationID)

	e.mu.Lock()
	e.removeExpiredLocked(e.now())

	if existing, found := e.entries[key]; found {
		if existing.fingerprint != fingerprint {
			e.mu.Unlock()
			return nil, ErrFingerprintConflict
		}

		e.mu.Unlock()

		<-existing.done

		if existing.err != nil {
			return nil, existing.err
		}

		response := cloneResponse(existing.response)
		response.RequestID = requestID

		return response, nil
	}

	if len(e.entries) >= e.capacity {
		e.mu.Unlock()
		return nil, ErrCapacityReached
	}

	current := &entry{
		fingerprint: fingerprint,
		done:        make(chan struct{}),
	}

	e.entries[key] = current
	e.mu.Unlock()

	return e.executeReserved(current, requestID, execute)
}

// executeReserved executa o callback associado a uma reserva.
func (e *Executor) executeReserved(
	current *entry,
	requestID uint64,
	execute func() *protocol.Response,
) (response *protocol.Response, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			e.completeUnknown(current)

			// Não converte um panic em erro silencioso.
			// A reserva permanece retida para impedir nova execução.
			panic(recovered)
		}
	}()

	response = execute()

	if response == nil {
		e.completeUnknown(current)
		return nil, ErrOutcomeUnknown
	}

	stored := cloneResponse(response)
	if stored == nil {
		e.completeUnknown(current)
		return nil, ErrOutcomeUnknown
	}

	e.mu.Lock()

	current.response = stored
	current.completed = true
	current.completedAt = e.now()
	close(current.done)

	e.mu.Unlock()

	result := cloneResponse(current.response)
	result.RequestID = requestID

	return result, nil
}

// completeUnknown finaliza uma reserva cujo resultado não pode ser
// considerado seguramente reproduzível.
func (e *Executor) completeUnknown(current *entry) {
	e.mu.Lock()
	defer e.mu.Unlock()

	current.err = ErrOutcomeUnknown
	current.completed = true
	current.completedAt = e.now()

	close(current.done)
}

// removeExpiredLocked remove somente registros concluídos cuja retenção
// já expirou. Registros em andamento nunca são removidos por tempo.
func (e *Executor) removeExpiredLocked(now time.Time) {
	for operationID, current := range e.entries {
		if !current.completed {
			continue
		}

		if now.Sub(current.completedAt) >= e.retention {
			delete(e.entries, operationID)
		}
	}
}

// cloneResponse cria uma cópia independente da resposta armazenada.
func cloneResponse(response *protocol.Response) *protocol.Response {
	if response == nil {
		return nil
	}

	cloned := &protocol.Response{
		RequestID: response.RequestID,
		Success:   response.Success,
	}

	switch result := response.Result.(type) {
	case *protocol.PongResult:
		cloned.Result = &protocol.PongResult{}

	case *protocol.SetResult:
		cloned.Result = &protocol.SetResult{}

	case *protocol.GetResult:
		cloned.Result = &protocol.GetResult{
			Found: result.Found,
			Value: append([]byte(nil), result.Value...),
		}

	case *protocol.DeleteResult:
		cloned.Result = &protocol.DeleteResult{
			Deleted: result.Deleted,
		}

	case *protocol.ErrorResult:
		cloned.Result = &protocol.ErrorResult{
			Code:    result.Code,
			Message: result.Message,
		}
	default:
		return nil
	}

	return cloned
}
