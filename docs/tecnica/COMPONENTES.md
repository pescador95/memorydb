# Componentes do MemoryDB

## 1. Objetivo

Este documento descreve os componentes implementados no MemoryDB, suas responsabilidades, interfaces relevantes, dependências e comportamentos observáveis.

A arquitetura geral, a direção das dependências e os fluxos entre módulos estão documentados em `docs/tecnica/ARQUITETURA.md`.

As descrições deste documento refletem o código existente. Funcionalidades planejadas ou não implementadas são identificadas como limitações.

## 2. Mapa dos componentes

| Componente | Localização | Responsabilidade |
|---|---|---|
| Executável do servidor | `server/cmd/memorydb` | Inicializar e encerrar o servidor |
| Server TCP | `server/internal/transport/tcp/server.go` | Aceitar conexões e controlar seu ciclo de vida |
| Handler TCP | `server/internal/transport/tcp/handler.go` | Processar solicitações de uma conexão |
| Codec | `server/internal/codec/codec.go` | Definir operações de serialização |
| Codec Protobuf | `server/internal/codec/protobuf/codec.go` | Converter mensagens internas e Protobuf |
| Modelo interno | `server/internal/protocol/message.go` | Representar comandos, solicitações e respostas |
| Engine | `server/internal/engine` | Validar e executar comandos |
| Storage | `server/internal/storage` | Armazenar dados em memória |
| Transporte TCP reutilizável | `transport/tcp/go` | Implementar framing e conexão |
| Driver Go | `drivers/go` | Expor uma API cliente para aplicações Go |
| Tipos Protobuf gerados | `protocol/generated/go` | Disponibilizar tipos gerados |
| Schema Protobuf | `protocol/proto/memorydb/v1/memorydb.proto` | Definir o contrato externo |
| Cliente de demonstração | `api/client` | Demonstrar o consumo do driver |

## 3. Executável do servidor

**Localização:** `server/cmd/memorydb/main.go`

**Responsabilidade:** compor as dependências e iniciar o servidor MemoryDB.

O ponto de entrada chama `run()`. A função `run`:

1. lê a variável de ambiente `MEMORYDB_ADDR`;
2. utiliza `:6380` quando a variável não está definida;
3. cria um listener TCP;
4. instancia o codec Protobuf;
5. instancia o engine;
6. cria o handler;
7. cria o servidor TCP;
8. inicia `Server.Serve()` em uma goroutine;
9. aguarda encerramento do servidor ou sinal do sistema operacional;
10. solicita shutdown com timeout de 10 segundos.

Sinais tratados:

- `SIGINT`;
- `SIGTERM`.

O timeout de shutdown é definido pela constante `shutdownTimeout`.

O listener é fechado ao final de `run()` por meio de `defer`.

**Configuração:**

| Variável | Padrão | Descrição |
|---|---|---|
| `MEMORYDB_ADDR` | `:6380` | Endereço TCP no qual o servidor escuta |

**Limitações:**

- Não há arquivo de configuração ou sistema de configuração externo identificado.
- Não há configuração de TLS.
- Não há autenticação ou autorização.
- O timeout de shutdown é uma constante no executável, não uma configuração externa.

## 4. Server TCP

**Localização:** `server/internal/transport/tcp/server.go`

**Tipo principal:** `Server`

**Responsabilidade:** aceitar conexões TCP e delegar seu processamento ao handler.

### 4.1 Construção

`NewServer(listener, handler)` recebe:

- um `net.Listener`;
- uma instância de `Handler`.

A função rejeita listener ou handler nulos.

### 4.2 Método `Serve`

`Serve()`:

- impede que o mesmo servidor seja iniciado simultaneamente mais de uma vez;
- não inicia a aceitação quando o servidor já está em encerramento;
- aceita conexões pelo listener;
- cria uma goroutine para cada conexão aceita;
- contabiliza as conexões ativas com `sync.WaitGroup`;
- delega o processamento ao `Handler.Handle`.

Se a aceitação falhar durante o encerramento, o método retorna normalmente. Se falhar fora do encerramento, retorna um erro contextualizado.

O erro retornado por `Handler.Handle` é atualmente descartado pela goroutine:

```go
go func() {
    defer s.connections.Done()
    _ = s.handler.Handle(conn)
}()
```

Consequentemente, falhas individuais de processamento não são propagadas pelo `Serve()` nem registradas nesse ponto.

### 4.3 Método `Shutdown`

`Shutdown(ctx)`:

1. marca o servidor como encerrando;
2. fecha o listener;
3. aguarda as conexões ativas terminarem;
4. retorna quando todas terminam ou quando o contexto é cancelado.

O método trata `net.ErrClosed` como fechamento já realizado.

**Limitação importante:** o shutdown não fecha diretamente as conexões ativas. Se uma conexão permanecer bloqueada, o método poderá retornar `ctx.Err()` ao expirar o prazo, sem interromper automaticamente aquela conexão.

O servidor também não implementa uma segunda etapa de encerramento forçado.

### 4.4 Concorrência

O estado de execução e encerramento é protegido por mutex. O controle de conexões ativas utiliza `sync.WaitGroup`.

A aceitação de novas conexões e o início do encerramento são coordenados pelo mutex do servidor.

## 5. Handler TCP

**Localização:** `server/internal/transport/tcp/handler.go`

**Tipo principal:** `Handler`

**Responsabilidade:** integrar o transporte TCP, o codec e o engine.

### 5.1 Construção

`NewHandler(commandEngine, messageCodec)` recebe:

- `*engine.Engine`;
- `codec.Codec`.

A função rejeita dependências nulas.

### 5.2 Método `Handle`

Assinatura:

```go
Handle(conn net.Conn) error
```

O método processa solicitações sequencialmente até que a conexão seja encerrada ou ocorra um erro.

Fluxo:

1. valida a conexão;
2. cria `transport/tcp/go.Connection`;
3. lê um frame;
4. decodifica o payload com `DecodeRequest`;
5. executa o comando por meio de `Engine.Execute`;
6. codifica a resposta com `EncodeResponse`;
7. escreve o frame de resposta;
8. repete o fluxo.

O fechamento da conexão é registrado com `defer`.

### 5.3 Tratamento de erros

O handler retorna erros contextualizados quando ocorre:

- falha na criação da conexão;
- falha na leitura do frame;
- falha na decodificação da solicitação;
- falha na codificação da resposta;
- falha na escrita do frame.

EOF durante a leitura é tratado como encerramento normal da conexão.

Uma solicitação que não pode ser decodificada não gera uma resposta estruturada de aplicação. O processamento da conexão é encerrado.

Erros de validação de comandos, por outro lado, são respostas normais do engine e são enviados ao cliente.

## 6. Codec

**Localização:** `server/internal/codec/codec.go`

**Interface:** `codec.Codec`

**Responsabilidade:** definir o contrato de codificação e decodificação entre o modelo interno e uma representação em bytes.

Métodos:

```go
type Codec interface {
    EncodeRequest(request *protocol.Request) ([]byte, error)
    DecodeRequest(data []byte) (*protocol.Request, error)

    EncodeResponse(response *protocol.Response) ([]byte, error)
    DecodeResponse(data []byte) (*protocol.Response, error)
}
```

A interface não determina o formato de serialização. A implementação concreta pode utilizar outro formato, desde que satisfaça o contrato.

A implementação atualmente utilizada pelo servidor é `codec/protobuf.Codec`.

## 7. Codec Protobuf

**Localização:** `server/internal/codec/protobuf/codec.go`

**Tipo principal:** `Codec`

**Responsabilidade:** converter as estruturas internas do servidor para mensagens Protobuf e realizar a conversão inversa.

A implementação utiliza:

- `protocol/generated/go/memorydb/v1`;
- `server/internal/protocol`;
- `google.golang.org/protobuf/proto`.

### 7.1 Construção

`New()` cria uma instância do codec Protobuf.

A implementação declara explicitamente que satisfaz `codec.Codec`.

### 7.2 Codificação de solicitações

`EncodeRequest` converte:

| Modelo interno | Mensagem Protobuf |
|---|---|
| `PingCommand` | `Request_Ping` |
| `SetCommand` | `Request_Set` |
| `GetCommand` | `Request_Get` |
| `DeleteCommand` | `Request_Delete` |

O identificador `RequestID` é transportado para `request_id`.

### 7.3 Decodificação de solicitações

`DecodeRequest`:

1. desserializa os bytes Protobuf;
2. recupera o `request_id`;
3. identifica o comando selecionado no `oneof`;
4. cria o comando correspondente no modelo interno.

Comandos ausentes ou não reconhecidos retornam erro de decodificação.

### 7.4 Codificação de respostas

`EncodeResponse` converte os resultados internos:

| Resultado interno | Resultado Protobuf |
|---|---|
| `PongResult` | `Response_Pong` |
| `SetResult` | `Response_Set` |
| `GetResult` | `Response_Get` |
| `DeleteResult` | `Response_Delete` |
| `ErrorResult` | `Response_Error` |

O identificador e o estado de sucesso também são transportados.

### 7.5 Decodificação de respostas

`DecodeResponse` desserializa a mensagem Protobuf, recupera os campos do envelope e converte o resultado selecionado no `oneof` para o modelo interno.

### 7.6 Limites e cuidados

O codec atual é específico para o schema Protobuf do MemoryDB.

A implementação valida solicitações e respostas nulas nos métodos de codificação e rejeita tipos de comando ou resultado não suportados.

**Ponto de atenção:** os casos de tipos internos representados por ponteiros tipados nulos devem ser tratados com cuidado. Nos `switch` de codificação, um tipo concreto reconhecido não garante que o ponteiro recebido seja diferente de `nil`. A validação desses casos deve ser mantida em mente ao evoluir o codec.

## 8. Modelo interno

**Localização:** `server/internal/protocol/message.go`

**Responsabilidade:** representar comandos, solicitações, resultados e respostas sem depender do transporte ou dos tipos Protobuf gerados.

### 8.1 Comandos

A interface interna `Command` é implementada por:

| Tipo | Campos | Descrição |
|---|---|---|
| `PingCommand` | Nenhum | Solicitação de disponibilidade |
| `SetCommand` | `Key`, `Value`, `TTLMillis` | Gravação de valor |
| `GetCommand` | `Key` | Consulta de valor |
| `DeleteCommand` | `Key` | Remoção de chave |

A interface utiliza um método não exportado para restringir os tipos que podem implementá-la dentro do pacote.

### 8.2 Solicitação

`Request` contém:

| Campo | Tipo | Descrição |
|---|---|---|
| `RequestID` | `uint64` | Identificador para correlação |
| `Command` | `Command` | Comando solicitado |

### 8.3 Resultados

A interface `Result` é implementada por:

| Tipo | Campos | Descrição |
|---|---|---|
| `PongResult` | Nenhum | Resultado de PING |
| `SetResult` | Nenhum | Resultado de SET |
| `GetResult` | `Found`, `Value` | Resultado de GET |
| `DeleteResult` | `Deleted` | Resultado de DELETE |
| `ErrorResult` | `Code`, `Message` | Erro de aplicação |

### 8.4 Resposta

`Response` contém:

| Campo | Tipo | Descrição |
|---|---|---|
| `RequestID` | `uint64` | Identificador correlacionado à solicitação |
| `Success` | `bool` | Indica sucesso ou falha de aplicação |
| `Result` | `Result` | Resultado correspondente |

O modelo interno preserva a separação entre o contrato externo e a lógica de execução.

## 9. Engine

**Localização:** `server/internal/engine`

**Tipo principal:** `Engine`

**Responsabilidade:** validar e executar comandos utilizando o storage.

### 9.1 Construção

`engine.New()` cria uma instância de `Engine` e instancia internamente um `storage.Storage`.

O engine não recebe uma interface de storage nem uma instância externa pelo construtor atual.

### 9.2 Método `Execute`

Assinatura:

```go
Execute(request *protocol.Request) *protocol.Response
```

O método identifica o tipo de comando, valida os parâmetros e retorna uma resposta interna.

### 9.3 Regras implementadas

| Situação | Resultado |
|---|---|
| Solicitação nula | `INVALID_REQUEST` |
| Comando ausente ou desconhecido | `INVALID_REQUEST` |
| Comando tipado nulo | `INVALID_REQUEST` |
| Chave vazia em SET | `INVALID_ARGUMENT` |
| Chave vazia em GET | `INVALID_ARGUMENT` |
| Chave vazia em DELETE | `INVALID_ARGUMENT` |
| SET com TTL positivo | `UNSUPPORTED_FEATURE` |
| SET com TTL zero | Gravação sem expiração |
| GET de chave inexistente | Sucesso com `Found=false` |
| GET de chave existente | Sucesso com `Found=true` |
| DELETE de chave inexistente | Sucesso com `Deleted=false` |
| DELETE de chave existente | Sucesso com `Deleted=true` |

O `RequestID` recebido é preservado na resposta.

Os códigos de erro estão definidos em `server/internal/engine/command.go`:

- `INVALID_REQUEST`;
- `INVALID_ARGUMENT`;
- `UNSUPPORTED_FEATURE`.

### 9.4 Limitações

- O engine depende concretamente de `*storage.Storage`.
- Não há interface de repositório ou storage.
- Não há transações.
- Não há TTL funcional.
- Não há deduplicação de operações.
- Não há identificação estável de operações entre tentativas.

## 10. Storage

**Localização:** `server/internal/storage`

**Tipos principais:** `Storage` e `Entry`

**Responsabilidade:** manter os pares chave-valor em memória.

### 10.1 Estrutura

O storage utiliza:

- um mapa `map[string]Entry`;
- `sync.RWMutex` para sincronização;
- valores armazenados como `[]byte`.

A estrutura `Entry` mantém o valor em um campo não exportado.

### 10.2 API

| Método | Comportamento |
|---|---|
| `New()` | Cria um storage vazio |
| `Set(key, value)` | Armazena uma cópia do valor |
| `Get(key)` | Retorna uma cópia do valor e indica se a chave existe |
| `Delete(key)` | Remove a chave e informa se ela existia |

Assinaturas relevantes:

```go
Set(key string, value []byte)

Get(key string) ([]byte, bool)

Delete(key string) bool
```

### 10.3 Isolamento dos dados

O storage copia os bytes recebidos em `Set` e os bytes retornados em `Get`.

Essa estratégia evita que alterações posteriores nos slices do chamador modifiquem diretamente os dados armazenados ou que o chamador altere o conteúdo interno por meio do valor retornado.

### 10.4 Concorrência

`Set` e `Delete` utilizam lock exclusivo.

`Get` utiliza read lock.

O controle permite operações concorrentes sem acesso desprotegido ao mapa.

### 10.5 Limitações

- Dados exclusivamente em memória.
- Sem persistência.
- Sem expiração.
- Sem replicação.
- Sem limite de memória configurável identificado.
- Sem política de eviction identificada.

## 11. Transporte TCP reutilizável

**Módulo:** `transport/tcp/go`

**Pacotes principais:** framing e conexão.

**Responsabilidade:** fornecer framing e abstração de conexão sem dependência do Protobuf ou do servidor.

### 11.1 Framing

**Arquivo:** `transport/tcp/go/framing.go`

Constantes:

| Constante | Valor |
|---|---|
| `HeaderSize` | 4 bytes |
| `MaxFrameSize` | 1 MiB |

Erros exportados:

- `ErrFrameTooLarge`;
- `ErrInvalidFrameSize`.

`ReadFrame`:

1. valida a presença do reader;
2. lê integralmente o cabeçalho;
3. interpreta o tamanho em Big Endian;
4. rejeita tamanho zero;
5. rejeita payload maior que o limite;
6. lê integralmente o payload.

`WriteFrame`:

1. valida a presença do writer;
2. rejeita payload vazio;
3. rejeita payload acima do limite;
4. escreve o cabeçalho Big Endian;
5. escreve o payload integralmente.

A função interna `writeFull` trata escritas parciais e rejeita contagens inválidas ou escritas sem progresso.

### 11.2 Connection

**Arquivo:** `transport/tcp/go/connection.go`

**Tipo:** `Connection`

A abstração encapsula `net.Conn`.

Métodos:

| Método | Comportamento |
|---|---|
| `NewConnection(conn)` | Cria uma abstração de conexão |
| `ReadFrame()` | Lê um frame completo |
| `WriteFrame(payload)` | Escreve um frame completo |
| `Close()` | Fecha a conexão subjacente |

`NewConnection` rejeita conexões nulas e interfaces que contenham valores tipados nulos dos tipos tratados pela implementação.

A conexão utiliza mutexes separados para leitura e escrita. Assim, pode haver uma leitura e uma escrita simultâneas, mas operações concorrentes do mesmo tipo são serializadas.

`Close` não depende dos mutexes de leitura e escrita. O fechamento da conexão subjacente pode interromper operações de rede bloqueadas.

## 12. Driver Go

**Módulo:** `drivers/go`

**Pacote:** `memorydb`

**Arquivo principal:** `drivers/go/client.go`

**Responsabilidade:** disponibilizar uma API Go para aplicações clientes acessarem o MemoryDB.

### 12.1 Tipo `Client`

O cliente encapsula:

- conexão TCP;
- mecanismo de serialização das operações;
- estado interno;
- controle de fechamento;
- contador de `request_id`.

O valor zero de `Client` não é utilizável. A criação deve ocorrer por `Dial`, `DialContext` ou `New`.

### 12.2 Construção e fechamento

| Método | Comportamento |
|---|---|
| `Dial(address)` | Estabelece conexão TCP |
| `DialContext(ctx, address)` | Estabelece conexão TCP utilizando contexto |
| `New(conn)` | Cria cliente a partir de uma conexão existente |
| `Close()` | Encerra a conexão e impede novas operações |

Quando criado por `New`, o cliente assume a propriedade da conexão e a encerra ao executar `Close`.

`DialContext` utiliza o contexto para controlar somente o estabelecimento da conexão. Ele não fica associado às operações posteriores.

### 12.3 Operações

| Método | Comportamento |
|---|---|
| `Ping()` | Verifica a disponibilidade do servidor |
| `PingContext(ctx)` | Executa PING com contexto |
| `Set(key, value)` | Grava um valor |
| `SetContext(ctx, key, value)` | Grava um valor com contexto |
| `Get(key)` | Recupera valor e existência |
| `GetContext(ctx, key)` | Recupera valor e existência com contexto |
| `Delete(key)` | Remove uma chave |
| `DeleteContext(ctx, key)` | Remove uma chave com contexto |

O `Set` do driver atual não expõe parâmetro de TTL. O servidor recebe o campo `ttl_ms` no contrato Protobuf, mas rejeita valores positivos.

### 12.4 Serialização das operações

O cliente permite que seus métodos sejam chamados concorrentemente.

Entretanto, `operationGate` permite somente uma operação por vez na conexão.

A serialização é necessária porque o protocolo atual não implementa multiplexação. O cliente envia uma solicitação e aguarda a resposta correspondente antes de liberar a conexão para a próxima operação.

### 12.5 Estados internos

O cliente possui três estados:

| Estado | Significado |
|---|---|
| `open` | Conexão disponível |
| `closed` | Cliente encerrado explicitamente |
| `failed` | Conexão invalidada após falha de comunicação ou protocolo |

A transição para `failed` fecha a conexão e impede novas operações.

`Close` pode ser chamado concorrentemente com operações. Como o fechamento interrompe o I/O, uma operação que já começou pode terminar com resultado indeterminado.

### 12.6 Contextos e cancelamento

O cliente diferencia o cancelamento enquanto aguarda a conexão do cancelamento durante a comunicação.

| Momento | Resultado | Estado esperado |
|---|---|---|
| Antes de adquirir `operationGate` | Erro de contexto; a operação não começou | `open` |
| Durante a espera por `operationGate` | Erro de contexto; a operação não começou | `open` |
| Durante comunicação de rede | Resultado potencialmente indeterminado | `failed` |

O cancelamento durante I/O fecha a conexão para interromper a operação bloqueada.

Como a solicitação pode já ter sido processada pelo servidor, não é possível concluir apenas pelo cancelamento se a operação foi ou não aplicada.

A implementação sincroniza a conclusão e a interrupção por meio de fases internas da operação. O resultado observado depende de qual transição é efetivada primeiro.

### 12.7 `ServerError`

**Tipo:** `ServerError`

Representa um erro estruturado recebido em uma resposta válida do servidor.

Campos:

| Campo | Descrição |
|---|---|
| `Code` | Código do erro |
| `Message` | Mensagem retornada |

O erro indica uma rejeição de aplicação reconhecida. Ele não invalida automaticamente a conexão.

### 12.8 `OperationOutcomeUnknownError`

**Tipo:** `OperationOutcomeUnknownError`

Representa uma operação que pode ter sido executada, mas cujo resultado não pôde ser confirmado pelo cliente.

Campos:

| Campo | Descrição |
|---|---|
| `RequestID` | Identificador da solicitação afetada |
| `Cause` | Causa subjacente da falha ou interrupção |

Métodos:

- `Error() string`;
- `Unwrap() error`.

A implementação de `Unwrap` permite inspecionar a causa por meio de `errors.Is` e `errors.As`.

Possíveis situações:

- escrita parcial da solicitação;
- falha de escrita depois que parte dos dados foi transmitida;
- cancelamento ou deadline durante I/O;
- conexão interrompida depois do envio;
- resposta perdida ou incompleta;
- resposta Protobuf inválida;
- divergência entre o `request_id` enviado e o recebido;
- resposta de tipo incompatível com o comando solicitado;
- ausência de detalhes de erro em uma resposta de falha.

O erro não afirma que a operação falhou. Ele informa que o cliente não possui confirmação suficiente para determinar se o servidor executou a operação.

### 12.9 Comportamento após resultado indeterminado

Quando o resultado é indeterminado, o cliente invalida a conexão.

Não há retry automático, reconexão transparente ou recuperação de resposta anterior.

A aplicação cliente não deve repetir cegamente a operação. O comando pode ter sido executado antes da perda da resposta.

### 12.10 Limitações do driver

- Não há multiplexação.
- Não há retry automático.
- Não há reconexão automática.
- Não há idempotency key.
- Não há deduplicação no servidor.
- `request_id` é local à conexão e utilizado para correlação.
- Não há recuperação de resultado após reconexão.

## 13. Schema Protobuf e tipos gerados

**Schema:** `protocol/proto/memorydb/v1/memorydb.proto`

**Módulo gerado:** `protocol/generated/go`

O schema define os envelopes de solicitação e resposta, os comandos, os resultados e os erros.

O campo `request_id` é definido como `uint64`.

Os comandos e resultados são representados por `oneof`, garantindo uma seleção entre as alternativas declaradas no contrato.

O campo `ttl_ms` existe em `SetCommand`, mas sua funcionalidade não está implementada no engine.

Os arquivos gerados devem ser atualizados a partir do schema por meio do processo de geração adotado pelo projeto.

**[LACUNA]** O processo oficial de geração, incluindo versão e instalação das ferramentas, deve ser documentado em um procedimento reproduzível se ainda não estiver definido em outro documento do repositório.

## 14. API de demonstração

**Localização:** `api/client`

**Arquivo principal:** `api/client/main.go`

O executável demonstra o uso do driver Go.

Ele utiliza a API pública do driver para se comunicar com o servidor. Não acessa diretamente os pacotes internos do servidor nem implementa um protocolo alternativo.

É uma aplicação de demonstração, não uma segunda implementação do servidor.

## 15. Testes por componente

Os testes estão distribuídos junto aos módulos e pacotes.

| Componente | Arquivo de teste | Cobertura identificada |
|---|---|---|
| Framing TCP | `transport/tcp/go/framing_test.go` | Big Endian, fragmentação, limites, truncamento, interfaces nulas e escritas parciais |
| Connection TCP | `transport/tcp/go/connection_test.go` | Fechamento, I/O bloqueado, concorrência e full-duplex |
| Driver Go | `drivers/go/client_test.go` | Operações, concorrência, contextos, erros, desconexões e resultados indeterminados |
| Codec Protobuf | `server/internal/codec/protobuf/codec_test.go` | Round-trip, conversões e mensagens inválidas |
| Engine | `server/internal/engine/engine_test.go` | Comandos, validações e erros |
| Storage | `server/internal/storage/storage_test.go` | Operações, cópia de bytes e concorrência |
| Handler | `server/internal/transport/tcp/handler_test.go` | Processamento sequencial de solicitações |
| Server TCP | `server/internal/transport/tcp/server_test.go` | Aceitação de clientes, concorrência e shutdown |
| Integração | `server/internal/integration/server_test.go` | Comunicação entre driver e servidor |

### 15.1 Testes do driver para resultados indeterminados

A cobertura recente do driver inclui cenários específicos de:

- escrita parcial;
- execução de SET no servidor seguida de perda da resposta;
- cancelamento durante a leitura da resposta;
- preservação da causa original;
- transição para o estado `failed`;
- cancelamento antes do início da operação;
- concorrência entre conclusão e cancelamento.

### 15.2 Validação recente do driver

Há registro de execução bem-sucedida dos seguintes comandos no módulo `drivers/go`:

```bash
go test -v -count=1 ./drivers/go
go test -race -count=1 ./drivers/go
go test -race -count=20 ./drivers/go
go vet ./drivers/go
```

Esses resultados dizem respeito ao módulo do driver. Não representam uma nova validação consolidada de todos os módulos do workspace.

Também há registro de validações anteriores das demais áreas e de execução manual ponta a ponta em 15/11/2026.

## 16. Pontos de atenção e limitações transversais

| Ponto | Situação observada | Consequência |
|---|---|---|
| Erros do handler | Retorno de `Handler.Handle` descartado pelo servidor | Falhas individuais não são registradas nesse ponto |
| Shutdown | Conexões ativas não são fechadas diretamente | O prazo pode expirar sem encerrar todas as conexões |
| Storage | Engine depende de implementação concreta | Substituição do backend exige alteração do engine |
| TTL | Campo existe no schema, mas TTL positivo é rejeitado | Não há expiração de entradas |
| Idempotência | Não há identificador estável nem deduplicação | Repetição após resultado indeterminado pode executar novamente |
| Persistência | Storage somente em memória | Dados são perdidos no encerramento do processo |
| Multiplexação | Driver serializa operações por conexão | Uma operação por vez em cada conexão |
| Observabilidade | Não há registro estruturado de falhas de conexão no servidor | Diagnóstico operacional limitado |
| Timeouts de I/O | Não há configuração identificada no servidor | Conexões podem permanecer bloqueadas |

## 17. Evoluções que envolvem múltiplos componentes

| Evolução | Componentes envolvidos | Resultado esperado |
|---|---|---|
| Idempotência | Schema, modelo interno, engine, armazenamento de operações e driver | Identificação estável das operações |
| Deduplicação | Handler, engine e armazenamento de resultados | Evitar execução duplicada e recuperar resposta anterior |
| TTL | Schema, modelo interno, engine e storage | Expiração de entradas |
| Persistência | Storage e recuperação | Preservação de dados após reinicialização |
| Shutdown forçado | Server e Connection | Fechamento das conexões após o prazo |
| Observabilidade | Server, Handler e driver | Registro e diagnóstico de falhas |
| Timeouts | Server, Handler e configuração | Controle de operações de rede bloqueadas |
| Multiplexação | Protocolo, handler e driver | Operações simultâneas na mesma conexão |

### 17.1 Considerações para idempotência

A implementação de idempotência exige decisões anteriores à alteração do schema:

- diferença entre `request_id` e `operation_id`;
- escopo e geração do identificador de operação;
- comandos que exigem deduplicação;
- comportamento quando o mesmo identificador é recebido com payload diferente;
- tratamento de operações concorrentes com o mesmo identificador;
- retenção e expiração dos registros;
- replay da resposta original;
- comportamento após reinicialização.

Uma garantia inicial de execução no máximo uma vez por identificador só pode ser afirmada dentro do escopo e do período de retenção definidos pelo contrato.

Para sobreviver a crash ou reinicialização, a alteração do dado e o registro de deduplicação precisarão de persistência com atomicidade.

O projeto não deve anunciar *exactly once* sem uma garantia técnica e contratual que sustente essa afirmação.

## 18. Diretrizes de manutenção dos componentes

### Comentários no código

- Documentar identificadores exportados conforme as convenções GoDoc.
- Explicar contratos, parâmetros, retornos e erros.
- Registrar invariantes e decisões não óbvias.
- Evitar comentários redundantes que apenas repetem o nome do método.
- Atualizar comentários quando o comportamento implementado mudar.

### Documentação Markdown

- Descrever responsabilidades e limites dos componentes.
- Explicar dependências e fluxos entre módulos.
- Registrar garantias e limitações observáveis.
- Documentar cenários de falha e concorrência.
- Manter exemplos alinhados à API pública atual.
- Distinguir funcionalidades implementadas de evoluções planejadas.

A documentação complementa os comentários do código e não deve substituí-los quando o contrato de uma API pública exigir explicação no próprio código.