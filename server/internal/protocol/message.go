package protocol

// Command representa um comando que pode ser executado pelo MemoryDB.
type Command interface {
	isCommand()
}

// PingCommand representa uma solicitação de verificação de disponibilidade.
type PingCommand struct{}

func (*PingCommand) isCommand() {}

// SetCommand representa uma operação de gravação de valor.
type SetCommand struct {
	OperationID []byte
	Key         string
	Value       []byte
	TTLMillis   uint64
}

func (*SetCommand) isCommand() {}

// GetCommand representa uma operação de leitura de valor.
type GetCommand struct {
	Key string
}

func (*GetCommand) isCommand() {}

// DeleteCommand representa uma operação de remoção de chave.
type DeleteCommand struct {
	OperationID []byte
	Key         string
}

func (*DeleteCommand) isCommand() {}

// Request representa uma solicitação recebida pelo MemoryDB.
type Request struct {
	RequestID uint64
	Command   Command
}

// Result representa o resultado de uma operação executada pelo MemoryDB.
type Result interface {
	isResult()
}

// PongResult representa a resposta de uma operação PING.
type PongResult struct{}

func (*PongResult) isResult() {}

// SetResult representa a resposta de uma operação SET.
type SetResult struct{}

func (*SetResult) isResult() {}

// GetResult representa o resultado de uma operação GET.
type GetResult struct {
	Found bool
	Value []byte
}

func (*GetResult) isResult() {}

// DeleteResult representa o resultado de uma operação DELETE.
type DeleteResult struct {
	Deleted bool
}

func (*DeleteResult) isResult() {}

// ErrorResult representa uma falha durante o processamento de uma solicitação.
type ErrorResult struct {
	Code    string
	Message string
}

func (*ErrorResult) isResult() {}

// Response representa uma resposta produzida pelo MemoryDB.
type Response struct {
	RequestID uint64
	Success   bool
	Result    Result
}
