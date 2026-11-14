package protobuf

import (
	"fmt"

	memorydbpb "github.com/pescador95/memorydb/api/proto"
	"github.com/pescador95/memorydb/internal/codec"
	"github.com/pescador95/memorydb/internal/protocol"
	"google.golang.org/protobuf/proto"
)

// Codec implementa a serialização de mensagens usando Protocol Buffers.
type Codec struct{}

// New cria uma implementação do codec Protobuf.
func New() *Codec {
	return &Codec{}
}

// Garante que Codec implementa a interface de serialização.
var _ codec.Codec = (*Codec)(nil)

// EncodeRequest converte uma solicitação interna para bytes Protobuf.
func (c *Codec) EncodeRequest(request *protocol.Request) ([]byte, error) {
	if request == nil {
		return nil, fmt.Errorf("encode request: request cannot be nil")
	}

	message := &memorydbpb.Request{
		RequestId: request.RequestID,
	}

	switch command := request.Command.(type) {
	case *protocol.PingCommand:
		message.Command = &memorydbpb.Request_Ping{
			Ping: &memorydbpb.PingCommand{},
		}
	case *protocol.SetCommand:
		message.Command = &memorydbpb.Request_Set{
			Set: &memorydbpb.SetCommand{
				Key:   command.Key,
				Value: command.Value,
				TtlMs: command.TTLMillis,
			},
		}
	case *protocol.GetCommand:
		message.Command = &memorydbpb.Request_Get{
			Get: &memorydbpb.GetCommand{Key: command.Key},
		}
	case *protocol.DeleteCommand:
		message.Command = &memorydbpb.Request_Delete{
			Delete: &memorydbpb.DeleteCommand{Key: command.Key},
		}
	default:
		return nil, fmt.Errorf("encode request: unsupported or missing command")
	}

	data, err := proto.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	return data, nil
}

// DecodeRequest converte bytes Protobuf para uma solicitação interna.
func (c *Codec) DecodeRequest(data []byte) (*protocol.Request, error) {
	message := &memorydbpb.Request{}

	if err := proto.Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}

	request := &protocol.Request{
		RequestID: message.GetRequestId(),
	}

	switch command := message.GetCommand().(type) {
	case *memorydbpb.Request_Ping:
		request.Command = &protocol.PingCommand{}
	case *memorydbpb.Request_Set:
		request.Command = &protocol.SetCommand{
			Key:       command.Set.GetKey(),
			Value:     command.Set.GetValue(),
			TTLMillis: command.Set.GetTtlMs(),
		}
	case *memorydbpb.Request_Get:
		request.Command = &protocol.GetCommand{
			Key: command.Get.GetKey(),
		}
	case *memorydbpb.Request_Delete:
		request.Command = &protocol.DeleteCommand{
			Key: command.Delete.GetKey(),
		}
	default:
		return nil, fmt.Errorf("decode request: unsupported or missing command")
	}

	return request, nil
}

// EncodeResponse converte uma resposta interna para bytes Protobuf.
func (c *Codec) EncodeResponse(response *protocol.Response) ([]byte, error) {
	if response == nil {
		return nil, fmt.Errorf("encode response: response cannot be nil")
	}

	message := &memorydbpb.Response{
		RequestId: response.RequestID,
		Success:   response.Success,
	}

	switch result := response.Result.(type) {
	case *protocol.PongResult:
		message.Result = &memorydbpb.Response_Pong{
			Pong: &memorydbpb.PongResult{},
		}
	case *protocol.SetResult:
		message.Result = &memorydbpb.Response_Set{
			Set: &memorydbpb.SetResult{},
		}
	case *protocol.GetResult:
		message.Result = &memorydbpb.Response_Get{
			Get: &memorydbpb.GetResult{
				Found: result.Found,
				Value: result.Value,
			},
		}
	case *protocol.DeleteResult:
		message.Result = &memorydbpb.Response_Delete{
			Delete: &memorydbpb.DeleteResult{
				Deleted: result.Deleted,
			},
		}
	case *protocol.ErrorResult:
		message.Result = &memorydbpb.Response_Error{
			Error: &memorydbpb.ErrorResult{
				Code:    result.Code,
				Message: result.Message,
			},
		}
	default:
		return nil, fmt.Errorf("encode response: unsupported or missing result")
	}

	data, err := proto.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode response: %w", err)
	}

	return data, nil
}

// DecodeResponse converte bytes Protobuf para uma resposta interna.
func (c *Codec) DecodeResponse(data []byte) (*protocol.Response, error) {
	message := &memorydbpb.Response{}

	if err := proto.Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	response := &protocol.Response{
		RequestID: message.GetRequestId(),
		Success:   message.GetSuccess(),
	}

	switch result := message.GetResult().(type) {
	case *memorydbpb.Response_Pong:
		response.Result = &protocol.PongResult{}
	case *memorydbpb.Response_Set:
		response.Result = &protocol.SetResult{}
	case *memorydbpb.Response_Get:
		response.Result = &protocol.GetResult{
			Found: result.Get.GetFound(),
			Value: result.Get.GetValue(),
		}
	case *memorydbpb.Response_Delete:
		response.Result = &protocol.DeleteResult{
			Deleted: result.Delete.GetDeleted(),
		}
	case *memorydbpb.Response_Error:
		response.Result = &protocol.ErrorResult{
			Code:    result.Error.GetCode(),
			Message: result.Error.GetMessage(),
		}
	default:
		return nil, fmt.Errorf("decode response: unsupported or missing result")
	}

	return response, nil
}
