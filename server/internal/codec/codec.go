package codec

import "github.com/pescador95/memorydb/server/internal/protocol"

// Codec define o contrato para codificação e decodificação de mensagens.
type Codec interface {
	EncodeRequest(request *protocol.Request) ([]byte, error)
	DecodeRequest(data []byte) (*protocol.Request, error)

	EncodeResponse(response *protocol.Response) ([]byte, error)
	DecodeResponse(data []byte) (*protocol.Response, error)
}
