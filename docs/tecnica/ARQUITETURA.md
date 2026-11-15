# Arquitetura do MemoryDB

## Visão geral

MemoryDB é um banco de dados chave-valor em memória, implementado inicialmente em Go. O servidor aceita os comandos PING, SET, GET e DELETE por TCP, utilizando Protocol Buffers como formato de serialização das mensagens.

A implementação separa armazenamento, execução de comandos, modelo interno, serialização e transporte. O transporte TCP reutilizável e o driver Go são módulos independentes do servidor. Essa organização permite evoluir os componentes sem acoplar diretamente o mecanismo de execução a um protocolo de rede específico.

O servidor compartilha uma única instância de armazenamento entre as conexões ativas. Os dados existem somente na memória do processo e são perdidos quando o servidor é encerrado.

O projeto utiliza múltiplos módulos Go coordenados por um workspace (`go.work`).

## Estrutura modular

| Módulo | Caminho | Responsabilidade |
|---|---|---|
| Servidor | `server/` | Composição do servidor, engine, storage, codec e integração com TCP |
| Transporte TCP | `transport/tcp/go/` | Framing e abstração de conexão TCP reutilizável |
| Driver Go | `drivers/go/` | Cliente Go para comunicação com o servidor |
| Protocolo gerado | `protocol/generated/go/` | Tipos Go gerados a partir do schema Protobuf |
| Contrato Protobuf | `protocol/proto/` | Definição independente da linguagem do protocolo externo |
| API de demonstração | `api/` | Executável de demonstração que utiliza o driver Go |

O arquivo `go.work`, na raiz do repositório, coordena os módulos durante o desenvolvimento local. Cada módulo possui seu próprio `go.mod`.

## Diagrama de módulos e responsabilidades

```mermaid
flowchart TB
    subgraph Workspace["Workspace Go"]
        API["api/client"]
        Driver["drivers/go"]
        Server["server"]
        TCP["transport/tcp/go"]
        Proto["protocol/generated/go"]
    end

    Schema["protocol/proto"]

    API --> Driver
    Driver --> TCP
    Driver --> Proto
    Server --> TCP
    Server --> Proto
    Proto -. gerado a partir de .-> Schema
```

O servidor e o driver compartilham os módulos de transporte TCP e de tipos Protobuf gerados. O driver não depende dos pacotes internos do servidor.

O contrato Protobuf é mantido separado do código gerado. O arquivo `.proto` é a fonte do contrato; os arquivos gerados não devem ser editados manualmente.

## Diagrama de camadas e dependências internas do servidor

As camadas abaixo representam responsabilidades observadas na implementação. Não significam que o projeto adote formalmente uma estrutura Clean Architecture ou DDD.

```mermaid
flowchart LR
    Client["Cliente / Driver Go"]

    subgraph Server["Módulo server"]
        subgraph Transport["Transporte e entrada"]
            TCPServer["Server"]
            Handler["Handler"]
            Framing["Framing / Connection"]
        end

        subgraph Serialization["Serialização"]
            CodecPort["Port: codec.Codec"]
            ProtoCodec["Adaptador Protobuf"]
        end

        subgraph Application["Execução de comandos"]
            Engine["Engine"]
        end

        subgraph InternalModel["Modelo interno"]
            Protocol["Request / Response / Commands / Results"]
        end

        subgraph StorageLayer["Armazenamento"]
            Storage["Storage em memória"]
            Entry["Entry"]
        end
    end

    ProtoTypes["Tipos Protobuf gerados"]
    TCPModule["Módulo transport/tcp/go"]

    Client --> TCPServer
    TCPServer --> Handler
    Handler --> Framing
    Framing --> TCPModule
    Handler --> CodecPort
    CodecPort -. implementada por .-> ProtoCodec
    ProtoCodec --> ProtoTypes
    Handler --> Engine
    Engine --> Protocol
    Engine --> Storage
    Storage --> Entry
```

O `codec.Codec` desacopla a implementação do handler da serialização concreta. O adaptador Protobuf implementa esse contrato.

O transporte TCP genérico é responsável pelo enquadramento das mensagens e pela comunicação com `net.Conn`. Ele não conhece Protocol Buffers nem o modelo interno do servidor.

O `Engine`, por outro lado, ainda depende diretamente de `*storage.Storage`. Não existe atualmente uma interface de armazenamento que permita substituir a implementação sem modificar o engine.

## Componentes principais

### Modelo interno — `server/internal/protocol`

Representa solicitações e respostas sem depender dos tipos gerados pelo Protobuf ou de detalhes do transporte.

`Request` contém o identificador da solicitação e um comando. `Response` contém o identificador, o estado de sucesso e o resultado.

Os comandos internos representam PING, SET, GET e DELETE. Os resultados representam PONG, SET, GET, DELETE e erro.

Arquivo principal: `server/internal/protocol/message.go`.

### Engine — `server/internal/engine`

O `Engine` executa os comandos e aplica as validações da aplicação.

Comportamentos implementados:

- Requisição nula, comando ausente/desconhecido ou comando tipado nulo resulta em `INVALID_REQUEST`.
- Chave vazia em SET, GET ou DELETE resulta em `INVALID_ARGUMENT`.
- SET com `TTLMillis > 0` resulta em `UNSUPPORTED_FEATURE`.
- SET com TTL igual a zero grava o valor sem expiração.
- GET distingue chave inexistente de chave existente com valor vazio.
- DELETE retorna sucesso e informa se a chave foi removida.
- O `RequestID` da solicitação é preservado na resposta.

O engine cria e mantém diretamente uma instância concreta de `storage.Storage`.

Arquivos principais:
- `server/internal/engine/engine.go`
- `server/internal/engine/command.go`

### Storage — `server/internal/storage`

Armazena os dados em um mapa em memória.

Características:
- Chaves do tipo `string`.
- Valores do tipo `[]byte`.
- Proteção de acesso concorrente com `sync.RWMutex`.
- Cópia dos bytes recebidos em SET.
- Cópia dos bytes retornados em GET.
- Remoção de chaves por DELETE.

A estrutura `Entry` guarda o valor. Não existe atualmente campo de expiração ou mecanismo de persistência.

Arquivos principais:
- `server/internal/storage/storage.go`
- `server/internal/storage/entry.go`

### Codec — `server/internal/codec`

Define a interface `codec.Codec`, responsável por converter o modelo interno em bytes e reconstruir o modelo a partir deles.

Operações:
- `EncodeRequest`
- `DecodeRequest`
- `EncodeResponse`
- `DecodeResponse`

O contrato não exige uma serialização específica. Isso permite implementar outros codecs sem alterar o modelo interno ou a interface consumidora.

Arquivo principal: `server/internal/codec/codec.go`.

### Adaptador Protobuf — `server/internal/codec/protobuf`

Implementa `codec.Codec` utilizando Protocol Buffers.

Converte os tipos internos de solicitação e resposta para os tipos gerados a partir do schema Protobuf e realiza a conversão inversa.

O adaptador depende do módulo `protocol/generated/go`, mas o modelo interno não depende dos tipos gerados.

Arquivo principal: `server/internal/codec/protobuf/codec.go`.

### Transporte TCP reutilizável — `transport/tcp/go`

Módulo independente que implementa o framing e encapsula a conexão TCP.

O protocolo de framing utiliza:
- Prefixo de 4 bytes.
- Tamanho representado por `uint32`.
- Ordem de bytes Big Endian.
- Payload entre 1 byte e 1 MiB.
- Leitura completa com tratamento de fragmentação.
- Escrita com tratamento de escritas parciais.

A abstração `Connection` utiliza mutexes separados para leitura e escrita, permitindo uma operação de leitura e outra de escrita simultaneamente, enquanto serializa operações concorrentes do mesmo tipo.

Arquivos principais:
- `transport/tcp/go/framing.go`
- `transport/tcp/go/connection.go`

Esse módulo não depende de Protobuf, do engine ou do servidor.

### Handler — `server/internal/transport/tcp`

O `Handler` conecta o transporte TCP ao codec e ao engine.

Para cada conexão:
1. Cria a abstração de conexão TCP.
2. Lê um frame.
3. Decodifica a solicitação.
4. Executa o comando no engine.
5. Codifica a resposta.
6. Escreve o frame de resposta.
7. Repete o processo enquanto a conexão permanecer aberta.

Erros de leitura, decodificação, execução de transporte ou escrita encerram o processamento da conexão. Uma falha de decodificação não gera uma resposta estruturada de aplicação.

Arquivo principal: `server/internal/transport/tcp/handler.go`.

### Server — `server/internal/transport/tcp`

O `Server` controla o listener TCP e o ciclo de vida das conexões.

`Serve()` aceita conexões e inicia uma goroutine para processar cada cliente. O servidor mantém um controle das conexões ativas.

`Shutdown(context.Context)` fecha o listener e aguarda as conexões ativas terminarem ou o contexto ser cancelado.

O servidor não fecha diretamente as conexões ativas durante o shutdown. Assim, uma conexão que não termina pode manter a operação aguardando até o cancelamento do contexto.

Arquivo principal: `server/internal/transport/tcp/server.go`.

### Driver Go — `drivers/go`

Biblioteca cliente que permite aplicações Go se conectarem ao MemoryDB sem depender dos pacotes internos do servidor.

Responsabilidades:
- Estabelecer conexão TCP.
- Enviar e receber frames.
- Codificar e decodificar mensagens Protobuf.
- Correlacionar solicitações e respostas por `request_id`.
- Expor operações PING, SET, GET e DELETE.
- Converter respostas de erro do servidor em erros Go.
- Serializar operações concorrentes do cliente no modelo sequencial de requisição/resposta.

A implementação atual não oferece multiplexação de requisições, reconexão automática ou operações com contexto e timeout configurável.

Arquivos principais:
- `drivers/go/client.go`
- `drivers/go/client_test.go`

### API de demonstração — `api/client`

Executável que utiliza o driver Go para demonstrar a comunicação com o servidor.

O executável não implementa outro protocolo de comunicação nem acessa diretamente os componentes internos do servidor. Ele utiliza a API pública do driver.

Arquivo principal: `api/client/main.go`.

### Executável do servidor — `server/cmd/memorydb`

Ponto de composição da aplicação.

Responsabilidades:
- Obter o endereço de escuta da variável `MEMORYDB_ADDR`.
- Utilizar `:6380` como endereço padrão.
- Instanciar o codec Protobuf.
- Instanciar o engine.
- Criar o handler TCP.
- Criar e iniciar o servidor.
- Tratar sinais de encerramento.
- Solicitar shutdown gracioso com timeout de 10 segundos.

Arquivo principal: `server/cmd/memorydb/main.go`.

## Fluxos principais

### SET seguido de GET

```mermaid
sequenceDiagram
    participant C as Driver Go
    participant S as Server
    participant H as Handler
    participant T as Transporte TCP
    participant P as Codec Protobuf
    participant E as Engine
    participant St as Storage

    C->>S: Conexão TCP
    S->>H: Delega conexão
    C->>T: Frame com Request SET
    T->>H: Payload
    H->>P: DecodeRequest
    P-->>H: Request interno
    H->>E: Execute(SET)
    E->>St: Set(key, value)
    St-->>E: Concluído
    E-->>H: Response SET
    H->>P: EncodeResponse
    P-->>H: Payload Protobuf
    H->>T: WriteFrame
    T-->>C: Frame de resposta

    C->>T: Frame com Request GET
    T->>H: Payload
    H->>P: DecodeRequest
    P-->>H: Request interno
    H->>E: Execute(GET)
    E->>St: Get(key)
    St-->>E: value, found
    E-->>H: Response GET
    H->>P: EncodeResponse
    P-->>H: Payload Protobuf
    H->>T: WriteFrame
    T-->>C: Frame de resposta
```

### PING

```mermaid
sequenceDiagram
    participant C as Driver Go
    participant H as Handler
    participant P as Codec
    participant E as Engine

    C->>H: Request PING enquadrada
    H->>P: DecodeRequest
    P-->>H: Request interno
    H->>E: Execute(PING)
    E-->>H: Response com PongResult
    H->>P: EncodeResponse
    P-->>H: Payload
    H-->>C: Frame com PONG
```

### Erro de argumento

```mermaid
sequenceDiagram
    participant C as Driver Go
    participant H as Handler
    participant P as Codec
    participant E as Engine

    C->>H: Request SET com chave vazia
    H->>P: DecodeRequest
    P-->>H: Request interno
    H->>E: Execute(SET)
    E-->>H: Response success=false
    Note over E,H: Código INVALID_ARGUMENT
    H->>P: EncodeResponse
    P-->>H: Payload
    H-->>C: Frame com resposta de erro
```

## Contrato de comunicação

O MemoryDB não expõe uma API HTTP na implementação atual. Os comandos são transmitidos por TCP utilizando frames com prefixo de tamanho e payload Protobuf.

O contrato externo está definido em `protocol/proto/memorydb/v1/memorydb.proto`.

| Comando | Campos da requisição | Resposta de sucesso | Erros de aplicação |
|---|---|---|---|
| PING | `request_id`; comando `ping` | `success=true`, resultado `pong` | `INVALID_REQUEST` |
| SET | `request_id`; `key`; `value`; `ttl_ms` | `success=true`, resultado `set` | `INVALID_ARGUMENT`, `UNSUPPORTED_FEATURE`, `INVALID_REQUEST` |
| GET | `request_id`; `key` | `success=true`, resultado `get`, com `found` e `value` | `INVALID_ARGUMENT`, `INVALID_REQUEST` |
| DELETE | `request_id`; `key` | `success=true`, resultado `delete`, com `deleted` | `INVALID_ARGUMENT`, `INVALID_REQUEST` |

O envelope de resposta contém `request_id`, `success` e um resultado selecionado por `oneof`.

Quando há erro de aplicação, o resultado contém `ErrorResult`, com código e mensagem.

Erros de framing ou de decodificação Protobuf encerram o processamento da conexão. Não há garantia de resposta estruturada de aplicação nesses casos.

## Modelo de dados

```mermaid
classDiagram
    class Storage {
        map[string]Entry entries
        RWMutex mu
        Set(key, value)
        Get(key)
        Delete(key)
    }

    class Entry {
        []byte value
    }

    class Request {
        uint64 RequestID
        Command Command
    }

    class Response {
        uint64 RequestID
        bool Success
        Result Result
    }

    Storage "1" o-- "*" Entry : chave -> valor
    Request --> Command
    Response --> Result
```

O armazenamento é um mapa em memória. As chaves são strings e os valores são bytes.

Não há persistência, identificação de tenant, timestamp ou expiração de entradas na implementação atual.

`Request` e `Response` são modelos de mensagens internas, não entidades de domínio com identidade ou ciclo de vida.

## Dependências e configurações

| Dependência ou configuração | Utilização |
|---|---|
| Go 1.27.0 | Versão declarada nos módulos |
| `google.golang.org/protobuf` v1.36.12 | Serialização e desserialização Protobuf |
| Biblioteca padrão `net` | Comunicação TCP |
| Biblioteca padrão `encoding/binary` | Codificação do tamanho dos frames |
| Biblioteca padrão `io` | Leitura e escrita completas |
| Biblioteca padrão `sync` | Sincronização de acesso |
| Biblioteca padrão `context` | Controle do shutdown |
| `MEMORYDB_ADDR` | Endereço do listener do servidor |
| `:6380` | Endereço padrão do servidor |
| `go.work` | Coordenação dos módulos durante o desenvolvimento |

## Decisões arquiteturais atuais

1. **Separação entre transporte e serialização:** o módulo TCP não depende de Protobuf. O framing pode ser reutilizado por outros protocolos.
2. **Modelo interno independente:** o engine e os componentes internos não utilizam diretamente os tipos Protobuf gerados.
3. **Driver independente do servidor:** aplicações Go consomem a biblioteca cliente sem importar pacotes internos do servidor.
4. **Módulos independentes:** cada componente possui seu próprio `go.mod`; o workspace facilita o desenvolvimento conjunto.
5. **Execução sequencial por conexão:** o driver serializa as operações e o handler processa uma solicitação por vez em cada conexão.
6. **Armazenamento compartilhado:** todas as conexões atendidas pela mesma instância do servidor utilizam o mesmo storage.

## Lacunas arquiteturais identificadas

1. **Port de armazenamento ausente:** o engine depende de `*storage.Storage`, não de uma interface.
2. **Domínio não formalizado:** o modelo interno representa comandos, resultados e mensagens, mas não há uma camada explícita de entidades ou value objects.
3. **Observabilidade incompleta:** o erro retornado por `Handler.Handle` é descartado pela goroutine do servidor; não há registro estruturado de falhas de conexão nesse ponto.
4. **Shutdown sem encerramento forçado:** o servidor fecha o listener e aguarda as conexões, mas não mantém uma estratégia de fechamento forçado das conexões ativas.
5. **Limites operacionais incompletos:** não há timeout configurável de leitura/escrita ou limite de conexões concorrentes.
6. **Segurança de transporte ausente:** TLS, autenticação e autorização não estão implementados.
7. **Persistência ausente:** os dados são perdidos quando o processo termina.
8. **TTL não implementado:** o campo existe no contrato, mas valores positivos são rejeitados.
9. **Multiplexação ausente:** o protocolo possui `request_id`, mas o driver atual trabalha com uma solicitação por vez em cada conexão.

## Testes e validação

O projeto possui testes para:
- Armazenamento e concorrência.
- Execução de comandos no engine.
- Conversão Protobuf.
- Framing TCP e tratamento de fragmentação.
- Comunicação e ciclo de vida das conexões.
- Driver Go e tratamento de erros.
- Integração entre cliente e servidor.

Na validação consolidada mais recente, foram executados `go test -race -count=1 ./...` e `go vet ./...` separadamente nos módulos:

- `protocol/generated/go`
- `transport/tcp/go`
- `drivers/go`
- `server`
- `api`

Todos os testes e verificações de `go vet` passaram nessa execução.