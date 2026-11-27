package main

import (
	"fmt"
	"math"
	"strings"

	memorydbpb "github.com/pescador95/memorydb/protocol/generated/go/memorydb/v1"
	"github.com/pescador95/memorydb/server/resources"
	transporttcp "github.com/pescador95/memorydb/transport/tcp/go"
	"google.golang.org/protobuf/proto"
)

// validateProtocolFrameLimits verifica se os maiores comandos e respostas
// permitidos pela configuração cabem no limite de payload de um frame TCP.
func validateProtocolFrameLimits(config resources.ResourceConfig) error {
	maxFrameSize := config.Network.MaxFramePayloadSize
	maxKeySize := config.Storage.MaxKeySize
	maxValueSize := config.Storage.MaxValueSize

	if maxFrameSize <= 0 {
		return fmt.Errorf("maximum frame payload size must be positive")
	}

	if maxFrameSize > int64(transporttcp.MaxFrameSize) {
		return fmt.Errorf(
			"maximum frame payload size %d exceeds transport limit %d",
			maxFrameSize,
			transporttcp.MaxFrameSize,
		)
	}

	if maxKeySize <= 0 {
		return fmt.Errorf("maximum key size must be positive")
	}

	if maxValueSize <= 0 {
		return fmt.Errorf("maximum value size must be positive")
	}

	if maxKeySize > maxFrameSize {
		return fmt.Errorf(
			"maximum key size %d exceeds maximum frame payload size %d",
			maxKeySize,
			maxFrameSize,
		)
	}

	if maxValueSize > maxFrameSize {
		return fmt.Errorf(
			"maximum value size %d exceeds maximum frame payload size %d",
			maxValueSize,
			maxFrameSize,
		)
	}

	// Os tamanhos já estão limitados pelo frame máximo do transporte.
	// Portanto, as conversões para int e as alocações abaixo são limitadas.
	key := strings.Repeat("k", int(maxKeySize))
	value := make([]byte, int(maxValueSize))
	operationID := make([]byte, 16)

	const maxUint64 = uint64(math.MaxUint64)

	requestID := maxUint64

	setRequest := &memorydbpb.Request{
		RequestId: requestID,
		Command: &memorydbpb.Request_Set{
			Set: &memorydbpb.SetCommand{
				Key:         key,
				Value:       value,
				TtlMs:       maxUint64,
				OperationId: operationID,
			},
		},
	}

	getRequest := &memorydbpb.Request{
		RequestId: requestID,
		Command: &memorydbpb.Request_Get{
			Get: &memorydbpb.GetCommand{
				Key: key,
			},
		},
	}

	deleteRequest := &memorydbpb.Request{
		RequestId: requestID,
		Command: &memorydbpb.Request_Delete{
			Delete: &memorydbpb.DeleteCommand{
				Key:         key,
				OperationId: operationID,
			},
		},
	}

	getResponse := &memorydbpb.Response{
		RequestId: requestID,
		Success:   true,
		Result: &memorydbpb.Response_Get{
			Get: &memorydbpb.GetResult{
				Found: true,
				Value: value,
			},
		},
	}

	messageSizes := []struct {
		name string
		size int
	}{
		{
			name: "SET request",
			size: proto.Size(setRequest),
		},
		{
			name: "GET request",
			size: proto.Size(getRequest),
		},
		{
			name: "DELETE request",
			size: proto.Size(deleteRequest),
		},
		{
			name: "GET response",
			size: proto.Size(getResponse),
		},
	}

	var incompatibleMessages []string

	for _, message := range messageSizes {
		if int64(message.size) > maxFrameSize {
			incompatibleMessages = append(
				incompatibleMessages,
				fmt.Sprintf(
					"%s requires %d bytes",
					message.name,
					message.size,
				),
			)
		}
	}

	if len(incompatibleMessages) > 0 {
		return fmt.Errorf(
			"maximum configured messages exceed frame payload limit of %d bytes: %s",
			maxFrameSize,
			strings.Join(incompatibleMessages, "; "),
		)
	}

	return nil
}
