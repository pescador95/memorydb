package tcp

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func TestNewConnectionRejectsNil(t *testing.T) {
	_, err := NewConnection(nil)

	if err == nil {
		t.Fatal("NewConnection(nil) expected error, got nil")
	}
}

func TestNewConnectionRejectsTypedNil(t *testing.T) {
	var tcpConn *net.TCPConn
	var conn net.Conn = tcpConn

	_, err := NewConnection(conn)

	if err == nil {
		t.Fatal("NewConnection(typed nil) expected error, got nil")
	}
}

func TestConnectionMethodsRejectNilReceiver(t *testing.T) {
	var conn *Connection

	if _, err := conn.ReadFrame(); err == nil {
		t.Fatal("ReadFrame() expected error for nil receiver")
	}

	if err := conn.WriteFrame([]byte("test")); err == nil {
		t.Fatal("WriteFrame() expected error for nil receiver")
	}

	if err := conn.Close(); err == nil {
		t.Fatal("Close() expected error for nil receiver")
	}
}

func TestConnectionCloseInterruptsBlockedRead(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()

	conn, err := NewConnection(local)
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)

	go func() {
		_, err := conn.ReadFrame()
		result <- err
	}()

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("ReadFrame() expected error after connection close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadFrame() remained blocked after connection close")
	}
}

func TestConnectionCloseInterruptsBlockedWrite(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()

	conn, err := NewConnection(local)
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)

	go func() {
		result <- conn.WriteFrame([]byte("blocked write"))
	}()

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("WriteFrame() expected error after connection close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WriteFrame() remained blocked after connection close")
	}
}

func TestConnectionConcurrentWrites(t *testing.T) {
	const count = 50

	local, remote := net.Pipe()
	defer remote.Close()

	conn, err := NewConnection(local)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := remote.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	readResults := make(chan []byte, count)
	readErrors := make(chan error, 1)

	go func() {
		for i := 0; i < count; i++ {
			payload, err := ReadFrame(remote)
			if err != nil {
				readErrors <- err
				return
			}
			readResults <- payload
		}
	}()

	var wg sync.WaitGroup
	writeErrors := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			payload := []byte(fmt.Sprintf("message-%d", i))
			if err := conn.WriteFrame(payload); err != nil {
				writeErrors <- err
			}
		}(i)
	}

	wg.Wait()
	close(writeErrors)

	for err := range writeErrors {
		t.Errorf("WriteFrame() error = %v", err)
	}

	received := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		select {
		case err := <-readErrors:
			t.Fatalf("ReadFrame() error = %v", err)
		case payload := <-readResults:
			key := string(payload)

			if received[key] {
				t.Errorf("duplicate frame received: %s", key)
			}

			received[key] = true
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for frames")
		}
	}

	if len(received) != count {
		t.Fatalf("received %d frames, expected %d", len(received), count)
	}
}

func TestConnectionConcurrentReads(t *testing.T) {
	const count = 50

	local, remote := net.Pipe()
	defer remote.Close()

	conn, err := NewConnection(local)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := remote.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan []byte, count)
	readErrors := make(chan error, count)

	for i := 0; i < count; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			payload, err := conn.ReadFrame()
			if err != nil {
				readErrors <- err
				return
			}

			results <- payload
		}()
	}

	writeErrors := make(chan error, 1)

	go func() {
		for i := 0; i < count; i++ {
			payload := []byte(fmt.Sprintf("message-%d", i))

			if err := WriteFrame(remote, payload); err != nil {
				writeErrors <- err
				return
			}
		}
	}()

	wg.Wait()

	select {
	case err := <-writeErrors:
		t.Fatalf("WriteFrame() error = %v", err)
	default:
	}

	close(readErrors)

	for err := range readErrors {
		t.Errorf("ReadFrame() error = %v", err)
	}

	received := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		select {
		case payload := <-results:
			key := string(payload)

			if received[key] {
				t.Errorf("duplicate frame received: %s", key)
			}

			received[key] = true
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for frames")
		}
	}

	if len(received) != count {
		t.Fatalf("received %d frames, expected %d", len(received), count)
	}
}

func TestConnectionFullDuplex(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()

	conn, err := NewConnection(local)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	readResult := make(chan []byte, 1)
	readError := make(chan error, 1)
	writeResult := make(chan error, 1)

	go func() {
		payload, err := conn.ReadFrame()
		if err != nil {
			readError <- err
			return
		}
		readResult <- payload
	}()

	go func() {
		writeResult <- conn.WriteFrame([]byte("request"))
	}()

	if err := remote.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	received, err := ReadFrame(remote)
	if err != nil {
		t.Fatalf("remote ReadFrame() error = %v", err)
	}

	if string(received) != "request" {
		t.Fatalf("received = %q, want %q", received, "request")
	}

	if err := WriteFrame(remote, []byte("response")); err != nil {
		t.Fatalf("remote WriteFrame() error = %v", err)
	}

	select {
	case err := <-writeResult:
		if err != nil {
			t.Fatalf("WriteFrame() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WriteFrame() remained blocked")
	}

	select {
	case err := <-readError:
		t.Fatalf("ReadFrame() error = %v", err)
	case payload := <-readResult:
		if string(payload) != "response" {
			t.Fatalf("received = %q, want %q", payload, "response")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFrame() remained blocked")
	}
}
