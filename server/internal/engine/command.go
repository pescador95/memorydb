package engine

import (
	"errors"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

const (
	ErrInvalidRequest     = "INVALID_REQUEST"
	ErrInvalidArgument    = "INVALID_ARGUMENT"
	ErrUnsupportedFeature = "UNSUPPORTED_FEATURE"
	ErrStorageFailure     = "STORAGE_ERROR"
)

type storageErrorCoder interface {
	StorageErrorCode() string
}

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

// storageErrorResponse preserva códigos específicos de erros do Storage
// sem exigir que o Engine conheça sua implementação concreta.
func storageErrorResponse(requestID uint64, err error) *protocol.Response {
	var codedError storageErrorCoder
	if errors.As(err, &codedError) {
		return errorResponse(
			requestID,
			codedError.StorageErrorCode(),
			err.Error(),
		)
	}

	return errorResponse(
		requestID,
		ErrStorageFailure,
		"storage operation failed",
	)
}
