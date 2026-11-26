package engine

// Storage define as operações de armazenamento utilizadas pelo Engine.
type Storage interface {
	Set(key string, value []byte)
	Get(key string) ([]byte, bool)
	Delete(key string) bool
}

// New cria uma instância do mecanismo de execução com o armazenamento fornecido.
func New(storage Storage) *Engine {
	return &Engine{
		storage: storage,
	}
}
