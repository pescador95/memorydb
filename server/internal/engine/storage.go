package engine

// Storage define as operações de armazenamento utilizadas pelo Engine.
//
// As operações podem retornar erros para que limites de capacidade,
// tamanho e memória sejam comunicados ao cliente sem acoplar o Engine
// à implementação concreta do armazenamento.
type Storage interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, bool, error)
	Delete(key string) (bool, error)
}

// New cria uma instância do mecanismo de execução com o armazenamento fornecido.
func New(storage Storage) *Engine {
	return &Engine{
		storage: storage,
	}
}
