# Requisitos e Critérios de Aceite — MemoryDB

Este documento descreve retrospectivamente o comportamento observável na implementação atual. Requisitos planejados no roadmap não são tratados como implementados. Código e testes citados abaixo são a evidência; itens sem implementação são marcados explicitamente.

## Requisitos Funcionais

### FR-001 — Responder ao comando PING

- **Descrição:** Para uma solicitação PING válida, o servidor retorna sucesso com resultado PONG e preserva o `request_id`.
- **Componente:** `Engine`.
- **Evidência:** [`Engine.Execute`](../../internal/engine/engine.go#L21) trata `PingCommand` e produz `PongResult`; [`TestPing`](../../internal/engine/engine_test.go#L9).

**Critério de aceite**

```gherkin
Cenário: Verificar disponibilidade
  Dado que o engine recebeu uma solicitação PING com request_id 1
  Quando executar a solicitação
  Então deve retornar uma resposta de sucesso com PongResult
  E deve preservar request_id 1
```

### FR-002 — Gravar e substituir valores

- **Descrição:** SET com chave não vazia e TTL igual a zero grava bytes para a chave; gravar novamente a mesma chave substitui o valor anterior.
- **Componente:** `Engine` e `Storage`.
- **Evidência:** validação e chamada ao storage em [`Engine.Execute`](../../internal/engine/engine.go#L40); gravação/cópia em [`Storage.Set`](../../internal/storage/storage.go#L19); testes [`TestSetAndGet`](../../internal/engine/engine_test.go#L30) e [`TestSetOverwritesExistingValue`](../../internal/storage/storage_test.go#L35).

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

### FR-003 — Rejeitar TTL ainda não suportado

- **Descrição:** SET com `ttl_ms` maior que zero não grava a entrada e retorna `UNSUPPORTED_FEATURE`.
- **Componente:** `Engine`.
- **Evidência:** [`Engine.Execute`](../../internal/engine/engine.go#L47); [`TestRejectsUnsupportedTTL`](../../internal/engine/engine_test.go#L133). O campo existe no contrato em [`SetCommand`](../../api/proto/memorydb.proto#L37).

**Critério de aceite**

```gherkin
Cenário: Rejeitar SET com TTL positivo
  Dado que SET recebe uma chave não vazia e ttl_ms maior que zero
  Quando o engine executar a solicitação
  Então deve responder com success=false
  E o código do erro deve ser UNSUPPORTED_FEATURE
```

### FR-004 — Consultar chave existente ou ausente

- **Descrição:** GET retorna os bytes e `found=true` para chave existente; para chave inexistente, retorna `found=false`, sem confundir ausência com valor vazio.
- **Componente:** `Engine` e `Storage`.
- **Evidência:** [`Engine.Execute`](../../internal/engine/engine.go#L58); [`Storage.Get`](../../internal/storage/storage.go#L28); [`TestSetAndGet`](../../internal/engine/engine_test.go#L30), [`TestGetMissingKey`](../../internal/engine/engine_test.go#L66), [`TestEmptyValue`](../../internal/storage/storage_test.go#L66).

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
```

### FR-005 — Remover chave

- **Descrição:** DELETE remove a chave e informa `deleted=true`; se ela não existir, a operação continua sendo uma resposta de sucesso e informa `deleted=false`.
- **Componente:** `Engine` e `Storage`.
- **Evidência:** [`Engine.Execute`](../../internal/engine/engine.go#L77); [`Storage.Delete`](../../internal/storage/storage.go#L40); [`TestDelete`](../../internal/engine/engine_test.go#L85) e [`TestDelete`](../../internal/storage/storage_test.go#L52).

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

### FR-006 — Validar requisições e argumentos

- **Descrição:** Requisição nula, comando ausente/desconhecido ou comando tipado nulo é rejeitado como `INVALID_REQUEST`; chave vazia em SET, GET ou DELETE é rejeitada como `INVALID_ARGUMENT`.
- **Componente:** `Engine`.
- **Evidência:** [`Engine.Execute`](../../internal/engine/engine.go#L21), [`errorResponse`](../../internal/engine/command.go#L11); testes [`TestRejectsNilRequest`](../../internal/engine/engine_test.go#L158), [`TestRejectsMissingCommand`](../../internal/engine/engine_test.go#L177) e [`TestRejectsEmptyKey`](../../internal/engine/engine_test.go#L111). O teste de chave vazia cobre SET; a validação de GET e DELETE está no código, mas não possui teste específico.

**Critério de aceite**

```gherkin
Cenário: Rejeitar uma solicitação nula
  Dado que o engine recebeu uma solicitação nula
  Quando tentar executá-la
  Então deve responder com INVALID_REQUEST

Cenário: Rejeitar chave vazia em SET
  Dado que SET foi recebido sem chave
  Quando o engine executar a solicitação
  Então deve responder com INVALID_ARGUMENT
```

### FR-007 — Correlacionar respostas com solicitações

- **Descrição:** O engine copia o `request_id` para a resposta; o formato externo também define o campo nos envelopes de requisição e resposta.
- **Componente:** `Engine`, codec Protobuf e protocolo.
- **Evidência:** [`Engine.Execute`](../../internal/engine/engine.go#L26), [`Request` e `Response`](../../api/proto/memorydb.proto#L9); validação em [`TestPing`](../../internal/engine/engine_test.go#L9) e round-trip em [`TestRequestRoundTrip`](../../internal/codec/protobuf/codec_test.go#L10)/[`TestResponseRoundTrip`](../../internal/codec/protobuf/codec_test.go#L94).

**Critério de aceite**

```gherkin
Cenário: Preservar o identificador da solicitação
  Dado que uma solicitação válida tem request_id 42
  Quando for executada pelo engine
  Então a resposta deve conter request_id 42
```

### FR-008 — Delimitar mensagens TCP

- **Descrição:** Cada frame usa cabeçalho de 4 bytes com tamanho `uint32` Big Endian e payload entre 1 byte e 1 MiB. Leitura completa tolera fragmentação; escrita trata escritas parciais. Cabeçalho/payload truncados, tamanho zero e tamanho acima do limite retornam erro.
- **Componente:** transporte TCP.
- **Evidência:** [`ReadFrame` e `WriteFrame`](../../internal/transport/tcp/framing.go#L11); testes [`TestFrameHeaderUsesBigEndian`](../../internal/transport/tcp/framing_test.go#L30), [`TestReadFrameWithFragmentedInput`](../../internal/transport/tcp/framing_test.go#L47), [`TestReadFrameRejectsZeroSize`](../../internal/transport/tcp/framing_test.go#L71), [`TestReadFrameRejectsOversizedFrame`](../../internal/transport/tcp/framing_test.go#L85), [`TestReadFrameRejectsIncompleteHeader`](../../internal/transport/tcp/framing_test.go#L99), [`TestReadFrameRejectsIncompletePayload`](../../internal/transport/tcp/framing_test.go#L109) e [`TestWriteFrameHandlesPartialWrites`](../../internal/transport/tcp/framing_test.go#L132).

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
```

### FR-009 — Atender várias requisições e clientes

- **Descrição:** O handler processa várias requisições sequenciais na mesma conexão; o servidor aceita conexões de múltiplos clientes simultaneamente.
- **Componente:** `Handler` e `Server`.
- **Evidência:** [`Handler.Handle`](../../internal/transport/tcp/handler.go#L36), [`Server.Serve`](../../internal/transport/tcp/server.go#L43); testes [`TestHandlerProcessesMultipleRequests`](../../internal/transport/tcp/handler_test.go#L12) e [`TestServerAcceptsMultipleClients`](../../internal/transport/tcp/server_test.go#L38).

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
```

### FR-010 — Encerrar servidor graciosamente

- **Descrição:** `Shutdown` fecha o listener e aguarda conexões ativas até que terminem ou o contexto seja cancelado; o executável solicita shutdown ao receber SIGINT/SIGTERM, com timeout configurado de 10 segundos.
- **Componente:** `Server` e executável do servidor.
- **Evidência:** [`Server.Shutdown`](../../internal/transport/tcp/server.go#L94), [`main.run`](../../cmd/memorydb/main.go#L56); teste [`TestServerShutdownWaitsForActiveConnections`](../../internal/transport/tcp/server_test.go#L142).

**Critério de aceite**

```gherkin
Cenário: Aguardar conexão ativa durante shutdown
  Dado que o servidor tem uma conexão ativa
  Quando Shutdown for chamado
  Então o listener deve parar de aceitar conexões
  E Shutdown deve aguardar a conexão terminar ou o contexto expirar
```

## Requisitos Não Funcionais

| ID | Requisito / observação verificável | Estado | Evidência |
|---|---|---|---|
| NFR-001 | Operações concorrentes de armazenamento são protegidas por `sync.RWMutex`; há teste de acesso concorrente. | Implementado | [`Storage`](../../internal/storage/storage.go#L8), [`TestConcurrentAccess`](../../internal/storage/storage_test.go#L112) |
| NFR-002 | Valores são tratados como bytes, incluindo dados binários, no modelo e no contrato Protobuf. | Implementado | [`SetCommand` / `GetResult`](../../api/proto/memorydb.proto#L37), [`TestRequestRoundTrip`](../../internal/codec/protobuf/codec_test.go#L10) |
| NFR-003 | Frames de entrada e saída são limitados a 1 MiB; framing usa Big Endian. | Implementado | [`MaxFrameSize`](../../internal/transport/tcp/framing.go#L11), [`TestFrameHeaderUsesBigEndian`](../../internal/transport/tcp/framing_test.go#L30) |
| NFR-004 | Servidor pode fechar o listener e aguardar conexões ativas; o executável limita o shutdown a 10 segundos. Conexões ativas não são fechadas diretamente pelo servidor. | Parcial | [`Server.Shutdown`](../../internal/transport/tcp/server.go#L94), [`shutdownTimeout`](../../cmd/memorydb/main.go#L19) |
| NFR-005 | Dados sobrevivem a reinicialização do processo. | [NÃO IMPLEMENTADO] O armazenamento é em memória e não há persistência. | [`Storage`](../../internal/storage/storage.go#L8); [roadmap](../../memorydb-roadmap.md) |
| NFR-006 | Comunicação autenticada e criptografada por TLS. | [NÃO IMPLEMENTADO] Não há autenticação nem TLS no servidor TCP. | [`Server`](../../internal/transport/tcp/server.go#L17); [roadmap](../../memorydb-roadmap.md) |
| NFR-007 | Timeout de leitura/escrita, limite de conexões concorrentes e proteção contra clientes lentos. | [NÃO IMPLEMENTADO] Não identifiquei tais limites no código; o roadmap os registra como pendências. | [`Connection`](../../internal/transport/tcp/connection.go#L16), [`Server.Serve`](../../internal/transport/tcp/server.go#L43), [roadmap](../../memorydb-roadmap.md) |
| NFR-008 | Meta de latência/throughput mensurável para operações. | [NÃO IMPLEMENTADO] Não há meta ou benchmark de performance no código analisado. | [roadmap](../../memorydb-roadmap.md) |
| NFR-009 | Erros de processamento de conexão devem ser observáveis em log. | [NÃO IMPLEMENTADO] O erro de `Handler.Handle` é descartado pela goroutine do servidor. | [`Server.Serve`](../../internal/transport/tcp/server.go#L88) |

## Casos de borda implementados

| Caso | Comportamento atual | Evidência e teste |
|---|---|---|
| Requisição nula ou comando ausente/desconhecido | Retorna `INVALID_REQUEST` no engine. | [`Engine.Execute`](../../internal/engine/engine.go#L21); [`TestRejectsNilRequest`](../../internal/engine/engine_test.go#L158), [`TestRejectsMissingCommand`](../../internal/engine/engine_test.go#L177) |
| Chave vazia | SET/GET/DELETE retornam `INVALID_ARGUMENT`. | [`Engine.Execute`](../../internal/engine/engine.go#L44); [`TestRejectsEmptyKey`](../../internal/engine/engine_test.go#L111) cobre SET |
| TTL maior que zero | SET retorna `UNSUPPORTED_FEATURE`; não há expiração. | [`Engine.Execute`](../../internal/engine/engine.go#L47); [`TestRejectsUnsupportedTTL`](../../internal/engine/engine_test.go#L133) |
| Valor vazio | Pode ser armazenado; GET ainda informa que a chave foi encontrada. | [`Storage`](../../internal/storage/storage.go#L19); [`TestEmptyValue`](../../internal/storage/storage_test.go#L66) |
| Chave inexistente | GET retorna `found=false`; DELETE retorna `deleted=false`. | [`Engine.Execute`](../../internal/engine/engine.go#L67); [`TestGetMissingKey`](../../internal/engine/engine_test.go#L66), [`TestDelete`](../../internal/storage/storage_test.go#L52) |
| Mutação de slice pelo chamador | SET e GET copiam os bytes para proteger o conteúdo armazenado. | [`Storage`](../../internal/storage/storage.go#L19); [`TestSetCopiesInputValue`](../../internal/storage/storage_test.go#L82), [`TestGetReturnsCopy`](../../internal/storage/storage_test.go#L97) |
| Frame inválido/truncado | Frame zero, acima de 1 MiB ou incompleto retorna erro. | [`ReadFrame`](../../internal/transport/tcp/framing.go#L27); testes em [`framing_test.go`](../../internal/transport/tcp/framing_test.go#L71) |
| Protobuf inválido ou comando/result ausente no codec | Decode/encode retorna erro Go; no handler, erro de leitura encerra a conexão sem resposta estruturada de aplicação. | [`protobuf.Codec`](../../internal/codec/protobuf/codec.go#L49), [`Handler.Handle`](../../internal/transport/tcp/handler.go#L36); [`TestDecodeRejectsInvalidProtobuf`](../../internal/codec/protobuf/codec_test.go#L219) |
| Cliente desconecta por EOF | Handler encerra sem tratar EOF como erro de protocolo para o chamador. | [`Handler.Handle`](../../internal/transport/tcp/handler.go#L50) |
| Concorrência no storage | Operações usam lock de leitura/escrita. | [`Storage`](../../internal/storage/storage.go#L8); [`TestConcurrentAccess`](../../internal/storage/storage_test.go#L112) |

## Rastreabilidade requisito–componente–teste

| Requisito | Componente principal | Teste existente | Cobertura |
|---|---|---|---|
| FR-001 | Engine | [`TestPing`](../../internal/engine/engine_test.go#L9) | Coberto |
| FR-002 | Engine / Storage | [`TestSetAndGet`](../../internal/engine/engine_test.go#L30), [`TestSetOverwritesExistingValue`](../../internal/storage/storage_test.go#L35) | Coberto em unidade; substituição validada diretamente no storage |
| FR-003 | Engine | [`TestRejectsUnsupportedTTL`](../../internal/engine/engine_test.go#L133) | Coberto |
| FR-004 | Engine / Storage | [`TestGetMissingKey`](../../internal/engine/engine_test.go#L66), [`TestEmptyValue`](../../internal/storage/storage_test.go#L66) | Coberto |
| FR-005 | Engine / Storage | [`TestDelete`](../../internal/engine/engine_test.go#L85), [`TestDelete`](../../internal/storage/storage_test.go#L52) | Parcial: engine testa exclusão existente; storage verifica segunda exclusão |
| FR-006 | Engine | [`TestRejectsEmptyKey`](../../internal/engine/engine_test.go#L111), [`TestRejectsNilRequest`](../../internal/engine/engine_test.go#L158), [`TestRejectsMissingCommand`](../../internal/engine/engine_test.go#L177) | Parcial: chave vazia testada para SET, não separadamente para GET/DELETE |
| FR-007 | Engine / Codec | [`TestPing`](../../internal/engine/engine_test.go#L9), [`TestRequestRoundTrip`](../../internal/codec/protobuf/codec_test.go#L10), [`TestResponseRoundTrip`](../../internal/codec/protobuf/codec_test.go#L94) | Coberto |
| FR-008 | Framing TCP | [`framing_test.go`](../../internal/transport/tcp/framing_test.go#L11) | Coberto para endian, fragmentação, limites, truncamento e escrita parcial |
| FR-009 | Handler / Server | [`TestHandlerProcessesMultipleRequests`](../../internal/transport/tcp/handler_test.go#L12), [`TestServerAcceptsMultipleClients`](../../internal/transport/tcp/server_test.go#L38) | Coberto |
| FR-010 | Server | [`TestServerShutdownWaitsForActiveConnections`](../../internal/transport/tcp/server_test.go#L142) | Parcial: shutdown do server coberto; integração do tratamento de sinal do executável sem teste |

## Lacunas de especificação / comportamento

- **[GAP: TTL funcional não implementado]** O contrato declara `ttl_ms`, mas qualquer valor positivo é rejeitado. Não existe comportamento de expiração implementado. Evidência: [`SetCommand`](../../api/proto/memorydb.proto#L37), [`Engine.Execute`](../../internal/engine/engine.go#L47).
- **[GAP: autenticação e autorização não implementadas]** O servidor não identifica nem autoriza clientes; não há isolamento por tenant. Evidência: [`Server`](../../internal/transport/tcp/server.go#L17); itens futuros em [roadmap](../../memorydb-roadmap.md).
- **[GAP: TLS não implementado]** O listener usa TCP sem camada TLS. Evidência: [`main.run`](../../cmd/memorydb/main.go#L37).
- **[GAP: persistência não implementada]** Reiniciar o processo perde os dados. Evidência: [`Storage`](../../internal/storage/storage.go#L8).
- **[GAP: limites operacionais incompletos]** Não há timeout de leitura/escrita ou cota de conexões; somente o tamanho de frame é limitado. Evidências: [`Connection`](../../internal/transport/tcp/connection.go#L16), [`MaxFrameSize`](../../internal/transport/tcp/framing.go#L11).
