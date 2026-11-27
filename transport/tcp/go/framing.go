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

	// ErrInvalidMaximumFrameSize indica que o limite configurado é inválido.
	ErrInvalidMaximumFrameSize = errors.New("maximum frame size is invalid")
)

// ReadFrame lê um quadro delimitado por um cabeçalho de tamanho.
//
// A função mantém compatibilidade com os consumidores existentes.
// Para controlar recursos antes da alocação do payload, utilize
// ReadFrameWithReservation.
func ReadFrame(r io.Reader) ([]byte, error) {
	payload, release, err := ReadFrameWithReservation(
		r,
		MaxFrameSize,
		nil,
	)
	if err != nil {
		return nil, err
	}

	if release != nil {
		release()
	}

	return payload, nil
}

// ReadFrameWithReservation lê um quadro após validar seu tamanho e obter
// uma reserva opcional de recursos.
//
// A função reserve é chamada depois da leitura e validação do cabeçalho,
// mas antes da alocação do payload.
//
// Quando reserve retorna erro, o payload não é alocado e qualquer reserva
// acompanhada de uma função release é liberada automaticamente.
//
// Quando a leitura do payload falha, a reserva também é liberada.
// Quando a leitura termina com sucesso, a responsabilidade pela liberação
// é transferida ao chamador.
//
// O chamador deve executar release exatamente uma vez após terminar de
// utilizar o payload e os recursos associados ao processamento.
func ReadFrameWithReservation(
	r io.Reader,
	maxSize uint32,
	reserve func(size uint32) (release func(), err error),
) ([]byte, func(), error) {
	if r == nil {
		return nil, nil, fmt.Errorf("reader cannot be nil")
	}

	if maxSize == 0 || maxSize > MaxFrameSize {
		return nil, nil, fmt.Errorf(
			"%w: received %d, supported range is 1..%d",
			ErrInvalidMaximumFrameSize,
			maxSize,
			MaxFrameSize,
		)
	}

	var header [HeaderSize]byte

	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	size := binary.BigEndian.Uint32(header[:])

	if size == 0 {
		return nil, nil, ErrInvalidFrameSize
	}

	if size > maxSize {
		return nil, nil, fmt.Errorf(
			"%w: received %d bytes, maximum is %d",
			ErrFrameTooLarge,
			size,
			maxSize,
		)
	}

	var release func()

	if reserve != nil {
		reservedRelease, reserveErr := reserve(size)

		if reserveErr != nil {
			if reservedRelease != nil {
				reservedRelease()
			}

			return nil, nil, fmt.Errorf(
				"reserve frame resources: %w",
				reserveErr,
			)
		}

		if reservedRelease == nil {
			return nil, nil, fmt.Errorf(
				"reserve frame resources: release function cannot be nil",
			)
		}

		release = reservedRelease
	}

	payload := make([]byte, size)

	if _, err := io.ReadFull(r, payload); err != nil {
		if release != nil {
			release()
		}

		return nil, nil, fmt.Errorf("failed to read frame payload: %w", err)
	}

	return payload, release, nil
}

// WriteFrame escreve um quadro composto por cabeçalho e payload.
func WriteFrame(w io.Writer, payload []byte) error {
	size := len(payload)

	if w == nil {
		return fmt.Errorf("writer cannot be nil")
	}

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

		if n < 0 || n > len(data) {
			return fmt.Errorf("invalid write count: %d", n)
		}

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
