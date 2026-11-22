package idempotency

import (
	"errors"
	"testing"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

func calculateFingerprint(t *testing.T, command protocol.Command) [32]byte {
	t.Helper()

	fingerprint, err := Fingerprint(command)
	if err != nil {
		t.Fatalf("Fingerprint() error = %v", err)
	}

	return fingerprint
}

func TestFingerprintSetIsDeterministic(t *testing.T) {
	first := calculateFingerprint(t, &protocol.SetCommand{
		Key:       "customer:1",
		Value:     []byte("value"),
		TTLMillis: 0,
	})

	second := calculateFingerprint(t, &protocol.SetCommand{
		Key:       "customer:1",
		Value:     []byte("value"),
		TTLMillis: 0,
	})

	if first != second {
		t.Fatal("identical SET commands produced different fingerprints")
	}
}

func TestFingerprintSetChangesWhenSemanticFieldsChange(t *testing.T) {
	tests := []struct {
		name    string
		command *protocol.SetCommand
	}{
		{
			name: "different key",
			command: &protocol.SetCommand{
				Key:       "customer:2",
				Value:     []byte("value"),
				TTLMillis: 0,
			},
		},
		{
			name: "different value",
			command: &protocol.SetCommand{
				Key:       "customer:1",
				Value:     []byte("other"),
				TTLMillis: 0,
			},
		},
		{
			name: "different TTL",
			command: &protocol.SetCommand{
				Key:       "customer:1",
				Value:     []byte("value"),
				TTLMillis: 1000,
			},
		},
	}

	original := calculateFingerprint(t, &protocol.SetCommand{
		Key:       "customer:1",
		Value:     []byte("value"),
		TTLMillis: 0,
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := calculateFingerprint(t, test.command)

			if got == original {
				t.Fatal("different SET command produced the same fingerprint")
			}
		})
	}
}

func TestFingerprintDeleteIsDeterministic(t *testing.T) {
	first := calculateFingerprint(t, &protocol.DeleteCommand{
		Key: "customer:1",
	})

	second := calculateFingerprint(t, &protocol.DeleteCommand{
		Key: "customer:1",
	})

	if first != second {
		t.Fatal("identical DELETE commands produced different fingerprints")
	}
}

func TestFingerprintDeleteChangesWhenKeyChanges(t *testing.T) {
	first := calculateFingerprint(t, &protocol.DeleteCommand{
		Key: "customer:1",
	})

	second := calculateFingerprint(t, &protocol.DeleteCommand{
		Key: "customer:2",
	})

	if first == second {
		t.Fatal("different DELETE keys produced the same fingerprint")
	}
}

func TestFingerprintDistinguishesCommands(t *testing.T) {
	setFingerprint := calculateFingerprint(t, &protocol.SetCommand{
		Key:   "customer:1",
		Value: []byte("value"),
	})

	deleteFingerprint := calculateFingerprint(t, &protocol.DeleteCommand{
		Key: "customer:1",
	})

	if setFingerprint == deleteFingerprint {
		t.Fatal("SET and DELETE produced the same fingerprint")
	}
}

func TestFingerprintIgnoresOperationID(t *testing.T) {
	first := calculateFingerprint(t, &protocol.SetCommand{
		OperationID: testOperationID(1),
		Key:         "customer:1",
		Value:       []byte("value"),
	})

	second := calculateFingerprint(t, &protocol.SetCommand{
		OperationID: testOperationID(2),
		Key:         "customer:1",
		Value:       []byte("value"),
	})

	if first != second {
		t.Fatal("OperationID must not affect the fingerprint")
	}
}

func TestFingerprintAvoidsConcatenationAmbiguity(t *testing.T) {
	first := calculateFingerprint(t, &protocol.SetCommand{
		Key:   "ab",
		Value: []byte("c"),
	})

	second := calculateFingerprint(t, &protocol.SetCommand{
		Key:   "a",
		Value: []byte("bc"),
	})

	if first == second {
		t.Fatal("different field boundaries produced the same fingerprint")
	}
}

func TestFingerprintRejectsUnsupportedCommands(t *testing.T) {
	tests := []struct {
		name    string
		command protocol.Command
	}{
		{
			name:    "nil command",
			command: nil,
		},
		{
			name:    "PING",
			command: &protocol.PingCommand{},
		},
		{
			name:    "GET",
			command: &protocol.GetCommand{Key: "customer:1"},
		},
		{
			name:    "nil SET",
			command: (*protocol.SetCommand)(nil),
		},
		{
			name:    "nil DELETE",
			command: (*protocol.DeleteCommand)(nil),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Fingerprint(test.command)

			if !errors.Is(err, ErrUnsupportedCommand) {
				t.Fatalf("error = %v, want ErrUnsupportedCommand", err)
			}
		})
	}
}
