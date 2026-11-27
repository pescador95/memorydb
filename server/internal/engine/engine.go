package engine

import (
	"github.com/pescador95/memorydb/server/internal/protocol"
)

// Engine executa comandos utilizando o mecanismo de armazenamento.
type Engine struct {
	storage Storage
}

// Execute valida e executa um comando, produzindo uma resposta interna.
func (e *Engine) Execute(request *protocol.Request) *protocol.Response {
	if request == nil {
		return errorResponse(0, ErrInvalidRequest, "request cannot be nil")
	}

	requestID := request.RequestID

	switch command := request.Command.(type) {
	case *protocol.PingCommand:
		if command == nil {
			return errorResponse(requestID, ErrInvalidRequest, "invalid PING command")
		}

		return &protocol.Response{
			RequestID: requestID,
			Success:   true,
			Result:    &protocol.PongResult{},
		}

	case *protocol.SetCommand:
		if command == nil {
			return errorResponse(requestID, ErrInvalidRequest, "invalid SET command")
		}
		if command.Key == "" {
			return errorResponse(requestID, ErrInvalidArgument, "key cannot be empty")
		}
		if command.TTLMillis > 0 {
			return errorResponse(requestID, ErrUnsupportedFeature, "TTL is not supported yet")
		}

		if err := e.storage.Set(command.Key, command.Value); err != nil {
			return storageErrorResponse(requestID, err)
		}

		return &protocol.Response{
			RequestID: requestID,
			Success:   true,
			Result:    &protocol.SetResult{},
		}

	case *protocol.GetCommand:
		if command == nil {
			return errorResponse(requestID, ErrInvalidRequest, "invalid GET command")
		}
		if command.Key == "" {
			return errorResponse(requestID, ErrInvalidArgument, "key cannot be empty")
		}

		value, found, err := e.storage.Get(command.Key)
		if err != nil {
			return storageErrorResponse(requestID, err)
		}

		return &protocol.Response{
			RequestID: requestID,
			Success:   true,
			Result: &protocol.GetResult{
				Found: found,
				Value: value,
			},
		}

	case *protocol.DeleteCommand:
		if command == nil {
			return errorResponse(requestID, ErrInvalidRequest, "invalid DELETE command")
		}
		if command.Key == "" {
			return errorResponse(requestID, ErrInvalidArgument, "key cannot be empty")
		}

		deleted, err := e.storage.Delete(command.Key)
		if err != nil {
			return storageErrorResponse(requestID, err)
		}

		return &protocol.Response{
			RequestID: requestID,
			Success:   true,
			Result: &protocol.DeleteResult{
				Deleted: deleted,
			},
		}

	default:
		return errorResponse(requestID, ErrInvalidRequest, "unknown or missing command")
	}
}
