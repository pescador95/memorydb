package memorydb

import (
	"crypto/rand"
)

const OperationIDSize = 16

// OperationID identifica uma operação lógica de mutação.
// O mesmo identificador deve ser reutilizado em tentativas da mesma operação.
type OperationID [OperationIDSize]byte

// NewOperationID gera um identificador aleatório criptograficamente seguro.
func NewOperationID() (OperationID, error) {
	var id OperationID

	if _, err := rand.Read(id[:]); err != nil {
		return OperationID{}, err
	}

	return id, nil
}
