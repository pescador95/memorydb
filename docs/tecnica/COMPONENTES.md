# Catálogo de Componentes

Este catálogo descreve os componentes presentes no código atual. As classificações de camada indicam responsabilidades observadas, não pacotes formais Clean Architecture.

| Nome | Camada | Tipo | Arquivo | Descrição | Depende de |
|---|---|---|---|---|---|
| `Request`, `Response`, comandos e resultados | Modelo interno | Modelo / contrato | [internal/protocol/message.go](../../internal/protocol/message.go) | Tipos internos independentes do transporte e dos tipos gerados Protobuf. | Biblioteca padrão Go |
| `Engine` | Aplicação / núcleo | Serviço de aplicação (sem Port de storage) | [internal/engine/engine.go](../../internal/engine/engine.go) | Valida e executa PING, SET, GET e DELETE, construindo respostas internas e erros. | `protocol`; implementação concreta `storage.Storage` |
| Códigos e resposta de erro | Aplicação / núcleo | Constantes / helper | [internal/engine/command.go](../../internal/engine/command.go) | Define `INVALID_REQUEST`, `INVALID_ARGUMENT`, `UNSUPPORTED_FEATURE` e helper de resposta. | `protocol` |
| `Storage` | Infraestrutura | Adaptador | [internal/storage/storage.go](../../internal/storage/storage.go) | Mapa chave-valor em memória protegido por `sync.RWMutex`; copia valores na entrada e saída. | Biblioteca padrão `sync`; `Entry` |
| `Entry` | Modelo de armazenamento | Estrutura de dados | [internal/storage/entry.go](../../internal/storage/entry.go) | Guarda bytes de um valor; não representa TTL. | Biblioteca padrão Go |
| `Codec` | Port / contrato | Port | [internal/codec/codec.go](../../internal/codec/codec.go) | Contrato de encode/decode para solicitações e respostas internas. | `protocol` |
| `protobuf.Codec` | Adaptadores | Adapter | [internal/codec/protobuf/codec.go](../../internal/codec/protobuf/codec.go) | Faz conversão entre protocolo interno e mensagens Protobuf. | `codec.Codec`, `protocol`, tipos gerados e `google.golang.org/protobuf` |
| Contrato Protobuf | Integração externa | Contrato de protocolo | [api/proto/memorydb.proto](../../api/proto/memorydb.proto) | Define envelopes, comandos, resultados, `request_id`, bytes, TTL e erros. | Protocol Buffers |
| Código Go Protobuf gerado | Integração externa | Código gerado | [api/proto/memorydb.pb.go](../../api/proto/memorydb.pb.go) | Tipos Go gerados a partir do schema externo. | `memorydb.proto`, runtime Protobuf |
| `ReadFrame` / `WriteFrame` | Transporte | Adapter / framing | [internal/transport/tcp/framing.go](../../internal/transport/tcp/framing.go) | Lê/escreve frames com prefixo de tamanho Big Endian e máximo de 1 MiB. | `encoding/binary`, `io` |
| `Connection` | Transporte | Adapter | [internal/transport/tcp/connection.go](../../internal/transport/tcp/connection.go) | Encapsula leitura/escrita de mensagens em `net.Conn`, serializando acesso com mutexes. | `net.Conn`, `codec.Codec`, framing, `protocol` |
| `Handler` | Transporte / aplicação | Adapter / dispatcher | [internal/transport/tcp/handler.go](../../internal/transport/tcp/handler.go) | Lê requisições em loop, chama o engine e devolve respostas. | `engine.Engine`, `codec.Codec`, `Connection` |
| `Server` | Transporte / apresentação | Adapter / servidor | [internal/transport/tcp/server.go](../../internal/transport/tcp/server.go) | Aceita clientes concorrentes e controla shutdown do listener e espera das conexões. | `net.Listener`, `Handler`, `context`, `sync` |
| Executável do servidor | Apresentação / composição | Entry point | [cmd/memorydb/main.go](../../cmd/memorydb/main.go) | Configura endereço, dependências, sinais e ciclo de vida do servidor. | Codec Protobuf, engine, TCP, biblioteca padrão |
| Executável cliente | Apresentação / demonstração | Entry point / cliente | [cmd/client/main.go](../../cmd/client/main.go) | Conecta ao endereço local e demonstra operações; verifica IDs e respostas. | Codec Protobuf, `protocol`, `Connection` |

## Interfaces e contratos públicos relevantes

| Interface ou método | Assinatura / comportamento | Evidência |
|---|---|---|
| `codec.Codec` | `EncodeRequest(*protocol.Request) ([]byte, error)`; `DecodeRequest([]byte) (*protocol.Request, error)`; `EncodeResponse(*protocol.Response) ([]byte, error)`; `DecodeResponse([]byte) (*protocol.Response, error)` | [internal/codec/codec.go](../../internal/codec/codec.go) |
| `Engine.Execute` | `Execute(*protocol.Request) *protocol.Response` | [internal/engine/engine.go](../../internal/engine/engine.go) |
| `Storage` | `Set(string, []byte)`; `Get(string) ([]byte, bool)`; `Delete(string) bool` | [internal/storage/storage.go](../../internal/storage/storage.go) |
| `Connection` | `ReadRequest() (*protocol.Request, error)`; `WriteRequest(*protocol.Request) error`; `ReadResponse() (*protocol.Response, error)`; `WriteResponse(*protocol.Response) error`; `Close() error` | [internal/transport/tcp/connection.go](../../internal/transport/tcp/connection.go) |
| `Handler.Handle` | `Handle(net.Conn) error` | [internal/transport/tcp/handler.go](../../internal/transport/tcp/handler.go) |
| `Server` | `Serve() error`; `Shutdown(context.Context) error` | [internal/transport/tcp/server.go](../../internal/transport/tcp/server.go) |

O único Port explícito observado é `codec.Codec`. O armazenamento é consumido por meio de uma implementação concreta; não há interface `Repository`/`Storage` no estado atual. Evidências: [codec.go](../../internal/codec/codec.go), [engine.go](../../internal/engine/engine.go).
