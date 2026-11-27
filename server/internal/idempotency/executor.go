package idempotency

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

const (
	OperationIDSize  = 16
	DefaultRetention = 24 * time.Hour
	DefaultCapacity  = 100_000

	// entryMetadataMemoryBytes estima o custo fixo lógico da estrutura
	// armazenada por operação, incluindo metadados e estruturas auxiliares.
	//
	// O tamanho do OperationID e do fingerprint é contabilizado à parte.
	// Não representa o overhead exato do runtime Go ou do alocador.
	entryMetadataMemoryBytes int64 = 128

	fingerprintSize int64 = 32
)

var (
	ErrInvalidOperationID  = errors.New("invalid operation ID")
	ErrFingerprintConflict = errors.New("operation ID already used with a different fingerprint")
	ErrCapacityReached     = errors.New("idempotency capacity reached")
	ErrMemoryBudgetReached = errors.New("idempotency memory budget reached")
	ErrOutcomeUnknown      = errors.New("operation outcome is unknown")
	ErrNilExecution        = errors.New("execution callback cannot be nil")
)

// Config define os limites do executor de idempotência.
type Config struct {
	Retention    time.Duration
	Capacity     int
	MemoryBudget int64
}

type entry struct {
	fingerprint [32]byte
	done        chan struct{}

	response *protocol.Response
	err      error

	completedAt time.Time
	completed   bool

	memoryBytes int64
}

// Executor controla a execução idempotente de operações.
type Executor struct {
	mu sync.Mutex

	entries   map[string]*entry
	retention time.Duration
	capacity  int

	memoryBudget int64
	usedMemory   int64

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
	if config.MemoryBudget < 0 {
		return nil, fmt.Errorf("memory budget cannot be negative")
	}

	return &Executor{
		entries:      make(map[string]*entry),
		retention:    config.Retention,
		capacity:     config.Capacity,
		memoryBudget: config.MemoryBudget,
		now:          time.Now,
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
		if response == nil {
			return nil, ErrOutcomeUnknown
		}

		response.RequestID = requestID

		return response, nil
	}

	if len(e.entries) >= e.capacity {
		e.mu.Unlock()
		return nil, ErrCapacityReached
	}

	reservation := entryBaseMemorySize()
	if !e.canReserveLocked(reservation) {
		e.mu.Unlock()
		return nil, ErrMemoryBudgetReached
	}

	current := &entry{
		fingerprint: fingerprint,
		done:        make(chan struct{}),
		memoryBytes: reservation,
	}

	e.entries[key] = current
	e.usedMemory += reservation

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

	responseBytes, supported := responseVariableMemory(response)
	if !supported {
		e.completeUnknown(current)
		return nil, ErrOutcomeUnknown
	}

	e.mu.Lock()

	if !e.canReserveLocked(responseBytes) {
		e.completeUnknownLocked(current)
		e.mu.Unlock()

		// A operação já foi executada. Não podemos liberar seu
		// OperationID e permitir que uma repetição execute novamente.
		return nil, ErrOutcomeUnknown
	}

	e.usedMemory += responseBytes
	current.memoryBytes += responseBytes

	e.mu.Unlock()

	stored := cloneResponse(response)
	if stored == nil {
		e.mu.Lock()
		e.releaseMemoryLocked(current, responseBytes)
		e.completeUnknownLocked(current)
		e.mu.Unlock()

		return nil, ErrOutcomeUnknown
	}

	e.mu.Lock()

	current.response = stored
	current.completed = true
	current.completedAt = e.now()
	close(current.done)

	e.mu.Unlock()

	result := cloneResponse(stored)
	if result == nil {
		return nil, ErrOutcomeUnknown
	}

	result.RequestID = requestID

	return result, nil
}

// completeUnknown finaliza uma reserva cujo resultado não pode ser
// considerado seguramente reproduzível.
func (e *Executor) completeUnknown(current *entry) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.completeUnknownLocked(current)
}

// completeUnknownLocked finaliza uma operação mantendo sua reserva.
//
// O chamador deve possuir e.mu.
func (e *Executor) completeUnknownLocked(current *entry) {
	current.err = ErrOutcomeUnknown
	current.completed = true
	current.completedAt = e.now()

	close(current.done)
}

// removeExpiredLocked remove somente registros concluídos cuja retenção
// já expirou. Registros em andamento nunca são removidos por tempo.
//
// O chamador deve possuir e.mu.
func (e *Executor) removeExpiredLocked(now time.Time) {
	for operationID, current := range e.entries {
		if !current.completed {
			continue
		}

		if now.Sub(current.completedAt) >= e.retention {
			delete(e.entries, operationID)
			e.releaseMemoryLocked(current, current.memoryBytes)
		}
	}
}

// entryBaseMemorySize retorna o custo lógico fixo de uma entrada.
//
// O OperationID e o fingerprint são contabilizados explicitamente.
// O restante é uma estimativa fixa dos metadados e estruturas auxiliares.
func entryBaseMemorySize() int64 {
	return int64(OperationIDSize) + fingerprintSize + entryMetadataMemoryBytes
}

// canReserveLocked verifica se uma reserva cabe no orçamento.
//
// MemoryBudget igual a zero representa orçamento ilimitado.
// O controle de overflow continua ativo nesse modo.
//
// O chamador deve possuir e.mu.
func (e *Executor) canReserveLocked(bytes int64) bool {
	if bytes < 0 {
		return false
	}

	if e.usedMemory > math.MaxInt64-bytes {
		return false
	}

	if e.memoryBudget > 0 && bytes > e.memoryBudget-e.usedMemory {
		return false
	}

	return true
}

// releaseMemoryLocked libera consumo lógico já contabilizado.
//
// O chamador deve possuir e.mu.
func (e *Executor) releaseMemoryLocked(current *entry, bytes int64) {
	if bytes <= 0 {
		return
	}

	if bytes > current.memoryBytes {
		bytes = current.memoryBytes
	}

	current.memoryBytes -= bytes
	e.usedMemory -= bytes

	if e.usedMemory < 0 {
		e.usedMemory = 0
	}
}

// responseVariableMemory retorna o tamanho lógico variável de uma resposta.
//
// O custo fixo da estrutura da resposta já está incluído na reserva base
// da entrada. Aqui são contabilizados os conteúdos variáveis.
//
// O segundo retorno indica se o tipo de resposta pode ser armazenado
// e reproduzido pelo executor.
func responseVariableMemory(response *protocol.Response) (int64, bool) {
	if response == nil || response.Result == nil {
		return 0, false
	}

	switch result := response.Result.(type) {
	case *protocol.PongResult:
		if result == nil {
			return 0, false
		}

		return 0, true

	case *protocol.SetResult:
		if result == nil {
			return 0, false
		}

		return 0, true

	case *protocol.GetResult:
		if result == nil {
			return 0, false
		}

		return int64(len(result.Value)), true

	case *protocol.DeleteResult:
		if result == nil {
			return 0, false
		}

		return 0, true

	case *protocol.ErrorResult:
		if result == nil {
			return 0, false
		}

		codeBytes := int64(len(result.Code))
		messageBytes := int64(len(result.Message))

		if codeBytes > math.MaxInt64-messageBytes {
			return 0, false
		}

		return codeBytes + messageBytes, true

	default:
		return 0, false
	}
}

// MemoryUsage retorna o consumo lógico atualmente contabilizado.
//
// O valor não representa o consumo exato do heap ou do RSS do processo.
func (e *Executor) MemoryUsage() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.usedMemory
}

// Len retorna a quantidade de entradas atualmente retidas.
func (e *Executor) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	return len(e.entries)
}

// cloneResponse cria uma cópia independente da resposta armazenada.
func cloneResponse(response *protocol.Response) *protocol.Response {
	if response == nil || response.Result == nil {
		return nil
	}

	cloned := &protocol.Response{
		RequestID: response.RequestID,
		Success:   response.Success,
	}

	switch result := response.Result.(type) {
	case *protocol.PongResult:
		if result == nil {
			return nil
		}

		cloned.Result = &protocol.PongResult{}

	case *protocol.SetResult:
		if result == nil {
			return nil
		}

		cloned.Result = &protocol.SetResult{}

	case *protocol.GetResult:
		if result == nil {
			return nil
		}

		cloned.Result = &protocol.GetResult{
			Found: result.Found,
			Value: append([]byte(nil), result.Value...),
		}

	case *protocol.DeleteResult:
		if result == nil {
			return nil
		}

		cloned.Result = &protocol.DeleteResult{
			Deleted: result.Deleted,
		}

	case *protocol.ErrorResult:
		if result == nil {
			return nil
		}

		cloned.Result = &protocol.ErrorResult{
			Code:    result.Code,
			Message: result.Message,
		}

	default:
		return nil
	}

	return cloned
}
