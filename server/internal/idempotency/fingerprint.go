package idempotency

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

var ErrUnsupportedCommand = errors.New("unsupported command for idempotency fingerprint")

const fingerprintVersion = "memorydb:idempotency:fingerprint:v1"

const (
	setCommandMarker    byte = 1
	deleteCommandMarker byte = 2
)

// Fingerprint calcula o SHA-256 dos campos semânticos de uma operação.
//
// OperationID e RequestID não participam do cálculo.
// Somente SET e DELETE são operações idempotentes nesta versão.
func Fingerprint(command protocol.Command) ([32]byte, error) {
	var zero [32]byte

	if command == nil {
		return zero, ErrUnsupportedCommand
	}

	data := make([]byte, 0, 128)
	data = append(data, fingerprintVersion...)

	switch command := command.(type) {
	case *protocol.SetCommand:
		if command == nil {
			return zero, ErrUnsupportedCommand
		}

		data = append(data, setCommandMarker)

		data = appendBytesField(data, []byte(command.Key))
		data = appendBytesField(data, command.Value)
		data = appendUint64(data, command.TTLMillis)

	case *protocol.DeleteCommand:
		if command == nil {
			return zero, ErrUnsupportedCommand
		}

		data = append(data, deleteCommandMarker)
		data = appendBytesField(data, []byte(command.Key))

	default:
		return zero, ErrUnsupportedCommand
	}

	return sha256.Sum256(data), nil
}

// appendBytesField acrescenta um campo precedido por seu tamanho em uint64
// Big Endian. Isso evita ambiguidades causadas pela concatenação simples.
func appendBytesField(data []byte, value []byte) []byte {
	data = appendUint64(data, uint64(len(value)))
	return append(data, value...)
}

// appendUint64 acrescenta um inteiro uint64 em Big Endian.
func appendUint64(data []byte, value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return append(data, encoded[:]...)
}
