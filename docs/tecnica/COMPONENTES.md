# Componentes do MemoryDB

## 1. Objetivo

Este documento descreve os componentes implementados no MemoryDB, suas responsabilidades, interfaces relevantes, dependências e comportamentos observáveis.

A arquitetura geral, a direção das dependências e os fluxos entre módulos estão documentados em `docs/tecnica/ARQUITETURA.md`.

As descrições refletem a implementação conhecida do projeto. Funcionalidades planejadas ou não implementadas são identificadas como limitações ou evoluções.

## 2. Mapa dos componentes

| Componente | Localização | Responsabilidade |
|---|---|---|
| Executável do servidor | `server/cmd/memorydb` | Compor as dependências e controlar a inicialização e o encerramento do servidor |
| Configuração do servidor | `server/internal/config` | Carregar, validar e disponibilizar as configurações operacionais do servidor |
| Configuração compartilhada | `shared/config` | Centralizar nomes de variáveis de ambiente e valores padrão compartilhados |
| Server TCP | `server/internal/transport/tcp/server.go` | Aceitar conexões e controlar seu ciclo de vida |
| Handler TCP | `server/internal/transport/tcp/handler.go` | Processar solicitações de uma conexão e coordenar a execução dos comandos |
| Codec | `server/internal/codec/codec.go` | Definir as operações de serialização e desserialização |
| Codec Protobuf | `server/internal/codec/protobuf/codec.go` | Converter mensagens internas e mensagens Protobuf |
| Modelo interno | `server/internal/protocol/message.go` | Representar comandos, solicitações, resultados e respostas |
| Engine | `server/internal/engine` | Validar e executar comandos |
| Executor de idempotência | `server/internal/idempotency` | Controlar identificadores, fingerprints, concorrência, replay, capacidade e retenção |
| Storage | `server/internal/storage` | Armazenar pares chave-valor em memória |
| Transporte TCP reutilizável | `transport/tcp/go` | Implementar framing e abstração de conexão |
| Driver Go | `drivers/go` | Expor uma API cliente para aplicações Go |
| Tipos Protobuf gerados | `protocol/generated/go` | Disponibilizar os tipos Go gerados a partir do schema |
| Schema Protobuf | `protocol/proto/memorydb/v1/memorydb.proto` | Definir o contrato externo de comunicação |
| Cliente de demonstração | `api/client` | Demonstrar o consumo do driver Go |
| Configuração do cliente de demonstração | `api/client/internal/config` | Carregar e validar as configurações utilizadas pelo executável cliente |

O projeto possui múltiplos módulos Go coordenados pelo workspace da raiz. O diretório `protocol/proto` contém o schema de origem e não constitui um módulo Go independente.

## 3. Executável do servidor

**Localização:** `server/cmd/memorydb/main.go`

**Responsabilidade:** compor as dependências e iniciar o servidor MemoryDB.

O ponto de entrada chama `run()`. A função `run`:

1. Carrega e valida as configurações operacionais do servidor.
2. Cria um listener TCP usando o endereço configurado.
3. Instancia o codec Protobuf.
4. Instancia o Engine.
5. Cria uma instância compartilhada do Executor de idempotência com os parâmetros configurados.
6. Cria o Handler, injetando Engine, Codec e Executor.
7. Cria o servidor TCP.
8. Inicia `Server.Serve()` em uma goroutine.
9. Aguarda o encerramento do servidor ou um sinal do sistema operacional.
10. Solicita o encerramento por meio de `Shutdown`, utilizando o timeout configurado.

Sinais tratados:

- `SIGINT`;
- `SIGTERM`.

O listener é fechado ao final de `run()` por meio de `defer`.

### 3.1 Configuração

As configurações específicas do servidor são carregadas por `server/internal/config`, utilizando também os nomes e valores padrão compartilhados disponibilizados por `shared/config`.

| Variável | Padrão | Descrição |
|---|---:|---|
| `MEMORYDB_ADDR` | `:6380` | Endereço TCP no qual o servidor escuta |
| `MEMORYDB_SHUTDOWN_TIMEOUT` | `10s` | Prazo da tentativa de encerramento gracioso antes do fechamento forçado das conexões restantes |
| `MEMORYDB_READ_TIMEOUT` | `30s` | Timeout de leitura de cada frame recebido pelo Handler |
| `MEMORYDB_WRITE_TIMEOUT` | `10s` | Timeout de escrita de cada resposta enviada pelo Handler |
| `MEMORYDB_MAX_CONNECTIONS` | `1024` | Limite de conexões TCP simultâneas admitidas pelo servidor |
| `MEMORYDB_IDEMPOTENCY_RETENTION` | `24h` | Tempo de retenção dos registros de idempotência após a conclusão |
| `MEMORYDB_IDEMPOTENCY_CAPACITY` | `100000` | Capacidade máxima de registros de idempotência |

Variáveis ausentes ou vazias utilizam os valores padrão. Valores explicitamente informados, mas inválidos, fazem o carregamento da configuração retornar erro e impedem a inicialização normal do servidor.

As configurações de duração utilizam o formato aceito por `time.ParseDuration`. Os valores de duração e capacidade precisam ser positivos.

A configuração `MEMORYDB_ADDR` é utilizada pelo listener do servidor. Ela não altera automaticamente o endereço configurado no cliente de demonstração.

### 3.2 Limitações

- Não há configuração de TLS.
- Não há autenticação ou autorização.
- O limite de conexões controla a quantidade de conexões TCP simultâneas admitidas, mas não representa um limite de requisições ou de operações simultâneas por conexão.
- Conexões excedentes ao limite configurado são fechadas pelo servidor.
- Os timeouts de leitura e escrita são aplicados pelo Handler às operações de I/O TCP.
- O fechamento forçado durante o shutdown interrompe operações bloqueadas na conexão, mas não representa cancelamento genérico de qualquer operação executada pelo Handler.

## 4. Server TCP

**Localização:** `server/internal/transport/tcp/server.go`

**Tipo principal:** `Server`

**Responsabilidade:** aceitar conexões TCP e delegar seu processamento ao Handler.

### 4.1 Construção

A construção padrão utiliza:

`NewServer(listener, handler)`

A construção com configuração explícita utiliza:

`NewServerWithConfig(listener, handler, config)`

A configuração é representada por:

```go
type ServerConfig struct {
    MaxConnections int
}
```

Os construtores recebem:

- `listener`: instância de `net.Listener` utilizada para aceitar conexões TCP;
- `handler`: instância de `Handler` responsável pelo processamento das conexões;
- `config`: configuração explícita do servidor, utilizada por `NewServerWithConfig`.

`NewServer` utiliza o valor padrão de `DefaultServerMaxConnections`, definido em `shared/config`.

`NewServerWithConfig` permite configurar explicitamente `MaxConnections`, que representa o limite de conexões TCP simultâneas admitidas.

Os construtores verificam se o listener e o Handler recebidos são `nil`. Essa verificação não detecta uma implementação de `net.Listener` que contenha um ponteiro tipado nulo.

`NewServerWithConfig` também rejeita valores de `MaxConnections` menores que 1.

### 4.2 Método `Serve`

`Serve()`:

- impede que o mesmo servidor seja iniciado simultaneamente mais de uma vez;
- não inicia a aceitação de conexões quando o servidor já está em encerramento;
- aceita conexões pelo listener;
- controla a quantidade de conexões simultâneas admitidas;
- fecha conexões excedentes ao limite configurado;
- registra as conexões aceitas na estrutura de conexões ativas;
- cria uma goroutine para cada conexão aceita;
- contabiliza as goroutines com `sync.WaitGroup`;
- delega o processamento ao método `Handler.Handle`;
- impede a admissão de novas conexões durante o encerramento.

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

1. Marca o servidor como encerrando.
2. Fecha o listener TCP, impedindo novas admissões.
3. Aguarda as conexões ativas terminarem normalmente.
4. Se o contexto expirar, fecha as conexões ainda ativas.
5. Aguarda a conclusão das goroutines responsáveis pelas conexões.
6. Retorna o erro do contexto, preservando eventual erro no fechamento do listener.

O método trata `net.ErrClosed` como fechamento já realizado.

O encerramento possui duas etapas: tentativa graciosa e fechamento forçado das conexões restantes após a expiração do contexto.

O fechamento forçado interrompe operações de rede bloqueadas pela conexão TCP. Não representa cancelamento genérico de qualquer operação executada pelo Handler.

### 4.4 Concorrência

O servidor utiliza:

- mutex para proteger o estado de execução e encerramento;
- `sync.WaitGroup` para acompanhar as goroutines responsáveis pelas conexões;
- mapa protegido por mutex para manter o conjunto de conexões ativas.

A aceitação de novas conexões e o início do encerramento são coordenados para impedir novas admissões após o início do shutdown.

O limite configurado em `MaxConnections` controla a quantidade de conexões TCP simultâneas admitidas. Conexões excedentes são fechadas, sem processamento pelo Handler.

## 5. Handler TCP

**Localização:** `server/internal/transport/tcp/handler.go`

**Tipo principal:** `Handler`

**Responsabilidade:** integrar o transporte TCP, o codec, o Executor de idempotência e o Engine.

### 5.1 Construção

A construção padrão utiliza:

`NewHandler(commandEngine, messageCodec, executor)`

A construção com configuração explícita utiliza:

`NewHandlerWithConfig(commandEngine, messageCodec, executor, config)`

A configuração é representada por:

```go
type HandlerConfig struct {
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
}
```

Os construtores recebem:

- `commandEngine`: instância de `*engine.Engine`;
- `messageCodec`: implementação da interface `codec.Codec`;
- `executor`: instância de `*idempotency.Executor`;
- `config`: configuração explícita dos timeouts, utilizada por `NewHandlerWithConfig`.

`NewHandler` utiliza os valores padrão definidos em `shared/config`:

- leitura: `DefaultServerReadTimeout` (30 segundos);
- escrita: `DefaultServerWriteTimeout` (10 segundos).

`NewHandlerWithConfig` permite configurar explicitamente os dois timeouts.

Os construtores verificam se o Engine, o Codec e o Executor recebidos são `nil`. Como `messageCodec` é uma interface, a verificação não detecta uma interface que contenha um ponteiro tipado nulo.

Os timeouts de leitura e escrita também precisam ser positivos.

O Handler criado é compartilhado pelas conexões admitidas pelo mesmo Server. Consequentemente, as solicitações dessas conexões utilizam a mesma instância do Executor de idempotência.

### 5.2 Método `Handle`

Assinatura:

```go
Handle(conn net.Conn) error
```

O método processa solicitações sequencialmente até que a conexão seja encerrada ou ocorra um erro.

Fluxo:

1. Valida a conexão.
2. Cria uma instância de `transport/tcp/go.Connection`.
3. Lê um frame.
4. Decodifica o payload com `DecodeRequest`.
5. Identifica o tipo de comando.
6. Encaminha `SET` e `DELETE` ao Executor de idempotência.
7. Encaminha `PING` e `GET` diretamente ao Engine.
8. Codifica a resposta com `EncodeResponse`.
9. Escreve o frame de resposta.
10. Repete o fluxo.

O fechamento da conexão é registrado com `defer`.

O Handler aplica os seguintes controles de I/O:

- antes de cada leitura de frame, configura o read deadline da conexão;
- antes de cada escrita de resposta, configura o write deadline da conexão.

Os prazos são configurados por `HandlerConfig.ReadTimeout` e `HandlerConfig.WriteTimeout`, respectivamente.

Os deadlines limitam operações individuais de leitura e escrita. Não representam um timeout global para toda a duração da conexão ou para a execução completa de um comando.

### 5.3 Execução de operações mutáveis

As operações `SET` e `DELETE` passam pelo mecanismo de idempotência antes de serem executadas no Engine.

O Handler calcula o fingerprint da operação por meio de `idempotency.Fingerprint()` e utiliza o `OperationID` recebido no comando para solicitar sua execução ao Executor.

O Executor recebe:

- o `OperationID`;
- o `RequestID` da tentativa atual;
- o fingerprint da operação;
- uma função que executa o comando no Engine.

A execução do Engine ocorre fora do mutex interno do Executor.

O Executor coordena a reserva do identificador, a execução original, a espera de chamadas concorrentes e o armazenamento do resultado terminal reproduzível.

O Engine não implementa deduplicação e não conhece o mecanismo de idempotência.

### 5.4 Execução de operações não mutáveis

As operações `PING` e `GET` não utilizam o Executor.

Esses comandos são encaminhados diretamente ao Engine, pois não fazem parte do contrato inicial de idempotência.

### 5.5 Tratamento de erros

O Handler retorna erros contextualizados quando ocorre:

- falha na criação da conexão;
- falha na leitura do frame;
- falha na decodificação da solicitação;
- falha na codificação da resposta;
- falha na escrita do frame.

EOF durante a leitura é tratado como encerramento normal da conexão.

Uma solicitação que não pode ser decodificada não gera uma resposta estruturada de aplicação. O processamento da conexão é encerrado.

Erros de validação de comandos são representados como respostas do Engine e enviados ao cliente.

Erros relacionados ao mecanismo de idempotência são convertidos em respostas estruturadas de aplicação, incluindo:

| Código | Significado |
|---|---|
| `INVALID_OPERATION_ID` | Identificador de operação inválido |
| `OPERATION_ID_CONFLICT` | Identificador reutilizado com parâmetros semanticamente diferentes |
| `IDEMPOTENCY_CAPACITY_REACHED` | Capacidade de registros de idempotência esgotada |
| `OPERATION_OUTCOME_UNKNOWN` | Resultado da execução não pode ser determinado |
| `INTERNAL_ERROR` | Erro interno não classificado por um código específico do Executor |

Essas respostas permitem que o cliente reconheça erros informados pelo servidor e os diferencie de falhas de comunicação. Isso não significa que toda resposta de erro represente uma falha de transporte ou invalide a conexão.

## 6. Codec

**Localização:** `server/internal/codec/codec.go`

**Interface:** `codec.Codec`

**Responsabilidade:** definir o contrato de codificação e decodificação entre o modelo interno e uma representação em bytes.

Métodos:

```go
type Codec interface {
    EncodeRequest(*protocol.Request) ([]byte, error)
    DecodeRequest(data []byte) (*protocol.Request, error)

    EncodeResponse(*protocol.Response) ([]byte, error)
    DecodeResponse(data []byte) (*protocol.Response, error)
}
```

A interface não determina o formato de serialização. Uma implementação concreta pode utilizar outro formato, desde que satisfaça o contrato.

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

A implementação declara explicitamente que satisfaz a interface `codec.Codec`.

### 7.2 Codificação de solicitações

`EncodeRequest` converte:

| Modelo interno | Mensagem Protobuf |
|---|---|
| `PingCommand` | `Request_Ping` |
| `SetCommand` | `Request_Set` |
| `GetCommand` | `Request_Get` |
| `DeleteCommand` | `Request_Delete` |

O identificador `RequestID` é transportado para `request_id`.

Nos comandos `SET` e `DELETE`, o `OperationID` é transportado para o campo `operation_id`.

O identificador é representado por `bytes` no contrato Protobuf e deve conter exatamente 16 bytes para ser aceito pelo mecanismo de idempotência do servidor.

### 7.3 Decodificação de solicitações

`DecodeRequest`:

1. Desserializa os bytes Protobuf.
2. Recupera o `request_id`.
3. Identifica o comando selecionado no `oneof`.
4. Cria o comando correspondente no modelo interno.
5. Copia os campos do comando, incluindo `operation_id` para `SET` e `DELETE`.

Comandos ausentes ou não reconhecidos retornam erro de decodificação.

A validação do tamanho e da obrigatoriedade do `OperationID` é realizada no fluxo de idempotência do Handler/Executor, não pelo codec.

### 7.4 Codificação de respostas

`EncodeResponse` converte os resultados internos:

| Resultado interno | Resultado Protobuf |
|---|---|
| `PongResult` | `Response_Pong` |
| `SetResult` | `Response_Set` |
| `GetResult` | `Response_Get` |
| `DeleteResult` | `Response_Delete` |
| `ErrorResult` | `Response_Error` |

O identificador da solicitação, o estado de sucesso e o resultado correspondente também são transportados.

### 7.5 Decodificação de respostas

`DecodeResponse` desserializa a mensagem Protobuf, recupera os campos do envelope e converte o resultado selecionado no `oneof` para o modelo interno.

### 7.6 Limites e cuidados

O codec atual é específico para o schema Protobuf do MemoryDB.

A implementação valida solicitações e respostas nulas nos métodos de codificação e rejeita tipos de comando ou resultado não suportados.

**Ponto de atenção:** tipos internos representados por ponteiros tipados nulos exigem tratamento explícito. Um `switch` que reconhece o tipo concreto não garante, por si só, que o ponteiro recebido seja diferente de `nil`. A validação desses casos deve ser preservada ao evoluir o codec.

## 8. Modelo interno

**Localização:** `server/internal/protocol/message.go`

**Responsabilidade:** representar comandos, solicitações, resultados e respostas sem depender do transporte ou dos tipos Protobuf gerados.

### 8.1 Comandos

A interface interna `Command` é implementada por:

| Tipo | Campos | Descrição |
|---|---|---|
| `PingCommand` | Nenhum | Solicitação de disponibilidade |
| `SetCommand` | `OperationID`, `Key`, `Value`, `TTLMillis` | Gravação de valor com identificação de operação |
| `GetCommand` | `Key` | Consulta de valor |
| `DeleteCommand` | `OperationID`, `Key` | Remoção de chave com identificação de operação |

O `OperationID` dos comandos `SET` e `DELETE` é representado internamente por `[]byte`.

A interface utiliza um método não exportado para restringir os tipos que podem implementá-la fora do pacote.

### 8.2 Solicitação

`Request` contém:

| Campo | Tipo | Descrição |
|---|---|---|
| `RequestID` | `uint64` | Identificador para correlação |
| `Command` | `Command` | Comando solicitado |

O `RequestID` identifica a tentativa de comunicação. Ele não é utilizado como identificador estável de uma operação lógica.

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

O `OperationID` não é um campo do envelope `Request`. Ele pertence aos comandos mutáveis `SetCommand` e `DeleteCommand`.

## 9. Engine

**Localização:** `server/internal/engine`

**Tipo principal:** `Engine`

**Responsabilidade:** validar e executar comandos utilizando o Storage.

### 9.1 Construção

`engine.New()` cria uma instância de `Engine` e instancia internamente um `storage.Storage`.

O Engine não recebe uma interface de Storage nem uma instância externa pelo construtor atual.

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

### 9.4 Relação com a idempotência

O Engine não implementa deduplicação e não mantém registros de `OperationID`.

A coordenação de idempotência pertence ao Handler e ao pacote `server/internal/idempotency`.

Para `SET` e `DELETE`, o Handler solicita a execução ao Executor, que invoca o Engine somente quando a operação precisa ser executada.

Para `PING` e `GET`, o Handler chama o Engine diretamente.

Essa separação mantém o Engine responsável pela validação e execução dos comandos, sem incorporar o ciclo de vida dos registros de idempotência.

### 9.5 Limitações

- O Engine depende concretamente de `*storage.Storage`.
- Não há interface de repositório ou Storage.
- Não há transações.
- Não há TTL funcional.
- O Engine não valida `OperationID`.
- O Engine não implementa deduplicação.
- A execução de comandos mutáveis depende da coordenação externa realizada pelo Handler e pelo Executor.

## 10. Executor de idempotência

**Localização:** `server/internal/idempotency`

**Tipo principal:** `Executor`

**Responsabilidade:** coordenar a execução de operações mutáveis identificadas por `OperationID`, impedindo a repetição da execução enquanto o registro correspondente estiver retido na mesma instância do servidor.

O Executor mantém registros próprios em memória, separados do Storage de dados de negócio.

Ele é criado durante a composição do servidor e compartilhado entre os Handlers das conexões da mesma instância.

### 10.1 Identificador de operação

O identificador possui exatamente 16 bytes (128 bits).

É obrigatório para:

- `SET`;
- `DELETE`.

Não é utilizado para:

- `PING`;
- `GET`.

O `OperationID` identifica a operação lógica entre tentativas. O `RequestID` identifica cada tentativa de comunicação.

### 10.2 Fingerprint

O Handler calcula um fingerprint SHA-256 determinístico dos parâmetros semânticos da operação, utilizando `idempotency.Fingerprint()`.

O fingerprint utiliza um esquema versionado e considera:

| Parâmetro | SET | DELETE |
|---|---|---|
| Tipo da operação | Sim | Sim |
| Chave | Sim | Sim |
| Valor | Sim | Não |
| TTL | Sim | Não |

O `OperationID` não integra o fingerprint.

Assim, o mesmo identificador somente pode ser reutilizado quando os parâmetros semânticos forem equivalentes.

O Executor recebe o fingerprint calculado pelo Handler e é responsável por compará-lo com o registro existente. O cálculo do fingerprint não é responsabilidade do Executor.

### 10.3 Método `Execute`

O Executor disponibiliza o método:

```go
Execute(
    operationID []byte,
    requestID uint64,
    fingerprint [32]byte,
    execute func() *protocol.Response,
)
```

O método recebe o identificador da operação, o identificador da tentativa, o fingerprint e uma função que executa a operação original.

O fluxo geral é:

1. Valida o `OperationID`.
2. Procura um registro existente.
3. Compara o fingerprint.
4. Verifica se a operação está em andamento ou concluída.
5. Reserva atomicamente um identificador inédito, quando necessário.
6. Executa a função recebida fora do mutex interno.
7. Armazena o resultado terminal reproduzível, quando aplicável.
8. Disponibiliza o resultado às chamadas concorrentes ou posteriores.

A execução fora do mutex permite que operações com identificadores diferentes avancem concorrentemente.

### 10.4 Comportamento por situação

| Situação | Comportamento |
|---|---|
| `OperationID` inédito e capacidade disponível | Reserva e executa a operação |
| Mesmo ID e mesmo fingerprint, operação concluída | Reproduz o resultado original |
| Mesmo ID e mesmo fingerprint, operação em andamento | Aguarda a execução original |
| Mesmo ID e fingerprint diferente | Retorna conflito |
| ID inválido | Rejeita a operação |
| Capacidade esgotada para um ID inédito | Rejeita antes da execução, sem reservar o identificador |

Quando uma operação é reproduzida, o resultado semântico original é preservado e o `RequestID` é atualizado para corresponder à tentativa atual.

Isso permite que uma nova tentativa receba uma resposta correlacionada ao seu próprio `RequestID`, sem executar novamente a mutação.

### 10.5 Concorrência

O Executor coordena operações concorrentes com o mesmo identificador.

Quando duas chamadas apresentam o mesmo `OperationID` e fingerprint enquanto a primeira ainda está em andamento, a segunda aguarda a conclusão da execução original.

Quando os identificadores são diferentes, as operações podem executar concorrentemente, sem manter o mutex interno durante a execução do Engine.

### 10.6 Capacidade e retenção

A implementação utiliza os seguintes valores padrão:

| Propriedade | Valor padrão |
|---|---|
| Capacidade | 100.000 registros |
| Retenção | 24 horas após a conclusão |
| Escopo | Global por instância do servidor |
| Armazenamento | Memória |
| Evicção antecipada de registros válidos | Não permitida |

A capacidade e a retenção são configuráveis na inicialização do servidor:

| Variável | Padrão | Descrição |
|---|---:|---|
| `MEMORYDB_IDEMPOTENCY_CAPACITY` | `100000` | Capacidade máxima de registros |
| `MEMORYDB_IDEMPOTENCY_RETENTION` | `24h` | Retenção dos registros concluídos |

A capacidade contabiliza operações em andamento e concluídas.

Registros válidos não são removidos antecipadamente para liberar espaço.

Quando a capacidade está esgotada, operações inéditas são rejeitadas antes da execução e não reservam o identificador.

Operações em andamento não expiram pela janela de retenção.

### 10.7 Resultados e falhas

Respostas terminais reproduzíveis, inclusive erros determinísticos, são armazenadas para replay.

O Executor não deve liberar uma reserva quando não é possível determinar se a mutação foi executada.

Essa situação é representada por resultado indeterminado.

Entre os erros definidos pelo pacote estão:

- `ErrInvalidOperationID`;
- `ErrFingerprintConflict`;
- `ErrCapacityReached`;
- `ErrOutcomeUnknown`;
- `ErrNilExecution`.

Uma falha comprovadamente anterior à execução da mutação pode permitir a liberação da reserva. Quando o resultado é indeterminado, a reserva não deve ser liberada de forma que permita uma execução duplicada.

### 10.8 Clonagem de respostas

O Executor mantém cópias das respostas utilizadas para replay.

A clonagem evita que a resposta entregue a uma chamada seja modificada externamente e altere o resultado armazenado para tentativas posteriores.

A resposta reproduzida mantém o resultado semântico original e utiliza o `RequestID` da tentativa atual.

### 10.9 Garantias e limitações

A garantia é de no máximo uma execução por `OperationID` enquanto o registro permanecer retido na mesma instância do servidor.

O mecanismo não oferece:

- persistência dos registros de idempotência;
- recuperação após reinicialização ou crash;
- deduplicação entre instâncias diferentes;
- garantia de execução exatamente uma vez;
- retry automático no driver.

O encerramento ou reinício do processo elimina os registros.

A expiração de um registro após o período de retenção também encerra a garantia de deduplicação daquele identificador.

A aplicação consumidora é responsável por fornecer e reutilizar o mesmo `OperationID` nas tentativas da mesma operação lógica.

## 11. Storage

**Localização:** `server/internal/storage`

**Tipos principais:** `Storage` e `Entry`

**Responsabilidade:** manter pares chave-valor em memória.

### 11.1 Estrutura

O Storage utiliza:

- um mapa `map[string]Entry`;
- `sync.RWMutex` para sincronização;
- valores armazenados como `[]byte`.

A estrutura `Entry` mantém o valor em um campo não exportado.

### 11.2 API

| Método | Comportamento |
|---|---|
| `New()` | Cria um Storage vazio |
| `Set(key, value)` | Armazena uma cópia do valor |
| `Get(key)` | Retorna uma cópia do valor e indica se a chave existe |
| `Delete(key)` | Remove a chave e informa se ela existia |

Assinaturas relevantes:

```go
Set(key string, value []byte)

Get(key string) ([]byte, bool)

Delete(key string) bool
```

### 11.3 Isolamento dos dados

O Storage copia os bytes recebidos em `Set` e os bytes retornados em `Get`.

Essa estratégia evita que alterações posteriores nos slices do chamador modifiquem diretamente os dados armazenados ou que o chamador altere o conteúdo interno por meio do valor retornado.

### 11.4 Concorrência

`Set` e `Delete` utilizam lock exclusivo.

`Get` utiliza read lock.

O controle permite operações concorrentes sem acesso desprotegido ao mapa.

### 11.5 Limitações

- Dados exclusivamente em memória.
- Sem persistência.
- Sem expiração.
- Sem replicação.
- Sem limite de memória configurável identificado.
- Sem política de eviction identificada.
- Os registros de idempotência não são armazenados neste componente.

## 12. Transporte TCP reutilizável

**Módulo:** `transport/tcp/go`

**Pacotes principais:** framing e conexão.

**Responsabilidade:** fornecer framing e abstração de conexão sem dependência do Protobuf ou do servidor.

### 12.1 Framing

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

1. Valida a presença do reader.
2. Lê integralmente o cabeçalho.
3. Interpreta o tamanho em Big Endian.
4. Rejeita tamanho zero.
5. Rejeita payload maior que o limite.
6. Lê integralmente o payload.

`WriteFrame`:

1. Valida a presença do writer.
2. Rejeita payload vazio.
3. Rejeita payload acima do limite.
4. Escreve o cabeçalho Big Endian.
5. Escreve o payload integralmente.

A função interna `writeFull` trata escritas parciais e rejeita contagens inválidas ou escritas sem progresso.

O limite de 1 MiB é aplicado ao payload de cada frame, sem incluir o cabeçalho de quatro bytes.

### 12.2 Connection

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

## 13. Driver Go

**Módulo:** `drivers/go`

**Pacote:** `memorydb`

**Arquivo principal:** `drivers/go/client.go`

**Responsabilidade:** disponibilizar uma API Go para aplicações clientes acessarem o MemoryDB.

O driver é uma biblioteca e não carrega automaticamente configurações de variáveis de ambiente. A aplicação consumidora é responsável por carregar suas configurações e fornecê-las ao driver por meio de sua API.

### 13.1 Tipo `Client`

O cliente encapsula:

- conexão TCP;
- mecanismo de serialização das operações;
- estado interno;
- controle de fechamento;
- contador de `request_id`.

O valor zero de `Client` não é utilizável. A criação deve ocorrer por `Dial`, `DialContext`, `DialWithConfig`, `DialContextWithConfig` ou `New`.

### 13.2 Identificador de operação

**Arquivo:** `drivers/go/operation_id.go`

O driver disponibiliza o tipo:

```go
type OperationID [OperationIDSize]byte
```

A constante `OperationIDSize` define o tamanho de 16 bytes.

A função:

```go
NewOperationID() (OperationID, error)
```

gera um identificador utilizando `crypto/rand`.

A aplicação consumidora é responsável por gerar e manter o identificador associado à operação lógica.

Para uma nova operação lógica, deve ser gerado um novo identificador.

Para repetir uma tentativa da mesma operação, deve ser reutilizado o identificador original.

O driver não gera identificadores implicitamente e não realiza retries automáticos.

### 13.3 Construção, configuração e fechamento

O driver disponibiliza as seguintes formas de construção:

| Método | Comportamento |
|---|---|
| `Dial(address)` | Estabelece conexão TCP |
| `DialContext(ctx, address)` | Estabelece conexão TCP utilizando contexto |
| `DialWithConfig(config)` | Estabelece conexão utilizando configuração explícita |
| `DialContextWithConfig(ctx, config)` | Estabelece conexão utilizando configuração explícita e contexto |
| `New(conn)` | Cria cliente a partir de uma conexão existente |
| `Close()` | Encerra a conexão e impede novas operações |

A configuração explícita utiliza:

```go
type DialConfig struct {
    Address string
    Timeout time.Duration
}
```

`Dial` e `DialContext` são preservados como formas de construção compatíveis com a API anterior. Os métodos com configuração permitem que a aplicação consumidora forneça endereço e timeout sem transferir ao driver a responsabilidade de ler variáveis de ambiente.

A validação da configuração rejeita endereço vazio e timeout negativo. Timeout igual a zero representa ausência de timeout explícito de conexão.

Quando criado por `New`, o cliente assume a propriedade da conexão e a encerra ao executar `Close`.

`DialContext` e `DialContextWithConfig` utilizam o contexto para controlar somente o estabelecimento da conexão. Ele não fica associado às operações posteriores.

### 13.4 Operações

| Método | Comportamento |
|---|---|
| `Ping()` | Verifica a disponibilidade do servidor |
| `PingContext(ctx)` | Executa PING com contexto |
| `Set(operationID, key, value)` | Grava um valor com identificador de operação |
| `SetContext(ctx, operationID, key, value)` | Grava um valor com contexto e identificador de operação |
| `Get(key)` | Recupera valor e existência |
| `GetContext(ctx, key)` | Recupera valor e existência com contexto |
| `Delete(operationID, key)` | Remove uma chave com identificador de operação |
| `DeleteContext(ctx, operationID, key)` | Remove uma chave com contexto e identificador de operação |

O `Set` do driver atual não expõe parâmetro de TTL. O servidor recebe o campo `ttl_ms` no contrato Protobuf, mas rejeita valores positivos.

### 13.5 Serialização das operações

O cliente permite que seus métodos sejam chamados concorrentemente.

Entretanto, `operationGate` permite somente uma operação por vez na conexão.

A serialização é necessária porque o protocolo atual não implementa multiplexação. O cliente envia uma solicitação e aguarda a resposta correspondente antes de liberar a conexão para a próxima operação.

### 13.6 Estados internos

O cliente possui três estados:

| Estado | Significado |
|---|---|
| `open` | Conexão disponível |
| `closed` | Cliente encerrado explicitamente |
| `failed` | Conexão invalidada após falha de comunicação ou protocolo |

A transição para `failed` fecha a conexão e impede novas operações.

`Close` pode ser chamado concorrentemente com operações. Como o fechamento interrompe o I/O, uma operação que já começou pode terminar com resultado indeterminado.

### 13.7 Contextos e cancelamento

O cliente diferencia o cancelamento enquanto aguarda a conexão para executar uma operação do cancelamento durante a comunicação.

| Momento | Resultado | Estado esperado |
|---|---|---|
| Antes de adquirir `operationGate` | Erro de contexto; a operação não começou | `open` |
| Durante a espera por `operationGate` | Erro de contexto; a operação não começou | `open` |
| Durante comunicação de rede | Resultado potencialmente indeterminado | `failed` |

O cancelamento durante I/O fecha a conexão para interromper a operação bloqueada.

Como a solicitação pode já ter sido processada pelo servidor, não é possível concluir apenas pelo cancelamento se a operação foi ou não aplicada.

A implementação sincroniza a conclusão e a interrupção por meio de fases internas da operação. O resultado observado depende de qual transição é efetivada primeiro.

### 13.8 `ServerError`

**Tipo:** `ServerError`

Representa um erro estruturado recebido em uma resposta válida do servidor.

Campos:

| Campo | Descrição |
|---|---|
| `Code` | Código do erro |
| `Message` | Mensagem retornada |

O erro indica uma rejeição ou falha de aplicação informada pelo servidor. Ele não invalida automaticamente a conexão.

### 13.9 `OperationOutcomeUnknownError`

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

Esse erro local do driver deve ser distinguido da resposta de protocolo `OPERATION_OUTCOME_UNKNOWN`, que é produzida pelo servidor quando o próprio mecanismo de idempotência identifica um resultado indeterminado.

### 13.10 Recuperação de operações mutáveis

Quando uma operação mutável retorna `OperationOutcomeUnknownError`, a aplicação pode realizar uma nova tentativa utilizando o mesmo `OperationID`.

A repetição precisa preservar os parâmetros semânticos da operação original.

Para `SET`, isso inclui:

- a mesma chave;
- o mesmo valor;
- o mesmo TTL.

Para `DELETE`, inclui a mesma chave.

O servidor utiliza o Executor para reconhecer a operação original e reproduzir o resultado, desde que o registro correspondente permaneça retido na mesma instância.

O driver não realiza retry automático, reconexão transparente ou recuperação de resposta anterior.

Se o servidor for reiniciado, ou se o registro expirar, a proteção de deduplicação deixa de estar disponível.

### 13.11 Limitações do driver

- Não há multiplexação.
- Não há retry automático.
- Não há reconexão automática.
- Não há recuperação de resultado após reconexão.
- O `RequestID` é utilizado para correlação, não para deduplicação.
- A aplicação consumidora é responsável por gerar e reutilizar `OperationID`.
- A garantia de idempotência depende da retenção do registro na mesma instância do servidor.
- Não há garantia de execução exatamente uma vez.
- O driver não carrega variáveis de ambiente automaticamente; a aplicação consumidora deve fornecer a configuração.

## 14. Schema Protobuf e tipos gerados

**Schema:** `protocol/proto/memorydb/v1/memorydb.proto`

**Módulo gerado:** `protocol/generated/go`

O schema define os envelopes de solicitação e resposta, os comandos, os resultados e os erros.

O campo `request_id` é definido como `uint64`.

Os comandos e resultados são representados por `oneof`, permitindo selecionar uma das alternativas declaradas no contrato.

### 14.1 Identificadores

Os comandos `SET` e `DELETE` possuem o campo `operation_id`, representado como `bytes`.

Esse campo transporta o identificador de operação de 16 bytes utilizado pelo mecanismo de idempotência.

O campo é obrigatório para esses comandos. A validação de tamanho é realizada pelo fluxo de idempotência no servidor.

O `request_id` permanece independente do `operation_id`:

- `request_id` identifica a tentativa;
- `operation_id` identifica a operação lógica entre tentativas.

### 14.2 TTL

O campo `ttl_ms` existe em `SetCommand`, mas sua funcionalidade não está implementada no Engine.

Valores positivos são rejeitados com `UNSUPPORTED_FEATURE`.

### 14.3 Geração dos tipos

Os arquivos gerados devem ser atualizados a partir do schema por meio do processo de geração adotado pelo projeto.

Os arquivos Go gerados devem ser tratados como artefatos de geração, não como arquivos para edição manual.

**Ponto de atenção:** o processo oficial de geração, incluindo versão e instalação das ferramentas, deve ser documentado em um procedimento reproduzível caso ainda não esteja definido em outro documento do repositório.

## 15. API de demonstração

**Localização:** `api/client`

**Arquivo principal:** `api/client/main.go`

O executável demonstra o uso do driver Go.

Ele utiliza a API pública do driver para se comunicar com o servidor. Não acessa diretamente os pacotes internos do servidor nem implementa um protocolo alternativo.

É uma aplicação de demonstração, não uma segunda implementação do servidor.

As operações mutáveis demonstradas geram e fornecem `OperationID` ao driver.

### 15.1 Configuração

A aplicação de demonstração possui configuração própria, carregada por `api/client/internal/config`.

| Variável | Padrão | Descrição |
|---|---|---|
| `MEMORYDB_CLIENT_ADDR` | `127.0.0.1:6380` | Endereço do servidor ao qual o cliente se conecta |
| `MEMORYDB_DIAL_TIMEOUT` | `5s` | Timeout para estabelecer a conexão TCP |

Variáveis ausentes ou vazias utilizam os valores padrão. Valores explicitamente informados, mas inválidos, fazem o carregamento da configuração retornar erro.

A aplicação injeta os valores carregados na configuração do driver por meio de `DialWithConfig`.

O driver não consulta essas variáveis diretamente.

O endereço do cliente é independente do endereço de escuta do servidor. Portanto, ao alterar `MEMORYDB_ADDR` para utilizar outra porta ou endereço, também é necessário configurar `MEMORYDB_CLIENT_ADDR` de acordo com o endereço no qual o servidor está acessível.

## 16. Testes por componente

Os testes automatizados são executados por meio dos scripts centralizados em `scripts/test/`. O repositório possui múltiplos módulos Go, e o pipeline percorre os módulos configurados no workspace.

Os procedimentos de execução, os comandos disponíveis, os critérios de validação e os relatórios de cobertura estão descritos no [Guia de testes](./TESTES.md).

Os testes estão distribuídos junto aos módulos e pacotes.

| Componente | Arquivo de teste | Cobertura identificada |
|---|---|---|
| Framing TCP | `transport/tcp/go/framing_test.go` | Big Endian, fragmentação, limites, truncamento, interfaces nulas e escritas parciais |
| Connection TCP | `transport/tcp/go/connection_test.go` | Fechamento, I/O bloqueado, concorrência e full-duplex |
| Driver Go | `drivers/go/client_test.go` | Operações, concorrência, contextos, erros, desconexões e resultados indeterminados |
| Codec Protobuf | `server/internal/codec/protobuf/codec_test.go` | Round-trip, conversões e mensagens inválidas |
| Engine | `server/internal/engine/engine_test.go` | Comandos, validações e erros |
| Storage | `server/internal/storage/storage_test.go` | Operações, cópia de bytes e concorrência |
| Executor de idempotência | `server/internal/idempotency/executor_test.go` | Concorrência, replay, conflitos, capacidade, expiração e resultados indeterminados |
| Fingerprint | `server/internal/idempotency/fingerprint_test.go` | Determinismo, diferenciação de operações e parâmetros semânticos |
| Handler | `server/internal/transport/tcp/handler_test.go` | Processamento de solicitações, integração com idempotência, replay, erros e timeouts de leitura e escrita |
| Server TCP | `server/internal/transport/tcp/server_test.go` | Aceitação de clientes, limite de conexões, concorrência, encerramento gracioso e fechamento forçado |
| Integração | `server/internal/integration/server_test.go` | Comunicação entre driver e servidor |

### 16.1 Testes do Executor de idempotência

A cobertura do Executor inclui cenários de:

- execução de uma operação inédita;
- replay de operação concluída;
- rejeição de identificador reutilizado com fingerprint diferente;
- concorrência entre chamadas com o mesmo identificador;
- execução concorrente de identificadores diferentes;
- rejeição por capacidade esgotada;
- expiração de registros concluídos;
- preservação de operações em andamento;
- rejeição de identificadores inválidos;
- tratamento de execução nula;
- tratamento de respostas nulas ou tipadas nulas;
- preservação de resultados indeterminados;
- clonagem de respostas para replay.

### 16.2 Testes do Handler para idempotência

A cobertura do Handler inclui:

- encaminhamento de `SET` e `DELETE` ao Executor;
- execução direta de `PING` e `GET`;
- replay de operações;
- atualização do `RequestID` durante replay;
- rejeição de `OperationID` inválido;
- rejeição de conflito de fingerprint;
- rejeição por capacidade esgotada;
- tratamento de comandos tipados nulos.

### 16.3 Testes do driver para resultados indeterminados

A cobertura do driver inclui cenários específicos de:

- escrita parcial;
- execução de SET no servidor seguida de perda da resposta;
- cancelamento durante a leitura da resposta;
- preservação da causa original;
- transição para o estado `failed`;
- cancelamento antes do início da operação;
- concorrência entre conclusão e cancelamento.

### 16.4 Validação dos módulos

Em 22/11/2026, foi registrada validação dos módulos do workspace com testes, detector de condições de corrida e análise estática.

Comandos executados:

```bash
for module in server transport/tcp/go protocol/generated/go drivers/go api; do
    (
        cd "$module" &&
        go test ./... &&
        go vet ./...
    ) || exit 1
done
```

Também foi executada a suíte com detector de condições de corrida e repetição dos testes:

```bash
for module in server transport/tcp/go protocol/generated/go drivers/go api; do
    (
        cd "$module" &&
        go test -race -count=20 ./...
    ) || exit 1
done
```

Segundo o registro dessa validação, os testes e as análises estáticas dos módulos foram concluídos sem erros.

Também foram verificados:

- formatação com `gofmt`;
- consistência do diff com `git diff --check`;
- sincronização entre o branch local e o remoto.

Esses resultados correspondem à validação registrada em 22/11/2026 e não substituem a execução dos testes após alterações futuras.

Para reproduzir as verificações, deve-se utilizar o pipeline centralizado descrito no [Guia de testes](./TESTES.md), em vez de depender dos comandos manuais registrados nesta seção.

## 17. Limitações transversais

| Ponto | Situação observada | Consequência |
|---|---|---|
| Erros do Handler | Retorno de `Handler.Handle` descartado pelo servidor | Falhas individuais não são propagadas nem registradas nesse ponto |
| Shutdown | Fechamento forçado das conexões restantes quando o contexto expira | O método ainda aguarda a conclusão das goroutines dos handlers após fechar as conexões |
| Storage | Engine depende de implementação concreta | Substituição do backend exige alteração do Engine |
| TTL | Campo existe no schema, mas TTL positivo é rejeitado | Não há expiração de entradas |
| Idempotência | Registros mantidos somente em memória | A garantia não sobrevive à reinicialização |
| Capacidade | Limite padrão de 100.000 registros, configurável | Novas operações podem ser rejeitadas quando a capacidade está esgotada |
| Retenção | Padrão de 24 horas após a conclusão, configurável | Após a expiração, o identificador pode deixar de ser reconhecido |
| Multiplexação | Driver serializa operações por conexão | Uma operação por vez em cada conexão |
| Observabilidade | Não há registro estruturado de falhas de conexão no servidor | Diagnóstico operacional limitado |
| Timeouts de I/O | Read e write deadlines configuráveis no Handler | Operações de leitura e escrita possuem prazos individuais; não há timeout global de execução do comando |
| Distribuição | Executor e Storage são locais à instância | Não há coordenação entre processos ou instâncias |
| Limite de conexões | Limite configurável de conexões TCP simultâneas | Conexões excedentes são fechadas; não há controle de admissão baseado em carga ou consumo de recursos |

## 18. Evoluções que envolvem múltiplos componentes

| Evolução | Componentes envolvidos | Resultado esperado |
|---|---|---|
| Persistência | Storage e recuperação | Preservação de dados após reinicialização |
| TTL | Schema, modelo interno, Engine e Storage | Expiração de entradas |
| Observabilidade | Server, Handler e driver | Registro e diagnóstico de falhas |
| Multiplexação | Protocolo, Handler e driver | Operações simultâneas na mesma conexão |
| Segurança | Server, transporte e autenticação | Proteção da comunicação e do acesso |
| Escalabilidade | Storage, protocolo e coordenação | Suporte a múltiplas instâncias e distribuição |

## 19. Diretrizes de manutenção dos componentes

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
- Manter as configurações documentadas alinhadas aos nomes das variáveis, valores padrão e regras de validação efetivamente implementados.

A documentação complementa os comentários do código e não deve substituí-los quando o contrato de uma API pública exigir explicação no próprio código.