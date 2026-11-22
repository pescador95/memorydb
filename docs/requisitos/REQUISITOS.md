# Requisitos e Critérios de Aceite — MemoryDB

Este documento descreve retrospectivamente os requisitos funcionais e não funcionais observáveis na implementação atual do MemoryDB.

Os requisitos são classificados com base no código e nos testes existentes. Funcionalidades planejadas no roadmap, mas ainda não implementadas, são identificadas explicitamente como pendências.

## 1. Escopo da implementação atual

O MemoryDB é um banco de dados chave-valor em memória, implementado inicialmente em Go, com comunicação TCP e serialização Protocol Buffers.

A implementação atual contempla:

- Armazenamento de chaves do tipo `string` e valores do tipo `[]byte`.
- Operações PING, SET, GET e DELETE.
- Protocolo de comunicação baseado em Protocol Buffers.
- Delimitação de mensagens TCP por frames.
- Conexões TCP persistentes.
- Atendimento concorrente de múltiplos clientes.
- Driver cliente em Go.
- Operações context-aware no driver Go, com suporte a cancelamento e deadlines.
- Sinalização de resultados indeterminados por meio de `OperationOutcomeUnknownError`.
- Encerramento gracioso do servidor.
- Workspace Go com módulos independentes.

**Limitações conhecidas:**

- Não há persistência em disco.
- TTL está presente no contrato, mas ainda não é funcional.
- Não há autenticação, autorização ou isolamento por tenant.
- Não há TLS.
- Não há multiplexação de requisições na mesma conexão.
- Não há idempotência ou deduplicação de operações no servidor.
- Não há retry automático nem recuperação de resultados de operações cujo desfecho seja indeterminado.
- Não há driver Java implementado e validado nesta etapa.

## 2. Requisitos funcionais

### FR-001 — Responder ao comando PING

**Descrição:** Para uma solicitação PING válida, o servidor retorna sucesso com resultado PONG e preserva o `request_id`.

**Componente:** `Engine`.

**Evidência:**
- [Engine.Execute](../../server/internal/engine/engine.go) trata `PingCommand` e produz `PongResult`.
- [TestPing](../../server/internal/engine/engine_test.go).

**Critério de aceite**

```gherkin
Cenário: Verificar disponibilidade
  Dado que o engine recebeu uma solicitação PING com request_id 1
  Quando executar a solicitação
  Então deve retornar uma resposta de sucesso com PongResult
  E deve preservar request_id 1
```

**Estado:** Implementado e testado.

### FR-002 — Gravar e substituir valores

**Descrição:** SET com chave não vazia e TTL igual a zero grava bytes associados à chave. Uma nova gravação para a mesma chave substitui o valor anterior.

**Componentes:** `Engine` e `Storage`.

**Evidência:**
- [Engine.Execute](../../server/internal/engine/engine.go) valida os argumentos e executa a gravação.
- [Storage.Set](../../server/internal/storage/storage.go) realiza o armazenamento e copia os bytes recebidos.
- [TestSetAndGet](../../server/internal/engine/engine_test.go).
- [TestSetOverwritesExistingValue](../../server/internal/storage/storage_test.go).

**Critério de aceite**

```gherkin
Cenário: Gravar um valor
  Dado que a chave "name" não está vazia
  E que ttl_ms é zero
  Quando SET for executado com o valor "memorydb"
  Então o engine deve responder com sucesso
  E um GET posterior deve encontrar os mesmos bytes

Cenário: Substituir valor existente
  Dado que a chave "name" contém "old"
  Quando SET gravar "new" para a mesma chave
  Então GET deve retornar "new"
```

**Estado:** Implementado e testado.

### FR-003 — Rejeitar TTL ainda não suportado

**Descrição:** SET com `ttl_ms` maior que zero não grava a entrada e retorna `UNSUPPORTED_FEATURE`.

**Componentes:** Contrato Protobuf e `Engine`.

**Evidência:**
- [SetCommand](../../protocol/proto/memorydb/v1/memorydb.proto) declara o campo `ttl_ms`.
- [Engine.Execute](../../server/internal/engine/engine.go) rejeita valores positivos.
- [TestRejectsUnsupportedTTL](../../server/internal/engine/engine_test.go).

**Critério de aceite**

```gherkin
Cenário: Rejeitar SET com TTL positivo
  Dado que SET recebe uma chave não vazia e ttl_ms maior que zero
  Quando o engine executar a solicitação
  Então deve responder com success=false
  E o código do erro deve ser UNSUPPORTED_FEATURE
  E a entrada não deve ser gravada
```

**Estado:** Implementado e testado como rejeição. A funcionalidade de expiração continua não implementada.

### FR-004 — Consultar chave existente ou ausente

**Descrição:** GET retorna os bytes e `found=true` para uma chave existente. Para uma chave inexistente, retorna `found=false`, sem confundir ausência com valor vazio.

**Componentes:** `Engine` e `Storage`.

**Evidência:**
- [Engine.Execute](../../server/internal/engine/engine.go).
- [Storage.Get](../../server/internal/storage/storage.go).
- [TestSetAndGet](../../server/internal/engine/engine_test.go).
- [TestGetMissingKey](../../server/internal/engine/engine_test.go).
- [TestEmptyValue](../../server/internal/storage/storage_test.go).
- [TestServerEmptyValueAndMissingKey](../../server/internal/integration/server_test.go).

**Critério de aceite**

```gherkin
Cenário: Consultar chave existente
  Dado que a chave "name" contém os bytes de "memorydb"
  Quando GET consultar "name"
  Então a resposta deve indicar found=true
  E deve conter os bytes armazenados

Cenário: Consultar chave inexistente
  Dado que a chave "missing" não foi gravada
  Quando GET consultar "missing"
  Então a resposta deve indicar found=false

Cenário: Consultar valor vazio
  Dado que a chave "empty" foi gravada com valor vazio
  Quando GET consultar "empty"
  Então a resposta deve indicar found=true
  E o valor retornado deve ser vazio
```

**Estado:** Implementado e testado.

### FR-005 — Remover chave

**Descrição:** DELETE remove a chave e informa `deleted=true`. Se ela não existir, a operação continua sendo uma resposta de sucesso e informa `deleted=false`.

**Componentes:** `Engine` e `Storage`.

**Evidência:**
- [Engine.Execute](../../server/internal/engine/engine.go).
- [Storage.Delete](../../server/internal/storage/storage.go).
- [TestDelete](../../server/internal/engine/engine_test.go).
- [TestDelete](../../server/internal/storage/storage_test.go).
- [TestServerSetGetDelete](../../server/internal/integration/server_test.go).

**Critério de aceite**

```gherkin
Cenário: Remover chave existente
  Dado que a chave "name" foi gravada
  Quando DELETE remover "name"
  Então a resposta deve indicar deleted=true
  E um GET posterior deve indicar found=false

Cenário: Remover chave ausente
  Dado que a chave "missing" não existe
  Quando DELETE remover "missing"
  Então a resposta deve ser bem-sucedida
  E deve indicar deleted=false
```

**Estado:** Implementado e testado.

### FR-006 — Validar requisições e argumentos

**Descrição:** Requisição nula, comando ausente ou comando tipado nulo é rejeitado como `INVALID_REQUEST`. Chave vazia em SET, GET ou DELETE é rejeitada como `INVALID_ARGUMENT`.

**Componente:** `Engine`.

**Evidência:**
- [Engine.Execute](../../server/internal/engine/engine.go).
- [errorResponse](../../server/internal/engine/command.go).
- [TestRejectsNilRequest](../../server/internal/engine/engine_test.go).
- [TestRejectsMissingCommand](../../server/internal/engine/engine_test.go).
- [TestRejectsEmptyKey](../../server/internal/engine/engine_test.go).

**Critério de aceite**

```gherkin
Cenário: Rejeitar uma solicitação nula
  Dado que o engine recebeu uma solicitação nula
  Quando tentar executá-la
  Então deve responder com INVALID_REQUEST

Cenário: Rejeitar comando ausente
  Dado que o engine recebeu uma solicitação sem comando
  Quando tentar executá-la
  Então deve responder com INVALID_REQUEST

Cenário: Rejeitar chave vazia
  Dado que SET, GET ou DELETE foi recebido sem chave
  Quando o engine executar a solicitação
  Então deve responder com INVALID_ARGUMENT
```

**Estado:** Implementado. A cobertura de testes para chave vazia é parcial: o teste identificado cobre SET, enquanto GET e DELETE possuem validação no código, mas não possuem testes específicos identificados.

### FR-007 — Correlacionar respostas com solicitações

**Descrição:** O engine copia o `request_id` da solicitação para a resposta. O contrato externo também define esse campo nos envelopes de requisição e resposta.

O identificador permite correlacionar uma resposta com sua solicitação correspondente. Não representa, por si só, uma chave de idempotência e não impede que uma operação seja executada mais de uma vez.

**Componentes:** `Engine`, protocolo interno e codec Protobuf.

**Evidência:**
- [Request e Response](../../protocol/proto/memorydb/v1/memorydb.proto).
- [Engine.Execute](../../server/internal/engine/engine.go).
- [TestPing](../../server/internal/engine/engine_test.go).
- [TestRequestRoundTrip](../../server/internal/codec/protobuf/codec_test.go).
- [TestResponseRoundTrip](../../server/internal/codec/protobuf/codec_test.go).

**Critério de aceite**

```gherkin
Cenário: Preservar o identificador da solicitação
  Dado que uma solicitação válida tem request_id 42
  Quando for executada pelo engine
  Então a resposta deve conter request_id 42

Cenário: Preservar identificador na serialização
  Dado que uma requisição possui request_id 42
  Quando for serializada e desserializada
  Então o identificador deve permanecer 42
```

**Estado:** Implementado e testado.

### FR-008 — Delimitar mensagens TCP

**Descrição:** Cada frame utiliza um cabeçalho de 4 bytes com tamanho `uint32` em Big Endian. O payload deve possuir entre 1 byte e 1 MiB.

A leitura deve reconstruir mensagens mesmo quando os bytes chegam fragmentados. A escrita deve tratar escritas parciais.

Cabeçalhos ou payloads truncados, tamanho zero e tamanho acima do limite devem retornar erro.

**Componente:** Módulo de transporte TCP.

**Evidência:**
- [Framing TCP](../../transport/tcp/go/framing.go).
- [TestFrameHeaderUsesBigEndian](../../transport/tcp/go/framing_test.go).
- [TestReadFrameWithFragmentedInput](../../transport/tcp/go/framing_test.go).
- [TestReadFrameRejectsZeroSize](../../transport/tcp/go/framing_test.go).
- [TestReadFrameRejectsOversizedFrame](../../transport/tcp/go/framing_test.go).
- [TestReadFrameRejectsIncompleteHeader](../../transport/tcp/go/framing_test.go).
- [TestReadFrameRejectsIncompletePayload](../../transport/tcp/go/framing_test.go).
- [TestWriteFrameHandlesPartialWrites](../../transport/tcp/go/framing_test.go).

**Critério de aceite**

```gherkin
Cenário: Ler frame fragmentado
  Dado que o cabeçalho ou payload chega em partes
  Quando ReadFrame consumir os bytes
  Então deve reconstruir o payload completo

Cenário: Rejeitar frame acima do limite
  Dado que o cabeçalho declara tamanho superior a 1 MiB
  Quando ReadFrame validar o tamanho
  Então deve retornar erro sem aceitar o payload

Cenário: Tratar escrita parcial
  Dado que a conexão realiza escritas parciais
  Quando WriteFrame transmitir o frame
  Então todos os bytes devem ser escritos corretamente
```

**Estado:** Implementado e testado.

### FR-009 — Atender várias requisições e clientes

**Descrição:** O handler processa várias requisições sequenciais em uma mesma conexão TCP. O servidor aceita conexões de múltiplos clientes simultaneamente, compartilhando a mesma instância de armazenamento.

O driver Go serializa o ciclo de requisição e resposta por conexão, garantindo a correspondência entre cada solicitação e sua resposta.

**Componentes:** `Handler`, `Server`, transporte TCP e driver Go.

**Evidência:**
- [Handler.Handle](../../server/internal/transport/tcp/handler.go).
- [Server.Serve](../../server/internal/transport/tcp/server.go).
- [Connection](../../transport/tcp/go/connection.go).
- [Driver Go](../../drivers/go/client.go).
- [TestHandlerProcessesMultipleRequests](../../server/internal/transport/tcp/handler_test.go).
- [TestServerAcceptsMultipleClients](../../server/internal/transport/tcp/server_test.go).
- [TestServerMultipleClientsShareStorage](../../server/internal/integration/server_test.go).
- [TestServerConcurrentClients](../../server/internal/integration/server_test.go).

**Critério de aceite**

```gherkin
Cenário: Enviar mais de uma solicitação pela conexão
  Dado que uma conexão TCP permanece aberta
  Quando enviar PING e depois SET pela mesma conexão
  Então o handler deve produzir uma resposta para cada solicitação

Cenário: Compartilhar armazenamento entre clientes
  Dado que dois clientes estão conectados à mesma instância
  Quando o primeiro gravar uma chave
  Então o segundo deve poder consultá-la

Cenário: Atender clientes simultaneamente
  Dado que múltiplos clientes estão conectados
  Quando executarem operações simultaneamente
  Então o servidor deve processar as conexões sem interromper o atendimento
```

**Estado:** Implementado e testado.

**Limitação:** Não existe multiplexação de múltiplas requisições simultâneas na mesma conexão. O driver Go utiliza sincronização para executar um ciclo de requisição e resposta por vez.

### FR-010 — Encerrar servidor graciosamente

**Descrição:** `Shutdown` fecha o listener e aguarda as conexões ativas terminarem ou o contexto ser cancelado.

O executável solicita o encerramento ao receber SIGINT ou SIGTERM, utilizando timeout de 10 segundos.

**Componentes:** `Server` e executável do servidor.

**Evidência:**
- [Server.Shutdown](../../server/internal/transport/tcp/server.go).
- [main.go](../../server/cmd/memorydb/main.go).
- [TestServerShutdownWaitsForActiveConnections](../../server/internal/transport/tcp/server_test.go).
- [TestServerShutdownWaitsForActiveConnections — integração](../../server/internal/integration/server_test.go).

**Critério de aceite**

```gherkin
Cenário: Aguardar conexão ativa durante shutdown
  Dado que o servidor tem uma conexão ativa
  Quando Shutdown for chamado
  Então o listener deve parar de aceitar conexões
  E Shutdown deve aguardar a conexão terminar
  Ou retornar erro quando o contexto expirar

Cenário: Encerrar após sinal do sistema operacional
  Dado que o executável do servidor está em execução
  Quando receber SIGINT ou SIGTERM
  Então deve solicitar o encerramento gracioso
  E deve respeitar o timeout configurado
```

**Estado:** Implementado. O encerramento do servidor possui testes; o fluxo de sinais do executável não possui teste automatizado específico identificado.

**Limitação:** O servidor não força o fechamento das conexões ativas quando o contexto de shutdown expira. Nesse caso, `Shutdown` retorna o erro de contexto, mas as conexões permanecem sob responsabilidade de seus respectivos handlers.

### FR-011 — Controlar operações do driver por contexto

**Descrição:** O driver Go oferece métodos baseados em `context.Context` para controlar o prazo e o cancelamento de operações. Também oferece `DialContext` para controlar o estabelecimento inicial da conexão.

O comportamento do cancelamento depende da fase em que a operação se encontra:

- Se o contexto for cancelado enquanto a operação aguarda acesso ao mecanismo de serialização, a operação não iniciou o I/O. O driver retorna o erro do contexto e mantém a conexão utilizável.
- Se o contexto for cancelado durante o I/O, o driver interrompe a operação fechando a conexão. Como não é possível determinar com segurança se o servidor executou o comando antes da interrupção, o driver retorna `OperationOutcomeUnknownError`.
- Quando uma operação termina normalmente, o resultado recebido é retornado conforme o contrato do comando.
- O erro `OperationOutcomeUnknownError` preserva o identificador da solicitação e a causa original, permitindo inspeção com `errors.As` e `errors.Is`.

O estado do cliente é atualizado de acordo com o resultado da operação. Após uma interrupção durante I/O, a conexão não é reutilizada.

**Componente:** Driver Go.

**Evidência:**
- [Driver Go](../../drivers/go/client.go).
- [TestClientPartialWriteReturnsUnknownOutcome](../../drivers/go/client_test.go).
- [TestClientSetReturnsUnknownOutcomeAfterServerExecution](../../drivers/go/client_test.go).
- [TestClientCancellationDuringFinalResponseRead](../../drivers/go/client_test.go).

**Critério de aceite**

```gherkin
Cenário: Cancelar operação antes do início do I/O
  Dado que uma operação aguarda acesso à conexão
  E que seu contexto é cancelado antes do início do I/O
  Quando a operação tentar prosseguir
  Então deve retornar o erro de contexto
  E o cliente deve permanecer utilizável

Cenário: Cancelar operação durante o I/O
  Dado que uma operação já iniciou a comunicação com o servidor
  Quando seu contexto for cancelado
  Então o driver deve interromper a conexão
  E deve retornar OperationOutcomeUnknownError
  E deve preservar o RequestID e a causa do cancelamento
  E o cliente não deve reutilizar a conexão interrompida

Cenário: Preservar a causa original do erro
  Dado que uma operação retorna OperationOutcomeUnknownError
  Quando a aplicação inspecionar o erro
  Então deve conseguir identificar seu tipo com errors.As
  E deve conseguir verificar sua causa com errors.Is
```

**Estado:** Implementado e testado no driver Go.

**Limitação:** O controle por contexto não oferece garantia de que uma operação cancelada durante I/O não tenha sido executada no servidor. Também não existe mecanismo server-side para consultar ou recuperar o resultado dessa operação. O driver não realiza retry automático.

## 3. Requisitos não funcionais

| ID | Requisito / observação verificável | Estado | Evidência |
|---|---|---|---|
| NFR-001 | Operações concorrentes de armazenamento são protegidas por `sync.RWMutex`. | Implementado | [Storage](../../server/internal/storage/storage.go), [TestConcurrentAccess](../../server/internal/storage/storage_test.go) |
| NFR-002 | Valores são tratados como bytes, incluindo dados binários, no modelo interno e no contrato Protobuf. | Implementado | [memorydb.proto](../../protocol/proto/memorydb/v1/memorydb.proto), [TestRequestRoundTrip](../../server/internal/codec/protobuf/codec_test.go) |
| NFR-003 | Frames de entrada e saída são limitados a 1 MiB e utilizam Big Endian. | Implementado | [framing.go](../../transport/tcp/go/framing.go), [framing_test.go](../../transport/tcp/go/framing_test.go) |
| NFR-004 | O servidor pode fechar o listener e aguardar conexões ativas. O executável limita o shutdown a 10 segundos. | Parcial | [server.go](../../server/internal/transport/tcp/server.go), [main.go](../../server/cmd/memorydb/main.go) |
| NFR-005 | Dados sobrevivem à reinicialização do processo. | Não implementado | O armazenamento é exclusivamente em memória. [storage.go](../../server/internal/storage/storage.go) |
| NFR-006 | Comunicação autenticada e criptografada por TLS. | Não implementado | O servidor utiliza listener TCP sem TLS. [main.go](../../server/cmd/memorydb/main.go) |
| NFR-007 | Timeout de leitura/escrita no servidor, limite de conexões concorrentes e proteção contra clientes lentos. | Não implementado | Não há políticas operacionais específicas identificadas em [connection.go](../../transport/tcp/go/connection.go) e [server.go](../../server/internal/transport/tcp/server.go). |
| NFR-008 | Meta mensurável de latência e throughput para operações. | Não implementado | Não há metas ou benchmarks de performance identificados. |
| NFR-009 | Erros de processamento de conexão devem ser observáveis em log. | Não implementado | O erro retornado por `Handler.Handle` é descartado pela goroutine do servidor. [server.go](../../server/internal/transport/tcp/server.go) |
| NFR-010 | Separação entre armazenamento, execução de comandos, protocolo interno, serialização e transporte. | Implementado | [Engine](../../server/internal/engine), [Storage](../../server/internal/storage), [Protocol](../../server/internal/protocol), [Codec](../../server/internal/codec), [Transporte TCP](../../server/internal/transport/tcp) |
| NFR-011 | O transporte TCP deve ser reutilizável sem dependência direta do Protobuf. | Implementado | [Módulo TCP](../../transport/tcp/go), [Driver Go](../../drivers/go), [Handler](../../server/internal/transport/tcp/handler.go) |
| NFR-012 | O driver Go deve permitir que aplicações utilizem o MemoryDB sem implementar manualmente o protocolo de comunicação. | Implementado | [Driver Go](../../drivers/go/client.go), [Testes do driver](../../drivers/go/client_test.go) |
| NFR-013 | Os módulos Go devem permitir desenvolvimento e testes independentes dentro do workspace. | Implementado | [go.work](../../go.work), arquivos `go.mod` dos módulos |
| NFR-014 | O código deve passar pelos testes com detector de condições de corrida e pela análise estática `go vet`. | Validado nas execuções registradas | Validação dos módulos em 15/11/2026 e validações posteriores do driver Go, incluindo `go test -race -count=20 ./drivers/go` e `go vet ./drivers/go`. |

**Observações sobre NFR-007:** O suporte a `context.Context` no driver permite controlar operações do lado cliente, mas não equivale à implementação de políticas server-side de timeout de leitura, timeout de escrita, inatividade ou proteção contra clientes lentos.

**Observações sobre NFR-014:** A validação é uma evidência vinculada à versão do código e à data de execução. Não representa garantia permanente de que alterações futuras continuarão passando pelos mesmos testes e verificações.

## 4. Casos de borda implementados

| Caso | Comportamento atual | Evidência |
|---|---|---|
| Requisição nula ou comando ausente | Retorna `INVALID_REQUEST` no engine. | [engine.go](../../server/internal/engine/engine.go), [engine_test.go](../../server/internal/engine/engine_test.go) |
| Comando tipado nulo | Rejeitado como requisição inválida. | [engine.go](../../server/internal/engine/engine.go), [engine_test.go](../../server/internal/engine/engine_test.go) |
| Chave vazia | SET, GET e DELETE rejeitam a operação com `INVALID_ARGUMENT`. | [engine.go](../../server/internal/engine/engine.go), [TestRejectsEmptyKey](../../server/internal/engine/engine_test.go) |
| TTL maior que zero | SET retorna `UNSUPPORTED_FEATURE`; não há expiração. | [engine.go](../../server/internal/engine/engine.go), [TestRejectsUnsupportedTTL](../../server/internal/engine/engine_test.go) |
| Valor vazio | Pode ser armazenado; GET informa que a chave foi encontrada. | [storage.go](../../server/internal/storage/storage.go), [TestEmptyValue](../../server/internal/storage/storage_test.go) |
| Chave inexistente | GET retorna `found=false`; DELETE retorna `deleted=false`. | [engine.go](../../server/internal/engine/engine.go), [engine_test.go](../../server/internal/engine/engine_test.go) |
| Substituição de valor | SET substitui o valor anterior da mesma chave. | [TestSetOverwritesExistingValue](../../server/internal/storage/storage_test.go) |
| Mutação de slice pelo chamador | SET e GET copiam os bytes para proteger o conteúdo armazenado. | [storage.go](../../server/internal/storage/storage.go), [storage_test.go](../../server/internal/storage/storage_test.go) |
| Frame com tamanho zero | Rejeitado pelo framing. | [framing.go](../../transport/tcp/go/framing.go), [framing_test.go](../../transport/tcp/go/framing_test.go) |
| Frame acima de 1 MiB | Rejeitado pelo framing. | [framing.go](../../transport/tcp/go/framing.go), [framing_test.go](../../transport/tcp/go/framing_test.go) |
| Frame truncado | Cabeçalho ou payload incompleto retorna erro. | [framing_test.go](../../transport/tcp/go/framing_test.go) |
| Escrita parcial | O framing continua escrevendo até transmitir o frame completo ou encontrar erro. | [framing_test.go](../../transport/tcp/go/framing_test.go) |
| Protobuf inválido | Decode retorna erro Go. | [codec.go](../../server/internal/codec/protobuf/codec.go), [codec_test.go](../../server/internal/codec/protobuf/codec_test.go) |
| Comando ou resultado ausente no codec | Encode/Decode rejeita estruturas inválidas. | [codec.go](../../server/internal/codec/protobuf/codec.go), [codec_test.go](../../server/internal/codec/protobuf/codec_test.go) |
| Cliente desconecta por EOF | Handler encerra sem tratar EOF como erro de protocolo para o chamador. | [handler.go](../../server/internal/transport/tcp/handler.go), [TestServerClientDisconnectDoesNotStopServer](../../server/internal/integration/server_test.go) |
| Concorrência no storage | Operações são sincronizadas por lock de leitura/escrita. | [storage.go](../../server/internal/storage/storage.go), [TestConcurrentAccess](../../server/internal/storage/storage_test.go) |
| Múltiplos clientes | Conexões distintas podem utilizar a mesma instância do servidor e compartilhar o armazenamento. | [server_test.go](../../server/internal/transport/tcp/server_test.go), [server_test.go — integração](../../server/internal/integration/server_test.go) |
| Desconexão durante operação | Falhas de leitura e escrita são propagadas pelo driver. | [client_test.go](../../drivers/go/client_test.go) |
| Cancelamento antes do início do I/O | A operação retorna o erro de contexto sem invalidar a conexão. | [client_test.go](../../drivers/go/client_test.go) |
| Cancelamento durante o I/O | A conexão é interrompida e o driver sinaliza resultado indeterminado. | [client.go](../../drivers/go/client.go), [client_test.go](../../drivers/go/client_test.go) |
| Escrita parcial seguida de falha | O driver não presume que a operação deixou de ser executada; retorna resultado indeterminado. | [client_test.go](../../drivers/go/client_test.go) |
| Servidor executa SET, mas a resposta não chega ao cliente | O cliente não consegue determinar se o comando foi aplicado e retorna resultado indeterminado quando detecta a falha. | [TestClientSetReturnsUnknownOutcomeAfterServerExecution](../../drivers/go/client_test.go) |
| Identificador da operação | `RequestID` identifica a solicitação e permite correlacionar a resposta; não realiza deduplicação. | [client.go](../../drivers/go/client.go), [memorydb.proto](../../protocol/proto/memorydb/v1/memorydb.proto) |

## 5. Rastreabilidade requisito–componente–teste

| Requisito | Componente principal | Testes existentes | Cobertura |
|---|---|---|---|
| FR-001 | Engine | `TestPing` | Coberto |
| FR-002 | Engine / Storage | `TestSetAndGet`, `TestSetOverwritesExistingValue` | Coberto |
| FR-003 | Engine | `TestRejectsUnsupportedTTL` | Coberto para rejeição |
| FR-004 | Engine / Storage | `TestGetMissingKey`, `TestEmptyValue`, `TestServerEmptyValueAndMissingKey` | Coberto |
| FR-005 | Engine / Storage | `TestDelete`, `TestServerSetGetDelete` | Coberto |
| FR-006 | Engine | `TestRejectsEmptyKey`, `TestRejectsNilRequest`, `TestRejectsMissingCommand` | Parcial: faltam testes específicos de chave vazia em GET e DELETE |
| FR-007 | Engine / Codec | `TestPing`, `TestRequestRoundTrip`, `TestResponseRoundTrip` | Coberto |
| FR-008 | Framing TCP | Testes de endian, fragmentação, limites, truncamento e escrita parcial | Coberto |
| FR-009 | Handler / Server / Driver | `TestHandlerProcessesMultipleRequests`, `TestServerAcceptsMultipleClients`, `TestServerMultipleClientsShareStorage`, `TestServerConcurrentClients` | Coberto para conexões distintas e requisições sequenciais |
| FR-010 | Server / Executável | `TestServerShutdownWaitsForActiveConnections` | Parcial: fluxo de sinais do executável sem teste específico |
| FR-011 | Driver Go | `TestClientPartialWriteReturnsUnknownOutcome`, `TestClientSetReturnsUnknownOutcomeAfterServerExecution`, `TestClientCancellationDuringFinalResponseRead` | Coberto para os cenários identificados |

## 6. Lacunas de especificação e comportamento

### GAP-001 — TTL funcional não implementado

O contrato Protobuf declara `ttl_ms`, mas qualquer valor positivo é rejeitado pelo engine. Não existe expiração automática de entradas.

**Impacto:** as aplicações não podem solicitar expiração temporária de chaves.

**Referências:** [memorydb.proto](../../protocol/proto/memorydb/v1/memorydb.proto), [engine.go](../../server/internal/engine/engine.go).

### GAP-002 — Persistência não implementada

O armazenamento é mantido exclusivamente em memória. A reinicialização do processo elimina os dados.

**Impacto:** não há durabilidade nem recuperação de dados após falhas ou reinicializações.

**Referência:** [storage.go](../../server/internal/storage/storage.go).

### GAP-003 — Autenticação e autorização não implementadas

O servidor não identifica clientes nem valida permissões para execução de comandos.

**Impacto:** qualquer cliente que consiga estabelecer conexão TCP pode tentar executar operações.

### GAP-004 — TLS não implementado

A comunicação utiliza TCP sem criptografia.

**Impacto:** os dados trafegam sem proteção criptográfica no transporte.

**Referência:** [main.go](../../server/cmd/memorydb/main.go).

### GAP-005 — Isolamento por tenant não implementado

O armazenamento atual é compartilhado entre todos os clientes conectados à mesma instância.

**Impacto:** não existe separação lógica de dados por organização ou cliente.

### GAP-006 — Limites operacionais incompletos

O tamanho dos frames é limitado a 1 MiB, mas não foram identificados limites específicos para:

- Quantidade de conexões simultâneas.
- Tempo máximo de leitura no servidor.
- Tempo máximo de escrita no servidor.
- Tempo máximo de inatividade de uma conexão.
- Proteção contra clientes que enviam dados lentamente.

**Impacto:** conexões lentas ou excessivas podem consumir recursos do servidor.

**Referências:** [connection.go](../../transport/tcp/go/connection.go), [server.go](../../server/internal/transport/tcp/server.go), [framing.go](../../transport/tcp/go/framing.go).

**Observação:** os métodos context-aware do driver permitem ao cliente controlar o prazo de suas operações, mas não substituem limites e timeouts operacionais aplicados pelo servidor.

### GAP-007 — Observabilidade de erros de conexão incompleta

O servidor executa `Handler.Handle` em goroutines, mas o erro retornado pelo handler é descartado.

**Impacto:** falhas de processamento de conexões não são registradas de maneira estruturada pelo servidor.

**Referência:** [server.go](../../server/internal/transport/tcp/server.go).

### GAP-008 — Multiplexação de requisições não implementada

O driver Go utiliza sincronização para garantir que uma solicitação seja enviada e sua resposta recebida antes de iniciar a próxima operação na mesma conexão.

**Impacto:** uma conexão não permite múltiplas requisições simultâneas em trânsito, embora o servidor atenda várias conexões simultaneamente.

**Referência:** [client.go](../../drivers/go/client.go).

### GAP-009 — Resultado indeterminado não possui recuperação server-side

O driver Go implementa operações baseadas em `context.Context` e identifica situações nas quais o resultado de uma operação não pode ser confirmado pelo cliente.

Quando o cancelamento acontece antes do início do I/O, o driver retorna o erro de contexto sem invalidar a conexão.

Quando o cancelamento ou uma falha de comunicação acontece durante o I/O, o servidor pode já ter executado o comando, mesmo que o cliente não tenha recebido a resposta. Nesse cenário, o driver interrompe a conexão e retorna `OperationOutcomeUnknownError`, preservando o `RequestID` e a causa original.

Entretanto, o servidor ainda não oferece:

- Identificador de operação estável para deduplicação.
- Registro de operações executadas e seus resultados.
- Consulta posterior do resultado de uma operação.
- Reenvio seguro de uma operação com garantia de não duplicação.
- Recuperação de resultados após desconexão.

**Impacto:** aplicações precisam tratar explicitamente resultados indeterminados. Repetir automaticamente uma operação após esse erro pode executar o comando novamente. O driver não realiza retry automático.

**Referência:** [client.go](../../drivers/go/client.go).

### GAP-010 — Driver Java não validado nesta etapa

A arquitetura prevê drivers independentes do servidor, mas a implementação e a validação do driver Java não fazem parte do estado funcional documentado aqui.

**Impacto:** a integração por Java ainda não deve ser considerada um requisito implementado.

### GAP-011 — Idempotência e deduplicação não implementadas

O protocolo possui `request_id`, utilizado para correlacionar requisições e respostas. Entretanto, esse identificador não constitui uma chave de idempotência.

O servidor não mantém um registro de operações já executadas, não identifica reenvios de uma mesma operação lógica e não reproduz resultados anteriores.

**Impacto:** em caso de perda de resposta ou resultado indeterminado, o cliente não consegue reenviar uma operação com garantia de que ela será executada no máximo uma vez.

A implementação futura deverá definir, entre outros aspectos:

- Identificador estável da operação, independente do identificador de transporte.
- Escopo e período de retenção dos identificadores.
- Armazenamento do resultado associado à operação.
- Comportamento para identificadores repetidos com conteúdo divergente.
- Política de concorrência para requisições com o mesmo identificador.
- Relação entre deduplicação, persistência e recuperação após reinicialização.

**Referência:** [memorydb.proto](../../protocol/proto/memorydb/v1/memorydb.proto), [client.go](../../drivers/go/client.go).

## 7. Validação técnica da implementação

Na validação registrada em 15/11/2026, foram executados os testes e a análise estática nos seguintes módulos:

| Módulo | `go test -race -count=1 ./...` | `go vet ./...` |
|---|---|---|
| `protocol/generated/go` | Aprovado — sem testes próprios | Aprovado |
| `transport/tcp/go` | Aprovado | Aprovado |
| `drivers/go` | Aprovado | Aprovado |
| `server` | Aprovado | Aprovado |
| `api` | Aprovado — sem testes próprios | Aprovado |

Após essa validação, o driver Go recebeu testes adicionais relacionados a resultados indeterminados, cancelamento durante I/O e falhas de comunicação.

Na validação posterior do driver, foram executados com sucesso:

```bash
go test -v -count=1 ./drivers/go
go test -race -count=1 ./drivers/go
go test -race -count=20 ./drivers/go
go vet ./drivers/go
```

Os testes adicionais contemplaram, entre outros cenários:

- Falha durante escrita parcial.
- Servidor executando SET antes de a resposta ser perdida.
- Cancelamento durante a leitura da resposta final.
- Preservação do identificador e da causa do resultado indeterminado.

**Observação:** a validação de 15/11 comprova o estado dos módulos naquela execução. A validação posterior comprova os cenários adicionais do driver Go, mas não substitui uma nova execução completa de todos os módulos após as alterações. Alterações futuras no código também exigem nova execução para manter essas evidências atualizadas.

## 8. Critério geral de conclusão do escopo atual

O escopo funcional inicial do MemoryDB é considerado implementado quando:

1. PING, SET, GET e DELETE funcionam conforme os contratos definidos.
2. O engine valida requisições e argumentos.
3. O armazenamento preserva a distinção entre chave ausente e valor vazio.
4. O protocolo Protobuf representa requisições e respostas.
5. O transporte TCP delimita mensagens corretamente.
6. O servidor atende múltiplos clientes e conexões persistentes.
7. O driver Go permite consumir as operações sem implementar manualmente o protocolo.
8. O driver Go oferece operações context-aware e sinaliza resultados indeterminados quando não é possível confirmar o desfecho de uma operação.
9. O servidor oferece encerramento gracioso.
10. Os testes automatizados e a análise estática passam em todos os módulos.

Os GAPs documentados neste arquivo não impedem a conclusão do escopo inicial, pois representam evoluções planejadas e limitações conhecidas da implementação atual.

**Este documento não substitui o roadmap:** o roadmap registra o que será desenvolvido; este documento registra o que existe, como pode ser verificado e quais limitações permanecem.