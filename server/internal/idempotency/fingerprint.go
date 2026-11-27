package idempotency

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"

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
//
// Os campos são escritos incrementalmente no hash para evitar a criação
// de um buffer temporário contendo toda a operação.
func Fingerprint(command protocol.Command) ([32]byte, error) {
	var zero [32]byte

	if command == nil {
		return zero, ErrUnsupportedCommand
	}

	hasher := sha256.New()

	_, _ = hasher.Write([]byte(fingerprintVersion))

	switch command := command.(type) {
	case *protocol.SetCommand:
		if command == nil {
			return zero, ErrUnsupportedCommand
		}

		_, _ = hasher.Write([]byte{setCommandMarker})

		writeStringField(hasher, command.Key)
		writeBytesField(hasher, command.Value)
		writeUint64(hasher, command.TTLMillis)

	case *protocol.DeleteCommand:
		if command == nil {
			return zero, ErrUnsupportedCommand
		}

		_, _ = hasher.Write([]byte{deleteCommandMarker})

		writeStringField(hasher, command.Key)

	default:
		return zero, ErrUnsupportedCommand
	}

	hasher.Sum(zero[:0])

	return zero, nil
}

// writeStringField acrescenta uma string precedida por seu tamanho
// em uint64 Big Endian.
func writeStringField(hasher hash.Hash, value string) {
	writeUint64(hasher, uint64(len(value)))
	_, _ = hasher.Write([]byte(value))
}

// writeBytesField acrescenta um slice precedido por seu tamanho
// em uint64 Big Endian.
func writeBytesField(hasher hash.Hash, value []byte) {
	writeUint64(hasher, uint64(len(value)))
	_, _ = hasher.Write(value)
}

// writeUint64 acrescenta um inteiro uint64 em Big Endian.
func writeUint64(hasher hash.Hash, value uint64) {
	var encoded [8]byte

	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = hasher.Write(encoded[:])
}
