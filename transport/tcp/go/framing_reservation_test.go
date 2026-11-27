package tcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func frameBytes(payload []byte) []byte {
	frame := make([]byte, HeaderSize+len(payload))
	binary.BigEndian.PutUint32(frame[:HeaderSize], uint32(len(payload)))
	copy(frame[HeaderSize:], payload)
	return frame
}

func TestReadFrameWithReservationReservesBeforeReadingPayload(t *testing.T) {
	payload := []byte("memorydb")
	reader := bytes.NewReader(frameBytes(payload))

	reserveCalls := 0
	releaseCalls := 0

	got, release, err := ReadFrameWithReservation(
		reader,
		MaxFrameSize,
		func(size uint32) (func(), error) {
			reserveCalls++

			if size != uint32(len(payload)) {
				t.Fatalf("reserved size = %d, want %d", size, len(payload))
			}

			if reader.Len() != len(payload) {
				t.Fatalf(
					"remaining bytes during reservation = %d, want %d",
					reader.Len(),
					len(payload),
				)
			}

			return func() {
				releaseCalls++
			}, nil
		},
	)
	if err != nil {
		t.Fatalf("ReadFrameWithReservation() error = %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload = %q, want %q", got, payload)
	}

	if reserveCalls != 1 {
		t.Errorf("reserve calls = %d, want 1", reserveCalls)
	}

	if releaseCalls != 0 {
		t.Fatalf("release calls before caller release = %d, want 0", releaseCalls)
	}

	if release == nil {
		t.Fatal("release function is nil")
	}

	release()

	if releaseCalls != 1 {
		t.Errorf("release calls = %d, want 1", releaseCalls)
	}
}

func TestReadFrameWithReservationRejectsBeforePayloadRead(t *testing.T) {
	payload := []byte("memorydb")
	reader := bytes.NewReader(frameBytes(payload))
	sentinel := errors.New("processing capacity reached")

	reserveCalls := 0

	got, release, err := ReadFrameWithReservation(
		reader,
		MaxFrameSize,
		func(uint32) (func(), error) {
			reserveCalls++
			return nil, sentinel
		},
	)

	if got != nil {
		t.Errorf("payload = %q, want nil", got)
	}

	if release != nil {
		t.Error("release function should be nil")
	}

	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}

	if reserveCalls != 1 {
		t.Errorf("reserve calls = %d, want 1", reserveCalls)
	}

	if reader.Len() != len(payload) {
		t.Errorf(
			"remaining bytes = %d, want %d; payload should not have been read",
			reader.Len(),
			len(payload),
		)
	}
}

func TestReadFrameWithReservationReleasesWhenPayloadReadFails(t *testing.T) {
	fullFrame := frameBytes([]byte("memorydb"))
	truncatedFrame := fullFrame[:HeaderSize+2]
	reader := bytes.NewReader(truncatedFrame)

	releaseCalls := 0

	_, release, err := ReadFrameWithReservation(
		reader,
		MaxFrameSize,
		func(uint32) (func(), error) {
			return func() {
				releaseCalls++
			}, nil
		},
	)

	if err == nil {
		t.Fatal("expected payload read error")
	}

	if release != nil {
		t.Error("release function should not be returned after a failed read")
	}

	if releaseCalls != 1 {
		t.Errorf("release calls = %d, want 1", releaseCalls)
	}

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrameWithReservationRejectsInvalidMaximum(t *testing.T) {
	tests := []struct {
		name    string
		maxSize uint32
	}{
		{
			name:    "zero",
			maxSize: 0,
		},
		{
			name:    "above transport maximum",
			maxSize: MaxFrameSize + 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadFrameWithReservation(
				bytes.NewReader(nil),
				tt.maxSize,
				nil,
			)

			if !errors.Is(err, ErrInvalidMaximumFrameSize) {
				t.Fatalf(
					"error = %v, want %v",
					err,
					ErrInvalidMaximumFrameSize,
				)
			}
		})
	}
}

func TestReadFrameWithReservationRequiresReleaseFunction(t *testing.T) {
	reader := bytes.NewReader(frameBytes([]byte("memorydb")))

	_, release, err := ReadFrameWithReservation(
		reader,
		MaxFrameSize,
		func(uint32) (func(), error) {
			return nil, nil
		},
	)

	if err == nil {
		t.Fatal("expected error when reservation returns a nil release function")
	}

	if release != nil {
		t.Error("release function should be nil")
	}
}
