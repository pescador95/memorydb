package tcp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// MaxFrameSize define o tamanho máximo permitido para um quadro: 1 MiB.
	MaxFrameSize uint32 = 1024 * 1024

	// HeaderSize define o tamanho do cabeçalho em bytes.
	HeaderSize = 4
)

var (
	// ErrFrameTooLarge indica que o quadro excede o tamanho máximo permitido.
	ErrFrameTooLarge = errors.New("frame exceeds maximum allowed size")

	// ErrInvalidFrameSize indica que o tamanho do quadro é inválido.
	ErrInvalidFrameSize = errors.New("frame size cannot be zero")
)

// ReadFrame lê um quadro delimitado por um cabeçalho de tamanho.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [HeaderSize]byte

	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	size := binary.BigEndian.Uint32(header[:])

	if size == 0 {
		return nil, ErrInvalidFrameSize
	}

	if size > MaxFrameSize {
		return nil, fmt.Errorf(
			"%w: received %d bytes, maximum is %d",
			ErrFrameTooLarge,
			size,
			MaxFrameSize,
		)
	}

	payload := make([]byte, size)

	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}

	return payload, nil
}

// WriteFrame escreve um quadro composto por cabeçalho e payload.
func WriteFrame(w io.Writer, payload []byte) error {
	size := len(payload)

	if size == 0 {
		return ErrInvalidFrameSize
	}

	if uint64(size) > uint64(MaxFrameSize) {
		return fmt.Errorf(
			"%w: received %d bytes, maximum is %d",
			ErrFrameTooLarge,
			size,
			MaxFrameSize,
		)
	}

	var header [HeaderSize]byte

	binary.BigEndian.PutUint32(header[:], uint32(size))

	if err := writeFull(w, header[:]); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}

	if err := writeFull(w, payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}

	return nil
}

// writeFull escreve todos os bytes, tratando escritas parciais.
func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)

		if n > 0 {
			data = data[n:]
		}

		if err != nil {
			return err
		}

		if n == 0 {
			return io.ErrShortWrite
		}
	}

	return nil
}
