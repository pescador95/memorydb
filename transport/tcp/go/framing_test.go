package tcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

type chunkReader struct {
	reader io.Reader
	chunk  int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(p) > r.chunk {
		p = p[:r.chunk]
	}

	return r.reader.Read(p)
}

type chunkWriter struct {
	buffer bytes.Buffer
	chunk  int
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	if len(p) > w.chunk {
		p = p[:w.chunk]
	}

	return w.buffer.Write(p)
}

type zeroWriter struct{}

func (w *zeroWriter) Write(p []byte) (int, error) {
	return 0, nil
}

type payloadErrorWriter struct {
	calls int
	err   error
}

func (w *payloadErrorWriter) Write(p []byte) (int, error) {
	w.calls++

	if w.calls == 1 {
		return len(p), nil
	}

	return 0, w.err
}

type invalidWriteCountWriter struct{}

func (w *invalidWriteCountWriter) Write(p []byte) (int, error) {
	return len(p) + 1, nil
}

func TestWriteAndReadFrame(t *testing.T) {
	payload := []byte("memorydb test")

	var buffer bytes.Buffer

	if err := WriteFrame(&buffer, payload); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	got, err := ReadFrame(&buffer)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}
}

func TestFrameHeaderUsesBigEndian(t *testing.T) {
	payload := []byte("test")

	var buffer bytes.Buffer

	if err := WriteFrame(&buffer, payload); err != nil {
		t.Fatal(err)
	}

	header := buffer.Bytes()[:HeaderSize]
	size := binary.BigEndian.Uint32(header)

	if size != uint32(len(payload)) {
		t.Fatalf("header size = %d, want %d", size, len(payload))
	}
}

func TestReadFrameWithFragmentedInput(t *testing.T) {
	payload := []byte("fragmented tcp message")

	var buffer bytes.Buffer

	if err := WriteFrame(&buffer, payload); err != nil {
		t.Fatal(err)
	}

	reader := &chunkReader{
		reader: &buffer,
		chunk:  2,
	}

	got, err := ReadFrame(reader)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}
}

func TestReadFrameRejectsZeroSize(t *testing.T) {
	var buffer bytes.Buffer

	if err := binary.Write(&buffer, binary.BigEndian, uint32(0)); err != nil {
		t.Fatal(err)
	}

	_, err := ReadFrame(&buffer)

	if !errors.Is(err, ErrInvalidFrameSize) {
		t.Fatalf("error = %v, want ErrInvalidFrameSize", err)
	}
}

func TestReadFrameRejectsOversizedFrame(t *testing.T) {
	var buffer bytes.Buffer

	if err := binary.Write(&buffer, binary.BigEndian, MaxFrameSize+1); err != nil {
		t.Fatal(err)
	}

	_, err := ReadFrame(&buffer)

	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("error = %v, want ErrFrameTooLarge", err)
	}
}

func TestReadFrameRejectsIncompleteHeader(t *testing.T) {
	reader := bytes.NewReader([]byte{0x00, 0x00})

	_, err := ReadFrame(reader)

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrameRejectsIncompletePayload(t *testing.T) {
	var buffer bytes.Buffer

	_ = binary.Write(&buffer, binary.BigEndian, uint32(10))
	buffer.Write([]byte("short"))

	_, err := ReadFrame(&buffer)

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestWriteFrameRejectsEmptyPayload(t *testing.T) {
	var buffer bytes.Buffer

	err := WriteFrame(&buffer, nil)

	if !errors.Is(err, ErrInvalidFrameSize) {
		t.Fatalf("error = %v, want ErrInvalidFrameSize", err)
	}
}

func TestWriteFrameHandlesPartialWrites(t *testing.T) {
	payload := []byte("partial write test")

	writer := &chunkWriter{
		chunk: 2,
	}

	if err := WriteFrame(writer, payload); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	reader := bytes.NewReader(writer.buffer.Bytes())

	got, err := ReadFrame(reader)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", got, payload)
	}
}

func TestWriteFrameRejectsWriterWithoutProgress(t *testing.T) {
	writer := &zeroWriter{}

	err := WriteFrame(writer, []byte("test"))

	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v, want io.ErrShortWrite", err)
	}
}

func TestReadFrameRejectsNilReader(t *testing.T) {
	_, err := ReadFrame(nil)

	if err == nil {
		t.Fatal("ReadFrame(nil) expected error, got nil")
	}
}

func TestWriteFrameRejectsNilWriter(t *testing.T) {
	err := WriteFrame(nil, []byte("test"))

	if err == nil {
		t.Fatal("WriteFrame(nil) expected error, got nil")
	}
}

func TestWriteAndReadFrameWithMaximumSize(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), int(MaxFrameSize))

	var buffer bytes.Buffer

	if err := WriteFrame(&buffer, payload); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	got, err := ReadFrame(&buffer)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatal("payload mismatch for maximum frame size")
	}
}

func TestWriteFrameRejectsOversizedPayload(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), int(MaxFrameSize)+1)

	var buffer bytes.Buffer

	err := WriteFrame(&buffer, payload)

	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("error = %v, want ErrFrameTooLarge", err)
	}

	if buffer.Len() != 0 {
		t.Fatalf("writer received %d bytes, want 0", buffer.Len())
	}
}

func TestWriteFrameReturnsPayloadWriteError(t *testing.T) {
	expectedErr := errors.New("payload write failed")
	writer := &payloadErrorWriter{err: expectedErr}

	err := WriteFrame(writer, []byte("test"))

	if !errors.Is(err, expectedErr) {
		t.Fatalf("error = %v, want %v", err, expectedErr)
	}

	if writer.calls != 2 {
		t.Fatalf("write calls = %d, want 2", writer.calls)
	}
}

func TestWriteFrameRejectsInvalidWriteCount(t *testing.T) {
	writer := &invalidWriteCountWriter{}

	err := WriteFrame(writer, []byte("test"))

	if err == nil {
		t.Fatal("WriteFrame() expected error for invalid write count")
	}

	if !strings.Contains(err.Error(), "invalid write count") {
		t.Fatalf("error = %v, want invalid write count error", err)
	}
}
