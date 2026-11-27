package idempotency

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/pescador95/memorydb/server/internal/protocol"
)

var fingerprintBenchmarkResult [32]byte

// legacyFingerprintForBenchmark reproduz a implementação anterior,
// que construía um buffer completo antes de calcular o SHA-256.
//
// É utilizada exclusivamente para comparação de desempenho.
func legacyFingerprintForBenchmark(command *protocol.SetCommand) [32]byte {
	data := make([]byte, 0, 128)
	data = append(data, fingerprintVersion...)
	data = append(data, setCommandMarker)

	data = legacyAppendBytesField(data, []byte(command.Key))
	data = legacyAppendBytesField(data, command.Value)
	data = legacyAppendUint64(data, command.TTLMillis)

	return sha256.Sum256(data)
}

func legacyAppendBytesField(data []byte, value []byte) []byte {
	data = legacyAppendUint64(data, uint64(len(value)))
	return append(data, value...)
}

func legacyAppendUint64(data []byte, value uint64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	return append(data, encoded[:]...)
}

func BenchmarkFingerprint(b *testing.B) {
	sizes := []int{
		1 << 10,
		64 << 10,
		256 << 10,
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("ValueSize_%dKiB", size>>10), func(b *testing.B) {
			command := &protocol.SetCommand{
				Key:       "customer:benchmark",
				Value:     make([]byte, size),
				TTLMillis: 0,
			}

			b.Run("Current", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(size))

				for i := 0; i < b.N; i++ {
					fingerprint, err := Fingerprint(command)
					if err != nil {
						b.Fatal(err)
					}

					fingerprintBenchmarkResult = fingerprint
				}
			})

			b.Run("Previous", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(size))

				for i := 0; i < b.N; i++ {
					fingerprintBenchmarkResult = legacyFingerprintForBenchmark(command)
				}
			})
		})
	}
}
