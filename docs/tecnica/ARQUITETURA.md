# Arquitetura do MemoryDB

## 1. Visão geral

MemoryDB é um banco de dados chave-valor em memória, implementado em Go. Na implementação atual, o servidor recebe comandos `PING`, `SET`, `GET` e `DELETE` por TCP, utilizando Protocol Buffers para serializar as mensagens.

A arquitetura separa responsabilidades de:

- composição e ciclo de vida do servidor;
- transporte TCP e framing;
- codificação e decodificação de mensagens;
- representação interna das solicitações e respostas;
- execução dos comandos;
- armazenamento em memória;
- controle de idempotência para operações mutáveis;
- comunicação de aplicações clientes por meio de um driver Go.

O servidor mantém uma instância de `Engine`, que possui uma instância própria de `Storage`. Essa mesma instância de `Engine` é utilizada pelos handlers das conexões aceitas pelo servidor. Portanto, as conexões atendidas pelo mesmo processo compartilham o mesmo armazenamento.

O Handler utiliza um Executor de idempotência compartilhado entre as conexões do servidor. Esse componente coordena as operações `SET` e `DELETE`, evitando a execução repetida de uma mesma operação lógica enquanto seu registro permanecer retido na instância do servidor.

As operações `PING` e `GET` não passam pelo Executor de idempotência.

Os dados e os registros de idempotência permanecem exclusivamente na memória do processo. Não há persistência, recuperação após reinicialização ou replicação.

A arquitetura descrita neste documento representa a implementação existente, não uma arquitetura-alvo.

## 2. Organização dos módulos Go

O repositório utiliza múltiplos módulos Go coordenados pelo workspace da raiz (`go.work`).

| Módulo | Diretório | Responsabilidade |
|---|---|---|
| Servidor | `server/` | Composição do servidor, execução de comandos, armazenamento, codec e integração TCP |
| Transporte TCP | `transport/tcp/go/` | Framing e abstração reutilizável de conexão TCP |
| Driver Go | `drivers/go/` | Biblioteca cliente para aplicações Go |
| Protocolo gerado | `protocol/generated/go/` | Tipos Go gerados a partir do schema Protobuf |
| API de demonstração | `api/` | Executável que demonstra o uso do driver |
| Contrato Protobuf | `protocol/proto/` | Schema independente da linguagem |

Cada módulo mantém seu próprio `go.mod`. O `go.work` inclui os módulos locais utilizados no desenvolvimento e define substituições para dependências internas do projeto.

O driver e o servidor compartilham os módulos de transporte TCP e de tipos Protobuf gerados. O driver não importa os pacotes internos do servidor.

### 2.1 Dependências entre módulos

```mermaid
flowchart TD
    API["api/client"] --> Driver["drivers/go"]
    Driver --> TCP["transport/tcp/go"]
    Driver --> Generated["protocol/generated/go"]

    Server["server"] --> TCP
    Server --> Generated

    Schema["protocol/proto/memorydb/v1/memorydb.proto"]
    Schema -. geração .-> Generated
```

O schema `.proto` é a fonte do contrato externo. Os arquivos Go gerados devem ser tratados como artefatos de geração, não como arquivos para edição manual.

A presença de opções de geração para outras linguagens no schema não significa que existam drivers ou implementações dessas linguagens no estado atual.

### 2.2 Execução dos testes

O repositório utiliza múltiplos módulos Go coordenados por um workspace. Por isso, a execução dos testes e das verificações técnicas é realizada individualmente em cada módulo.

Os procedimentos de formatação, análise estática, testes unitários, detecção de condições de corrida, compilação e cobertura estão centralizados nos scripts de `scripts/test/`.

Consulte o [Guia de testes](./TESTES.md) para conhecer os comandos disponíveis, os critérios de validação e a geração dos relatórios.

## 3. Arquitetura interna do servidor

As responsabilidades internas do servidor estão distribuídas entre os seguintes pacotes:

| Pacote | Responsabilidade |
|---|---|
| `server/cmd/memorydb` | Inicialização, composição das dependências, configuração e encerramento |
| `server/internal/transport/tcp` | Aceitação de conexões e processamento de solicitações |
| `server/internal/codec` | Contrato de codificação e decodificação |
| `server/internal/codec/protobuf` | Adaptação entre o modelo interno e os tipos Protobuf |
| `server/internal/protocol` | Modelo interno de solicitações, comandos, respostas e resultados |
| `server/internal/engine` | Validação e execução dos comandos |
| `server/internal/idempotency` | Controle de identificadores de operação, fingerprints, deduplicação, concorrência, replay, capacidade e retenção |
| `server/internal/storage` | Armazenamento chave-valor em memória |

### 3.1 Diagrama de responsabilidades

```mermaid
flowchart TB
    Client["Cliente TCP / Driver Go"]

    subgraph Server["Módulo server"]
        Entry["cmd/memorydb"]
        TCPServer["transport/tcp.Server"]
        Handler["transport/tcp.Handler"]
        CodecPort["codec.Codec"]
        ProtoCodec["codec/protobuf.Codec"]
        Idempotency["idempotency.Executor"]
        Engine["engine.Engine"]
        Model["internal/protocol"]
        Storage["storage.Storage"]
    end

    TCPModule["Módulo transport/tcp/go"]
    Generated["Módulo protocol/generated/go"]

    Entry --> TCPServer
    Entry --> Handler
    Entry --> Idempotency
    TCPServer --> Handler
    Handler --> TCPModule
    Handler --> CodecPort
    Handler --> Idempotency
    Handler --> Engine
    ProtoCodec -. implementa .-> CodecPort
    ProtoCodec --> Model
    ProtoCodec --> Generated
    Engine --> Model
    Engine --> Storage
    Idempotency --> Model

    Client --> TCPServer
```

O diagrama representa responsabilidades e dependências. Não significa que o projeto implemente formalmente Clean Architecture, Hexagonal Architecture ou DDD.

### 3.2 Direção das dependências

O modelo interno (`server/internal/protocol`) não depende dos tipos gerados pelo Protobuf nem do transporte TCP.

O pacote `codec.Codec` define um contrato utilizado pelo Handler. A implementação concreta atual é `codec/protobuf.Codec`.

O transporte TCP reutilizável não conhece Protobuf, Engine, Storage ou os pacotes internos do servidor.

O `Engine`, por outro lado, depende diretamente de `*storage.Storage`. Não existe atualmente uma interface de armazenamento que permita substituir o backend sem alterar o Engine.

O pacote `server/internal/idempotency` implementa o controle de idempotência das operações `SET` e `DELETE`.

O Handler utiliza o Executor para reservar e verificar o `OperationID` antes de executar a operação no Engine. O Executor recebe a execução como uma função, mantendo o mecanismo de deduplicação desacoplado da implementação concreta do Engine.

O Executor mantém seus próprios registros em memória, separados dos dados de negócio armazenados pelo Storage.

A instância do Executor é criada durante a composição do servidor e compartilhada entre os handlers das conexões aceitas pelo mesmo processo.

### 3.3 Responsabilidades do Handler, Executor e Engine

A separação entre esses componentes é importante para compreender o fluxo de execução:

- **Handler:** interpreta a solicitação interna, encaminha `SET` e `DELETE` ao Executor e executa diretamente no Engine os comandos `PING` e `GET`.
- **Executor:** valida o identificador da operação, calcula e compara a identidade semântica da solicitação por meio do fingerprint, coordena operações concorrentes e controla os resultados armazenados para replay.
- **Engine:** valida os comandos de negócio e executa suas operações sobre o Storage.
- **Storage:** mantém os dados chave-valor em memória e protege o acesso concorrente.

O Engine não implementa idempotência e não valida o tamanho do `OperationID`. Essa responsabilidade pertence ao fluxo de idempotência utilizado pelo Handler.

## 4. Transporte e contrato de comunicação

A comunicação externa utiliza TCP. Cada mensagem é transmitida em um frame composto por:

- cabeçalho de 4 bytes;
- tamanho do payload representado por `uint32`;
- ordem de bytes Big Endian;
- payload com tamanho mínimo de 1 byte;
- tamanho máximo de 1 MiB.

O payload contém uma mensagem serializada em Protocol Buffers.

O framing é implementado em `transport/tcp/go/framing.go`. A abstração `Connection`, em `transport/tcp/go/connection.go`, encapsula `net.Conn` e fornece operações de leitura, escrita e fechamento.

### 4.1 Concorrência no transporte

A `Connection` utiliza mutexes independentes para leitura e escrita:

- operações de leitura concorrentes são serializadas;
- operações de escrita concorrentes são serializadas;
- uma leitura e uma escrita podem ocorrer simultaneamente;
- `Close` fecha diretamente a conexão subjacente e pode interromper operações de I/O bloqueadas.

A implementação de framing utiliza leitura completa e escrita em loop para tratar fragmentação e escritas parciais.

O transporte TCP é independente do protocolo de serialização. Isso permite reutilizá-lo sem acoplar o framing ao Protobuf.

### 4.2 Contrato Protobuf

O contrato externo está definido em `protocol/proto/memorydb/v1/memorydb.proto`.

O envelope `Request` contém:

- `request_id`;
- um comando selecionado por `oneof`.

O envelope `Response` contém:

- `request_id`;
- `success`;
- um resultado selecionado por `oneof`.

Os comandos definidos são `PING`, `SET`, `GET` e `DELETE`. O schema também define os respectivos resultados e uma estrutura de erro com código e mensagem.

O campo `request_id` permite correlacionar uma resposta com sua solicitação. Ele não é um identificador de operação estável entre tentativas e não oferece idempotência ou deduplicação.

Os comandos `SET` e `DELETE` também possuem o campo `operation_id`, representado como `bytes` no Protobuf.

Esse campo transporta o identificador de 128 bits utilizado pelo mecanismo de idempotência. O servidor valida se o identificador possui exatamente 16 bytes.

O `operation_id` é obrigatório para `SET` e `DELETE`. Sua ausência ou tamanho inválido resulta em erro de aplicação.

O campo não substitui o `request_id`: cada tentativa possui seu próprio identificador de requisição, enquanto as tentativas de uma mesma operação lógica reutilizam o mesmo identificador de operação.

### 4.3 Semântica dos comandos

| Comando | Comportamento implementado |
|---|---|
| `PING` | Retorna uma resposta `PONG` |
| `SET` | Armazena o valor associado à chave |
| `GET` | Retorna o valor e informa se a chave existe |
| `DELETE` | Remove a chave e informa se ela existia |

As validações de aplicação são distribuídas entre o fluxo de idempotência e o Engine.

O Engine realiza as seguintes validações:

- solicitação nula, comando ausente, desconhecido ou tipado nulo: `INVALID_REQUEST`;
- chave vazia em `SET`, `GET` ou `DELETE`: `INVALID_ARGUMENT`;
- `SET` com `ttl_ms` maior que zero: `UNSUPPORTED_FEATURE`;
- `SET` com TTL igual a zero: gravação sem expiração.

O `GET` diferencia uma chave inexistente de uma chave existente cujo valor é vazio. O `DELETE` informa se a chave foi efetivamente removida.

As operações `SET` e `DELETE` exigem um `OperationID` válido. O Handler encaminha essas operações ao Executor de idempotência antes de solicitar sua execução ao Engine.

`PING` e `GET` são executados diretamente pelo Engine, sem passar pelo Executor.

A validação do `OperationID` pertence ao fluxo de idempotência do Handler/Executor, não ao Engine. O Engine permanece responsável pela validação e execução dos comandos de negócio.

Erros de framing ou de decodificação Protobuf encerram o processamento da conexão. Eles não são necessariamente convertidos em uma resposta estruturada de aplicação.

## 5. Fluxo de processamento de uma solicitação

O Handler processa solicitações sequencialmente em cada conexão.

```mermaid
sequenceDiagram
    participant C as Cliente
    participant H as Handler
    participant P as Codec
    participant I as Idempotency Executor
    participant E as Engine
    participant St as Storage

    C->>H: Request TCP
    H->>P: DecodeRequest
    P-->>H: Request interno

    alt SET ou DELETE
        H->>I: Execute(OperationID, RequestID, Fingerprint)
        I->>I: Validar identificador e fingerprint

        alt Operação inédita
            I->>I: Reservar identificador
            I->>E: Executar operação
            E->>St: Mutação
            St-->>E: Resultado
            E-->>I: Response
            I->>I: Armazenar resultado terminal
            I-->>H: Resultado original
        else Operação repetida concluída
            I->>I: Recuperar resultado registrado
            I-->>H: Resultado reproduzido
        else Operação em andamento
            I->>I: Aguardar execução original
            I-->>H: Resultado original
        else Fingerprint divergente
            I-->>H: Erro de conflito
        end
    else PING ou GET
        H->>E: Execute(Request)
        E->>St: Consultar ou executar comando
        St-->>E: Resultado
        E-->>H: Response
    end

    H->>P: EncodeResponse
    P-->>H: Payload Protobuf
    H-->>C: Response TCP
```

Para `SET` e `DELETE`, o Handler calcula o fingerprint da operação e solicita sua execução ao Executor de idempotência.

O Executor valida o `OperationID` e coordena a reserva do identificador. A reserva ocorre atomicamente antes da invocação do Engine. A execução da operação ocorre fora do mutex interno do Executor, permitindo que operações distintas avancem concorrentemente.

Quando uma operação com o mesmo identificador e fingerprint já está em andamento, a nova tentativa aguarda sua conclusão. Quando já existe um resultado terminal registrado, ele é reproduzido sem executar novamente a mutação.

Em caso de replay, o resultado semântico original é preservado, mas o `RequestID` da resposta é atualizado para corresponder à tentativa atual.

Se o mesmo identificador for apresentado com parâmetros semanticamente diferentes, a operação é rejeitada por conflito.

O Handler executa o seguinte ciclo de processamento por conexão:

1. cria uma abstração `Connection` para a conexão recebida;
2. lê um frame;
3. decodifica a solicitação;
4. encaminha `SET` e `DELETE` ao Executor de idempotência, que coordena sua execução no Engine; para `PING` e `GET`, chama diretamente o Engine;
5. codifica a resposta;
6. escreve o frame de resposta;
7. repete o processamento enquanto a conexão permanecer aberta.

Uma falha de leitura, decodificação, codificação ou escrita encerra o processamento daquela conexão.

O Handler retorna o erro ao servidor. Atualmente, a goroutine criada por `Server.Serve()` descarta esse retorno, sem registro estruturado da falha.

### 5.1 Resultados e erros do fluxo de idempotência

O Executor pode produzir resultados associados aos seguintes cenários:

| Condição | Resultado |
|---|---|
| Identificador ausente ou com tamanho inválido | `INVALID_OPERATION_ID` |
| Mesmo identificador com fingerprint diferente | `OPERATION_ID_CONFLICT` |
| Capacidade esgotada para identificador inédito | `IDEMPOTENCY_CAPACITY_REACHED` |
| Resultado da operação não pode ser determinado com segurança | `OPERATION_OUTCOME_UNKNOWN` |

Esses códigos representam erros estruturados do fluxo de aplicação no servidor. São diferentes do erro local `OperationOutcomeUnknownError`, utilizado pelo driver para representar uma falha de comunicação na qual o cliente não consegue confirmar o resultado da operação.

O Handler converte os erros do Executor em respostas do protocolo. Outros erros não classificados seguem o tratamento de erro interno previsto pelo Handler.

## 6. Armazenamento e ciclo de vida dos dados

O pacote `server/internal/storage` mantém os dados em um mapa indexado por chave (`string`), com valores em `[]byte`.

O acesso concorrente é protegido por `sync.RWMutex`.

O Storage:

- copia os bytes recebidos em `Set`;
- copia os bytes retornados em `Get`;
- informa se uma chave existe;
- remove entradas por `Delete`.

A estrutura `Entry` guarda o valor. Não há campo de expiração.

A instância de `Storage` é criada internamente por `engine.New()`. Não existe atualmente um construtor do Engine que receba uma implementação externa de Storage.

O servidor cria uma instância de Engine durante sua composição e utiliza esse Engine para processar as conexões. Assim, as conexões de um mesmo processo compartilham os dados.

Os registros de idempotência não são armazenados nesse mapa. O Executor mantém seu próprio estado em memória, independente do Storage.

### 6.1 Limites do armazenamento atual

- Os dados são voláteis e existem apenas na memória do processo.
- O encerramento ou reinício do processo elimina os dados.
- Não há persistência em disco.
- Não há replicação.
- Não há compartilhamento de dados entre processos ou instâncias.
- TTL positivo não é implementado.

## 7. Driver Go e modelo de comunicação

O módulo `drivers/go` fornece uma biblioteca para aplicações Go se comunicarem com o servidor.

O driver depende do transporte TCP reutilizável e dos tipos Protobuf gerados. Não depende dos pacotes internos do servidor.

A API inclui:

- `Dial` e `DialContext`;
- `New`;
- `Close`;
- `Ping` e `PingContext`;
- `Set` e `SetContext`;
- `Get` e `GetContext`;
- `Delete` e `DeleteContext`.

As operações são serializadas por conexão por meio de `operationGate`. O driver permite chamadas concorrentes, mas apenas uma operação utiliza a conexão por vez.

Esse comportamento é necessário porque o protocolo atual trabalha com uma solicitação e sua resposta por vez na mesma conexão. Não existe multiplexação.

### 7.1 Identificação e correlação

O driver utiliza dois identificadores com responsabilidades distintas:

| Identificador | Responsabilidade | Ciclo de vida |
|---|---|---|
| `RequestID` | Correlacionar uma requisição com sua resposta | Novo para cada tentativa |
| `OperationID` | Identificar uma operação lógica mutável | Reutilizado nas tentativas da mesma operação |

O `RequestID` é gerado pelo cliente e utilizado para correlacionar a resposta recebida. Ele não é estável entre tentativas.

O `OperationID` é representado pelo tipo `memorydb.OperationID`, com 16 bytes. A função `NewOperationID()` gera um identificador utilizando `crypto/rand`.

As operações `Set`, `SetContext`, `Delete` e `DeleteContext` recebem o identificador como argumento obrigatório.

A aplicação consumidora é responsável por manter e reutilizar o mesmo `OperationID` quando decidir repetir uma tentativa da mesma operação lógica.

Para uma nova operação lógica, deve ser gerado um novo identificador.

O driver não realiza retries automáticos e não gera identificadores implicitamente para as operações mutáveis.

### 7.2 Estados do cliente

O driver mantém três estados internos:

| Estado | Significado |
|---|---|
| `open` | Cliente disponível para operações |
| `closed` | Cliente encerrado explicitamente |
| `failed` | Conexão invalidada após falha de comunicação ou protocolo |

O cliente não deve ser reutilizado após entrar em `closed` ou `failed`.

### 7.3 Contextos e cancelamento

`DialContext` utiliza o contexto apenas para controlar o estabelecimento da conexão. Esse contexto não fica associado às operações posteriores.

Cada operação pode receber seu próprio contexto.

O driver distingue:

- **Cancelamento enquanto aguarda acesso à conexão:** a operação não começou, o erro de contexto é retornado e a conexão permanece disponível.
- **Cancelamento durante I/O:** a conexão é fechada para interromper a comunicação e o resultado pode ser indeterminado.

O fechamento explícito do cliente também pode interromper uma operação de rede em andamento.

### 7.4 Resultado indeterminado

O driver disponibiliza `OperationOutcomeUnknownError` para representar uma operação cuja execução não pode ser confirmada.

O erro contém `RequestID` e `Cause`, além de implementar `Unwrap()` para preservar a inspeção da causa original.

O resultado pode ser indeterminado quando, por exemplo:

- parte da solicitação foi transmitida antes de uma falha de escrita;
- a solicitação foi enviada e a resposta não chegou;
- o contexto foi cancelado durante a comunicação;
- a conexão foi interrompida;
- a resposta recebida não pôde ser decodificada ou validada.

Esse erro não significa que o servidor deixou de executar a operação. Significa que o cliente não possui confirmação suficiente para determinar o resultado.

O driver não realiza retry automático. Repetir uma operação após resultado indeterminado exige que a aplicação considere a possibilidade de a operação original já ter sido executada.

Quando uma operação mutável retorna `OperationOutcomeUnknownError`, a aplicação pode utilizar o mesmo `OperationID` em uma nova tentativa, desde que mantenha os mesmos parâmetros semânticos da operação original.

Para `SET`, isso inclui manter a mesma chave, o mesmo valor e o mesmo TTL.

A repetição poderá ser protegida pelo Executor enquanto o registro correspondente estiver retido na mesma instância do servidor. Essa proteção não pode ser presumida após a expiração do registro ou a reinicialização do processo.

Essa garantia não equivale a uma garantia de execução exatamente uma vez.

### 7.5 Erros de aplicação

Quando o servidor retorna uma resposta estruturada de erro válida, o driver a representa por `ServerError`.

Esse cenário é diferente de uma falha de comunicação: o cliente recebeu uma resposta de aplicação reconhecível e não invalida automaticamente a conexão.

O erro `OPERATION_OUTCOME_UNKNOWN` retornado pelo servidor e o erro local `OperationOutcomeUnknownError` do driver representam situações relacionadas, mas distintas:

- o primeiro é um código de erro do protocolo produzido pelo servidor;
- o segundo é um erro do cliente que indica que uma falha de comunicação impediu a confirmação do resultado.

## 8. Inicialização e encerramento do servidor

O executável está em `server/cmd/memorydb/main.go`.

A configuração de endereço utiliza:

| Configuração | Comportamento |
|---|---|
| `MEMORYDB_ADDR` | Endereço TCP utilizado pelo listener |
| Valor padrão | `:6380` |

O ponto de entrada realiza a composição do servidor:

1. lê a configuração do endereço TCP;
2. cria o listener TCP;
3. instancia o codec Protobuf;
4. instancia o Engine;
5. cria uma instância compartilhada do Executor de idempotência;
6. cria o Handler, injetando Engine, Codec e Executor;
7. cria o servidor;
8. inicia `Server.Serve()` em uma goroutine;
9. trata `SIGINT` e `SIGTERM`;
10. solicita encerramento com timeout de 10 segundos.

O Executor é criado uma única vez durante a composição do servidor e compartilhado pelos handlers das conexões atendidas pelo mesmo processo.

### 8.1 Semântica do shutdown

`Server.Shutdown(ctx)` marca o servidor como encerrando, fecha o listener e aguarda a conclusão das conexões ativas.

O método não fecha diretamente as conexões ativas.

Se uma conexão não terminar, o método pode retornar o erro do contexto quando o prazo expirar, enquanto a conexão permanece aberta. Não há atualmente uma etapa de encerramento forçado das conexões ativas.

O encerramento gracioso, portanto, depende de as conexões terminarem por conta própria dentro do prazo.

## 9. Limites arquiteturais atuais

| Área | Estado atual |
|---|---|
| Persistência | Não implementada |
| Replicação | Não implementada |
| Distribuição entre instâncias | Não implementada |
| Autenticação e autorização | Não implementadas |
| TLS | Não implementado |
| TTL | Campo no protocolo; funcionalidade não implementada |
| Idempotência | Implementada para `SET` e `DELETE` |
| Deduplicação server-side | Implementada na instância do servidor |
| Retry automático | Não implementado |
| Multiplexação | Não implementada |
| Timeouts de I/O configuráveis no servidor | Não identificados na implementação atual |
| Observabilidade estruturada por conexão | Não implementada no fluxo atual |
| Shutdown forçado de conexões | Não implementado |

### 9.1 Idempotência e deduplicação

**Estado: implementado e validado em 22/11/2026.**

O MemoryDB implementa idempotência para operações `SET` e `DELETE`, por meio do pacote `server/internal/idempotency`.

O mecanismo é executado pelo Handler antes da chamada ao Engine e mantém registros independentes dos dados de negócio do Storage.

#### Identificador

O `OperationID` possui exatamente 16 bytes (128 bits) e é obrigatório para operações `SET` e `DELETE`.

O `RequestID` continua sendo utilizado para correlacionar cada tentativa com sua resposta. Os identificadores possuem finalidades distintas.

#### Fingerprint

O Executor utiliza um fingerprint SHA-256 determinístico para identificar os parâmetros semânticos da operação.

O fingerprint considera:

- tipo da operação;
- chave;
- valor, para `SET`;
- TTL, para `SET`.

O `OperationID` não integra o fingerprint.

O fingerprint permite verificar se uma tentativa que reutiliza um identificador corresponde à mesma operação lógica. A utilização de um identificador já registrado com parâmetros diferentes resulta em conflito.

#### Retenção e capacidade

A implementação utiliza:

- retenção de 24 horas após a conclusão da operação;
- capacidade padrão de 100.000 registros;
- armazenamento exclusivamente em memória;
- compartilhamento dos registros entre as conexões da mesma instância.

A capacidade contabiliza operações em andamento e concluídas.

Registros válidos não são removidos antecipadamente para liberar espaço. Quando a capacidade está esgotada, operações inéditas são rejeitadas antes da execução e não reservam o identificador.

Operações em andamento não expiram pela janela de retenção.

#### Comportamento

| Condição | Comportamento |
|---|---|
| `OperationID` inédito | Reserva e executa a operação |
| Mesmo ID e mesmo fingerprint, operação concluída | Reproduz o resultado original |
| Mesmo ID e mesmo fingerprint, operação em andamento | Aguarda a execução original |
| Mesmo ID e fingerprint diferente | Retorna conflito |
| ID inválido | Rejeita a operação |
| Capacidade esgotada | Rejeita operação inédita antes da execução |

O replay preserva o resultado semântico original e atualiza o `RequestID` para a tentativa atual.

Respostas terminais reproduzíveis, inclusive erros determinísticos, são armazenadas para replay.

O Executor não libera uma reserva quando não é possível determinar se a mutação foi executada. Essa condição é representada por resultado indeterminado.

#### Garantias e limitações

A garantia é de no máximo uma execução por `OperationID` enquanto o registro permanecer retido na mesma instância do servidor.

A implementação não oferece:

- persistência dos registros de idempotência;
- recuperação após reinicialização ou crash;
- deduplicação entre instâncias diferentes;
- garantia de execução exatamente uma vez;
- retry automático no driver.

A aplicação consumidora é responsável por fornecer e reutilizar o mesmo identificador nas tentativas da mesma operação lógica, preservando os parâmetros semânticos.

## 10. Evoluções arquiteturais

As evoluções abaixo exigem decisões de contrato e mudanças coordenadas entre componentes.

| Evolução | Componentes envolvidos | Objetivo |
|---|---|---|
| Persistência | Storage e recuperação | Preservar dados após reinicialização |
| TTL | Schema, modelo interno, Engine e Storage | Expirar entradas |
| Shutdown forçado | Server e Connection | Encerrar conexões após o prazo |
| Observabilidade | Server, Handler e driver | Diagnosticar falhas e acompanhar operação |
| Timeouts de I/O | Server, Handler e configuração | Limitar operações de rede bloqueadas |
| Multiplexação | Protocolo, Handler e driver | Permitir várias operações em andamento na mesma conexão |
| Segurança | Server, transporte e autenticação | Proteger comunicação e acesso |
| Escalabilidade | Storage, protocolo e coordenação | Suportar múltiplas instâncias e distribuição |