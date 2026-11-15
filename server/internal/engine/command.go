package engine

import "github.com/pescador95/memorydb/server/internal/protocol"

const (
	ErrInvalidRequest     = "INVALID_REQUEST"
	ErrInvalidArgument    = "INVALID_ARGUMENT"
	ErrUnsupportedFeature = "UNSUPPORTED_FEATURE"
)

func errorResponse(requestID uint64, code, message string) *protocol.Response {
	return &protocol.Response{
		RequestID: requestID,
		Success:   false,
		Result: &protocol.ErrorResult{
			Code:    code,
			Message: message,
		},
	}
}
