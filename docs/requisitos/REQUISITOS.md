# Requisitos e Critérios de Aceite — MemoryDB

## 1. Objetivo

Este documento registra os requisitos funcionais e as características não funcionais verificáveis na implementação atual do MemoryDB.

A documentação é retrospectiva: os requisitos são descritos a partir do comportamento implementado no código e dos testes identificados no repositório.

O documento também registra limitações e lacunas que devem ser consideradas em evoluções futuras.

Os estados utilizados são:

| Estado | Significado |
|---|---|
| Implementado e testado | Comportamento presente no código e coberto por testes identificados |
| Implementado, cobertura parcial | Comportamento presente no código, mas sem cobertura de teste suficiente para todos os cenários descritos |
| Implementado, validação histórica | Há registro de execução bem-sucedida, mas não necessariamente uma validação completa do estado atual |
| Parcial | Parte do comportamento está implementada, mas há limitações conhecidas |
| Não implementado | Não há implementação funcional identificada |

## 2. Escopo atual

MemoryDB é um banco de dados chave-valor em memória, implementado em Go, com comunicação TCP e serialização Protocol Buffers.

O escopo funcional atual contempla:

- Armazenamento de chaves do tipo `string`.
- Armazenamento de valores binários do tipo `[]byte`.
- Operações `PING`, `SET`, `GET` e `DELETE`.
- Protocolo de comunicação baseado em Protocol Buffers.
- Delimitação de mensagens TCP por frames.
- Conexões TCP persistentes.
- Atendimento concorrente de múltiplos clientes.
- Compartilhamento do armazenamento entre conexões da mesma instância.
- Driver cliente em Go.
- Operações context-aware no driver Go.
- Sinalização de resultados indeterminados por `OperationOutcomeUnknownError`.
- Encerramento gracioso do servidor.
- Workspace Go composto por módulos independentes.

### 2.1 Limitações do escopo

A implementação atual não oferece:

- Persistência em disco.
- Recuperação de dados após reinicialização.
- TTL funcional.
- Autenticação ou autorização.
- TLS.
- Isolamento por tenant.
- Replicação ou distribuição entre instâncias.
- Multiplexação de requisições na mesma conexão.
- Idempotência ou deduplicação server-side.
- Recuperação de resultados indeterminados.
- Retry automático no driver.
- Driver Java implementado e validado.

## 3. Requisitos funcionais

### FR-001 — Responder ao comando PING

**Descrição:** Para uma solicitação PING válida, o servidor deve retornar sucesso com resultado PONG e preservar o `request_id`.

**Componentes:** `Engine`, modelo interno e codec.

**Evidências:**
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/engine_test.go` — `TestPing`.

**Critérios de aceite**

```gherkin
Cenário: Verificar disponibilidade
  Dado que o engine recebeu uma solicitação PING com request_id 1
  Quando executar a solicitação
  Então deve retornar uma resposta de sucesso
  E o resultado deve ser PongResult
  E deve preservar request_id 1
```

**Estado:** Implementado e testado.

### FR-002 — Gravar e substituir valores

**Descrição:** SET com chave não vazia e `ttl_ms` igual a zero deve armazenar os bytes associados à chave.

Uma nova gravação para a mesma chave deve substituir o valor anterior.

**Componentes:** `Engine` e `Storage`.

**Evidências:**
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/storage/storage.go` — `Storage.Set`.
- `server/internal/engine/engine_test.go` — `TestSetAndGet`.
- `server/internal/storage/storage_test.go` — `TestSetOverwritesExistingValue`.

**Critérios de aceite**

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

**Descrição:** SET com `ttl_ms` maior que zero deve ser rejeitado com `UNSUPPORTED_FEATURE`, sem gravar a entrada.

O campo existe no contrato externo, mas a expiração automática de entradas não está implementada.

**Componentes:** Contrato Protobuf e `Engine`.

**Evidências:**
- `protocol/proto/memorydb/v1/memorydb.proto` — campo `ttl_ms`.
- `server/internal/engine/engine.go` — validação de `TTLMillis`.
- `server/internal/engine/engine_test.go` — `TestRejectsUnsupportedTTL`.

**Critérios de aceite**

```gherkin
Cenário: Rejeitar SET com TTL positivo
  Dado que SET recebe uma chave não vazia
  E que ttl_ms é maior que zero
  Quando o engine executar a solicitação
  Então deve responder com success=false
  E o código do erro deve ser UNSUPPORTED_FEATURE
  E a entrada não deve ser gravada
```

**Estado:** Implementado e testado quanto à rejeição.

**Limitação:** A funcionalidade de expiração continua não implementada.

### FR-004 — Consultar chave existente ou ausente

**Descrição:** GET deve retornar os bytes e `found=true` para uma chave existente.

Para uma chave inexistente, deve retornar `found=false`, sem confundir ausência com valor vazio.

**Componentes:** `Engine` e `Storage`.

**Evidências:**
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/storage/storage.go` — `Storage.Get`.
- `server/internal/engine/engine_test.go` — `TestSetAndGet` e `TestGetMissingKey`.
- `server/internal/storage/storage_test.go` — `TestEmptyValue`.
- `server/internal/integration/server_test.go` — `TestServerEmptyValueAndMissingKey`.

**Critérios de aceite**

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

**Descrição:** DELETE deve remover a chave e informar `deleted=true`.

Se a chave não existir, a operação deve continuar sendo uma resposta de sucesso e informar `deleted=false`.

**Componentes:** `Engine` e `Storage`.

**Evidências:**
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/storage/storage.go` — `Storage.Delete`.
- `server/internal/engine/engine_test.go` — `TestDelete`.
- `server/internal/storage/storage_test.go` — `TestDelete`.
- `server/internal/integration/server_test.go` — `TestServerSetGetDelete`.

**Critérios de aceite**

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

**Descrição:** O engine deve rejeitar:

- solicitação nula;
- comando ausente;
- comando desconhecido;
- comando representado por ponteiro tipado nulo;
- chave vazia em SET, GET ou DELETE.

Os erros devem ser representados por:

| Condição | Código |
|---|---|
| Solicitação ou comando inválido | `INVALID_REQUEST` |
| Chave vazia | `INVALID_ARGUMENT` |

**Componentes:** `Engine` e `errorResponse`.

**Evidências:**
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/command.go` — `errorResponse`.
- `server/internal/engine/engine_test.go` — `TestRejectsNilRequest`, `TestRejectsMissingCommand` e `TestRejectsEmptyKey`.

**Critérios de aceite**

```gherkin
Cenário: Rejeitar solicitação nula
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

**Estado:** Implementado, cobertura parcial.

**Lacuna de testes:** O teste identificado para chave vazia cobre SET. GET e DELETE possuem validação no código, mas não foram identificados testes específicos equivalentes para essas operações.

### FR-007 — Correlacionar respostas com solicitações

**Descrição:** O engine deve copiar o `request_id` da solicitação para a resposta.

O codec deve preservar o identificador durante a serialização e desserialização.

O identificador permite correlacionar uma resposta com sua solicitação correspondente. Não representa uma chave de idempotência e não impede a execução repetida de uma operação.

**Componentes:** `Engine`, modelo interno e codec Protobuf.

**Evidências:**
- `protocol/proto/memorydb/v1/memorydb.proto` — envelopes `Request` e `Response`.
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/engine_test.go` — `TestPing`.
- `server/internal/codec/protobuf/codec_test.go` — `TestRequestRoundTrip` e `TestResponseRoundTrip`.

**Critérios de aceite**

```gherkin
Cenário: Preservar identificador no engine
  Dado que uma solicitação válida tem request_id 42
  Quando for executada pelo engine
  Então a resposta deve conter request_id 42

Cenário: Preservar identificador na serialização
  Dado que uma requisição possui request_id 42
  Quando for serializada e desserializada
  Então o identificador deve permanecer 42
```

**Estado:** Implementado e testado.

**Limitação:** Não existe deduplicação baseada nesse identificador.

### FR-008 — Delimitar mensagens TCP

**Descrição:** Cada frame deve utilizar:

- cabeçalho de 4 bytes;
- tamanho `uint32`;
- ordem de bytes Big Endian;
- payload mínimo de 1 byte;
- payload máximo de 1 MiB.

A leitura deve reconstruir mensagens mesmo quando os bytes chegam fragmentados. A escrita deve tratar escritas parciais.

Cabeçalhos ou payloads truncados, tamanho zero e tamanho acima do limite devem retornar erro.

**Componente:** Módulo `transport/tcp/go`.

**Evidências:**
- `transport/tcp/go/framing.go` — `ReadFrame` e `WriteFrame`.
- `transport/tcp/go/framing_test.go` — testes de Big Endian, fragmentação, limites, truncamento e escrita parcial.

**Critérios de aceite**

```gherkin
Cenário: Ler frame fragmentado
  Dado que o cabeçalho ou payload chega em partes
  Quando ReadFrame consumir os bytes
  Então deve reconstruir o payload completo

Cenário: Rejeitar frame acima do limite
  Dado que o cabeçalho declara tamanho superior a 1 MiB
  Quando ReadFrame validar o tamanho
  Então deve retornar erro

Cenário: Tratar escrita parcial
  Dado que o writer realiza escritas parciais
  Quando WriteFrame transmitir o frame
  Então todos os bytes devem ser escritos corretamente
  Ou deve retornar o erro ocorrido
```

**Estado:** Implementado e testado.

### FR-009 — Atender várias requisições e clientes

**Descrição:** O handler deve processar várias requisições sequenciais em uma mesma conexão TCP.

O servidor deve aceitar conexões de múltiplos clientes simultaneamente. As conexões de uma mesma instância devem compartilhar o mesmo armazenamento.

O driver Go deve serializar o ciclo de requisição e resposta por conexão, garantindo a correspondência entre cada solicitação e sua resposta.

**Componentes:** `Handler`, `Server`, transporte TCP, `Engine`, `Storage` e driver Go.

**Evidências:**
- `server/internal/transport/tcp/handler.go` — `Handler.Handle`.
- `server/internal/transport/tcp/server.go` — `Server.Serve`.
- `transport/tcp/go/connection.go` — `Connection`.
- `drivers/go/client.go` — `Client.execute`.
- `server/internal/transport/tcp/handler_test.go` — `TestHandlerProcessesMultipleRequests`.
- `server/internal/transport/tcp/server_test.go` — `TestServerAcceptsMultipleClients`.
- `server/internal/integration/server_test.go` — testes de compartilhamento de storage e clientes concorrentes.

**Critérios de aceite**

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

**Limitação:** Não existe multiplexação de requisições simultâneas na mesma conexão. O driver Go executa um ciclo de requisição e resposta por vez.

### FR-010 — Encerrar servidor graciosamente

**Descrição:** `Shutdown` deve fechar o listener e aguardar as conexões ativas terminarem ou o contexto ser cancelado.

O executável deve solicitar encerramento ao receber SIGINT ou SIGTERM, utilizando timeout de 10 segundos.

**Componentes:** `Server` e executável do servidor.

**Evidências:**
- `server/internal/transport/tcp/server.go` — `Server.Shutdown`.
- `server/cmd/memorydb/main.go` — inicialização e tratamento de sinais.
- `server/internal/transport/tcp/server_test.go` — `TestServerShutdownWaitsForActiveConnections`.
- `server/internal/integration/server_test.go` — teste de shutdown com conexões ativas.

**Critérios de aceite**

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

**Estado:** Implementado, cobertura parcial.

**Limitações:**
- O servidor não força o fechamento das conexões ativas quando o contexto expira.
- O fluxo de sinais do executável não possui teste automatizado específico identificado.

### FR-011 — Controlar operações do driver por contexto

**Descrição:** O driver Go deve oferecer métodos baseados em `context.Context` para controlar cancelamento e deadlines das operações.

`DialContext` deve permitir controlar o estabelecimento inicial da conexão. O contexto utilizado para estabelecer a conexão não permanece associado ao cliente após a conexão ser criada.

O comportamento do cancelamento depende da fase da operação:

1. Se o contexto for cancelado enquanto a operação aguarda acesso ao mecanismo de serialização, a operação ainda não iniciou o I/O. O driver retorna o erro de contexto e mantém a conexão utilizável.
2. Se o contexto for cancelado durante o I/O, o driver interrompe a comunicação fechando a conexão. O resultado pode ser indeterminado, pois o servidor pode já ter executado o comando.
3. Se a comunicação e a recepção da resposta forem concluídas antes da interrupção, o resultado é processado normalmente.

O erro `OperationOutcomeUnknownError` deve preservar o identificador da solicitação e a causa original, permitindo inspeção por `errors.As` e `errors.Is`.

**Componente:** Driver Go.

**Evidências:**
- `drivers/go/client.go` — `DialContext`, métodos context-aware e `Client.execute`.
- `drivers/go/client_test.go` — testes de escrita parcial, execução no servidor seguida de perda da resposta e cancelamento durante a leitura da resposta.

**Critérios de aceite**

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
  E deve preservar o RequestID e a causa
  E o cliente não deve reutilizar a conexão interrompida

Cenário: Preservar a causa original
  Dado que uma operação retorna OperationOutcomeUnknownError
  Quando a aplicação inspecionar o erro
  Então deve conseguir identificar seu tipo com errors.As
  E deve conseguir verificar sua causa com errors.Is
```

**Estado:** Implementado e testado no driver Go.

**Limitação:** O controle por contexto não garante que uma operação cancelada durante I/O deixou de ser executada no servidor. Não existe mecanismo server-side para consultar ou recuperar seu resultado. O driver não realiza retry automático.

### FR-012 — Executar operações mutáveis com idempotência

**Descrição:** O servidor deverá oferecer deduplicação para as operações `SET` e `DELETE`, utilizando um identificador estável de operação (`OperationID`) independente do identificador de requisição (`RequestID`).

O contrato está especificado, mas a funcionalidade ainda não está implementada.

**Contrato aprovado:**

| Propriedade | Definição |
|---|---|
| Identificador | `OperationID` de 128 bits |
| Representação externa | Campo Protobuf `bytes`, com exatamente 16 bytes |
| Escopo | Global na instância do servidor |
| Operações protegidas | `SET` e `DELETE` |
| Retenção | 24 horas após o resultado terminal |
| Capacidade padrão | 100.000 registros |
| Capacidade configurável | Sim |
| Armazenamento | Memória |
| Evicção antecipada | Proibida para registros ainda válidos |

`SET` e `DELETE` deverão rejeitar solicitações sem `OperationID` ou com identificador de tamanho inválido.

O `RequestID` continuará identificando cada tentativa de requisição. O `OperationID` deverá ser reutilizado quando a aplicação repetir a mesma operação lógica.

O fingerprint deverá considerar o comando e todos os parâmetros semanticamente relevantes, incluindo chave, valor e TTL quando aplicável.

**Comportamentos exigidos:**

| Situação | Resultado esperado |
|---|---|
| Primeiro uso de um `OperationID` válido | Reservar o identificador e executar a operação |
| Mesmo identificador e mesmo fingerprint, após conclusão | Reproduzir o resultado armazenado sem executar novamente |
| Mesmo identificador e fingerprint diferente | Rejeitar por conflito |
| Requisições concorrentes com mesmo identificador e fingerprint | Aguardar a execução original e receber resultado semanticamente equivalente |
| Capacidade esgotada para identificador inédito | Rejeitar antes da execução, com erro específico |
| Identificador já expirado | Não há mais garantia de deduplicação para esse registro |

A resposta reproduzida deverá conter o `RequestID` da tentativa atual, preservando a correlação entre a requisição recebida e a resposta enviada. O resultado semântico deverá corresponder ao resultado armazenado da operação original.

A capacidade máxima deverá contar registros em andamento e concluídos. A rejeição por capacidade não deverá reservar o identificador nem executar a mutação. O identificador poderá ser tentado novamente quando houver capacidade disponível.

O prazo de retenção começará após a conclusão da operação. Registros em andamento não expiram por essa janela. Respostas estruturadas e reproduzíveis, inclusive erros determinísticos de aplicação, deverão ser armazenadas.

Uma reserva somente poderá ser liberada após falha quando houver comprovação de que a mutação não foi executada. Um resultado indeterminado não poderá ser tratado como uma falha comprovadamente anterior à execução.

**Limites da garantia:**

A garantia inicial será de no máximo uma execução por `OperationID` enquanto o registro estiver retido na mesma instância do servidor.

Os registros são exclusivamente em memória. Portanto, a deduplicação não sobrevive à reinicialização ou perda do processo.

Essa garantia não representa *exactly-once*. A durabilidade exigiria persistência atômica da mutação e do registro de deduplicação.

**Componentes previstos:**
- Schema Protobuf e tipos gerados.
- Modelo interno de protocolo.
- Handler e fluxo de processamento do servidor.
- Gerenciador de idempotência.
- Engine.
- Driver Go.

**Critérios de aceite**

```gherkin
Cenário: Executar uma operação pela primeira vez
  Dado um SET com OperationID válido e inédito
  Quando o servidor processar a solicitação
  Então deverá reservar o identificador
  E executar a operação
  E armazenar o resultado

Cenário: Repetir uma operação concluída
  Dado um OperationID já concluído
  E o mesmo fingerprint da operação original
  Quando o cliente enviar uma nova tentativa
  Então o servidor não deverá executar novamente a mutação
  E deverá reproduzir o resultado armazenado
  E deverá utilizar o RequestID da tentativa atual na resposta

Cenário: Reutilizar identificador com conteúdo diferente
  Dado um OperationID já registrado
  Quando uma solicitação utilizar o mesmo identificador com fingerprint diferente
  Então o servidor deverá rejeitar a solicitação por conflito

Cenário: Receber operações concorrentes com o mesmo identificador
  Dado que uma operação está em andamento
  Quando outra requisição chegar com o mesmo OperationID e fingerprint
  Então a segunda requisição deverá aguardar a execução original
  E não deverá executar uma segunda mutação

Cenário: Atingir a capacidade máxima
  Dado que todos os registros permitidos estão ocupados
  Quando chegar uma operação com OperationID inédito
  Então o servidor deverá rejeitá-la com erro específico
  E não deverá executar a mutação
  E não deverá remover registros válidos antecipadamente

Cenário: Rejeitar operação mutável sem identificador
  Dado que uma solicitação SET ou DELETE não contém OperationID válido
  Quando o servidor validar a solicitação
  Então deverá rejeitá-la antes da execução
```

**Estado:** Especificado e aprovado em 22/11/2026; não implementado.

**Observação de compatibilidade:** tornar `OperationID` obrigatório para `SET` e `DELETE` altera o comportamento esperado dos clientes existentes. Os drivers deverão ser atualizados de forma coordenada com o servidor.

## 4. Requisitos não funcionais e características verificáveis

Esta seção registra características observáveis da implementação atual. Itens não implementados representam lacunas, não garantias oferecidas pelo sistema.

| ID | Requisito ou característica | Estado | Evidência |
|---|---|---|---|
| NFR-001 | Operações concorrentes de armazenamento devem ser sincronizadas. | Implementado e testado | `server/internal/storage/storage.go`, `storage_test.go` |
| NFR-002 | Valores devem ser tratados como bytes, inclusive dados binários. | Implementado | `memorydb.proto`, modelo interno e codec |
| NFR-003 | Frames devem respeitar limite de 1 MiB e codificação Big Endian. | Implementado e testado | `transport/tcp/go/framing.go`, `framing_test.go` |
| NFR-004 | O servidor deve interromper a aceitação e aguardar conexões durante shutdown. | Parcial | `server/internal/transport/tcp/server.go` |
| NFR-005 | Dados devem sobreviver à reinicialização do processo. | Não implementado | `server/internal/storage/storage.go` |
| NFR-006 | Comunicação deve oferecer criptografia TLS. | Não implementado | `server/cmd/memorydb/main.go` |
| NFR-007 | O servidor deve impor timeouts de I/O e limites operacionais de conexões. | Não implementado | `server/internal/transport/tcp/server.go`, `transport/tcp/go/connection.go` |
| NFR-008 | O projeto deve possuir metas mensuráveis de latência e throughput. | Não identificado | Não foram identificadas metas ou benchmarks de performance |
| NFR-009 | Falhas de processamento de conexões devem ser observáveis em logs. | Não implementado no fluxo atual | `server/internal/transport/tcp/server.go` |
| NFR-010 | Execução de comandos, storage, protocolo, codec e transporte devem possuir responsabilidades separadas. | Implementado | Pacotes internos do módulo `server` |
| NFR-011 | O transporte TCP deve ser reutilizável sem dependência direta do Protobuf. | Implementado | `transport/tcp/go`, `drivers/go`, handler do servidor |
| NFR-012 | Aplicações Go devem consumir o serviço por meio de uma API cliente. | Implementado e testado | `drivers/go/client.go`, `drivers/go/client_test.go` |
| NFR-013 | Os módulos Go devem permitir desenvolvimento e testes independentes no workspace. | Implementado | `go.work` e arquivos `go.mod` |
| NFR-014 | O código deve passar pelos testes com detector de condições de corrida e análise estática. | Validação histórica | Registros de validação por módulo descritos na seção 8 |
| NFR-015 | O registro de idempotência deve respeitar capacidade configurada, retenção de 24 horas e ausência de evicção antecipada de registros válidos. | Especificado, não implementado | Contrato FR-012; implementação futura no gerenciador de idempotência |
### 4.1 Observações sobre os requisitos não funcionais

**NFR-004 — Shutdown**

O servidor fecha o listener e aguarda as conexões ativas. O executável utiliza timeout de 10 segundos. A expiração do contexto não implica fechamento forçado das conexões.

**NFR-007 — Limites operacionais**

O limite de tamanho dos frames não equivale a um limite de conexões, timeout de leitura, timeout de escrita ou proteção contra clientes lentos.

Os contextos do driver controlam operações no lado cliente, mas não substituem políticas operacionais aplicadas pelo servidor.

**NFR-009 — Observabilidade**

A goroutine criada por `Server.Serve()` descarta o erro retornado por `Handler.Handle`. Portanto, falhas individuais de conexão não são registradas de forma estruturada nesse ponto.

**NFR-014 — Validação**

A aprovação em testes é uma evidência associada a uma versão do código e a uma execução específica. Não representa garantia permanente para alterações futuras.

## 5. Casos de borda e comportamento observado

| Caso | Comportamento atual | Evidência |
|---|---|---|
| Solicitação nula | Retorna `INVALID_REQUEST` no engine | `engine.go`, `engine_test.go` |
| Comando ausente | Retorna `INVALID_REQUEST` | `engine.go`, `engine_test.go` |
| Comando tipado nulo | Rejeitado como requisição inválida | `engine.go`, `engine_test.go` |
| Chave vazia | SET, GET e DELETE rejeitam com `INVALID_ARGUMENT` | `engine.go`, `engine_test.go` |
| TTL maior que zero | SET retorna `UNSUPPORTED_FEATURE` | `engine.go`, `engine_test.go` |
| Valor vazio | Pode ser armazenado; GET informa `found=true` | `storage.go`, `storage_test.go` |
| Chave inexistente | GET retorna `found=false`; DELETE retorna `deleted=false` | `engine.go`, `engine_test.go` |
| Substituição de valor | SET substitui o valor anterior | `storage_test.go` |
| Mutação de slice pelo chamador | SET e GET copiam bytes | `storage.go`, `storage_test.go` |
| Frame com tamanho zero | Rejeitado pelo framing | `framing.go`, `framing_test.go` |
| Frame acima de 1 MiB | Rejeitado pelo framing | `framing.go`, `framing_test.go` |
| Frame truncado | Cabeçalho ou payload incompleto retorna erro | `framing_test.go` |
| Escrita parcial | O framing tenta completar a escrita ou retorna erro | `framing_test.go` |
| Protobuf inválido | Decode retorna erro | `codec.go`, `codec_test.go` |
| Comando ou resultado ausente no codec | Estrutura inválida é rejeitada | `codec.go`, `codec_test.go` |
| Cliente desconecta por EOF | Handler encerra sem tratar EOF como falha de aplicação | `handler.go`, testes de integração |
| Concorrência no storage | Operações são sincronizadas por `RWMutex` | `storage.go`, `storage_test.go` |
| Múltiplos clientes | Conexões distintas compartilham o storage da instância | Testes do servidor e integração |
| Desconexão durante operação | Driver propaga falha de comunicação | `client.go`, `client_test.go` |
| Cancelamento antes do I/O | Retorna erro de contexto sem invalidar a conexão | `client.go`, `client_test.go` |
| Cancelamento durante o I/O | Interrompe conexão e sinaliza resultado indeterminado | `client.go`, `client_test.go` |
| Escrita parcial seguida de falha | Driver não presume que a operação deixou de ser executada | `client_test.go` |
| Servidor executa SET, mas resposta não chega | Driver pode retornar resultado indeterminado | `TestClientSetReturnsUnknownOutcomeAfterServerExecution` |
| Identificador da solicitação | `request_id` correlaciona solicitação e resposta; não deduplica | `memorydb.proto`, `client.go` |

## 6. Rastreabilidade requisito–componente–teste

| Requisito | Componente principal | Testes identificados | Cobertura |
|---|---|---|---|
| FR-001 | Engine | `TestPing` | Coberto |
| FR-002 | Engine / Storage | `TestSetAndGet`, `TestSetOverwritesExistingValue` | Coberto |
| FR-003 | Engine | `TestRejectsUnsupportedTTL` | Coberto para rejeição |
| FR-004 | Engine / Storage | `TestGetMissingKey`, `TestEmptyValue`, `TestServerEmptyValueAndMissingKey` | Coberto |
| FR-005 | Engine / Storage | `TestDelete`, `TestServerSetGetDelete` | Coberto |
| FR-006 | Engine | `TestRejectsEmptyKey`, `TestRejectsNilRequest`, `TestRejectsMissingCommand` | Parcial: faltam testes específicos de chave vazia em GET e DELETE |
| FR-007 | Engine / Codec | `TestPing`, `TestRequestRoundTrip`, `TestResponseRoundTrip` | Coberto |
| FR-008 | Framing TCP | Testes de endian, fragmentação, limites, truncamento e escrita parcial | Coberto |
| FR-009 | Handler / Server / Driver | `TestHandlerProcessesMultipleRequests`, `TestServerAcceptsMultipleClients`, testes de integração | Coberto para conexões distintas e requisições sequenciais |
| FR-010 | Server / Executável | `TestServerShutdownWaitsForActiveConnections` | Parcial: fluxo de sinais sem teste específico identificado |
| FR-011 | Driver Go | Testes de escrita parcial, execução no servidor com perda da resposta e cancelamento durante leitura | Coberto para os cenários identificados |

## 7. Lacunas de especificação e comportamento

Os itens desta seção não são requisitos funcionais já disponíveis. São limitações identificadas que precisam de decisões de produto e engenharia antes de serem transformadas em novas especificações.

### GAP-001 — TTL funcional não implementado

O contrato Protobuf declara `ttl_ms`, mas qualquer valor positivo é rejeitado pelo engine.

Não existe expiração automática de entradas.

**Impacto:** aplicações não podem solicitar expiração temporária de chaves.

**Referências:** `protocol/proto/memorydb/v1/memorydb.proto`, `server/internal/engine/engine.go`.

### GAP-002 — Persistência não implementada

O armazenamento é mantido exclusivamente em memória.

**Impacto:** a reinicialização do processo elimina os dados. Não há durabilidade nem recuperação após falhas.

**Referência:** `server/internal/storage/storage.go`.

### GAP-003 — Autenticação e autorização não implementadas

O servidor não identifica clientes nem valida permissões para execução de comandos.

**Impacto:** qualquer cliente que consiga estabelecer conexão TCP pode tentar executar operações.

### GAP-004 — TLS não implementado

A comunicação utiliza TCP sem criptografia.

**Impacto:** os dados não possuem proteção criptográfica no transporte.

**Referência:** `server/cmd/memorydb/main.go`.

### GAP-005 — Isolamento por tenant não implementado

O armazenamento é compartilhado entre todos os clientes conectados à mesma instância.

**Impacto:** não existe separação lógica de dados por organização ou cliente.

### GAP-006 — Limites operacionais incompletos

O tamanho dos frames é limitado a 1 MiB, mas não foram identificados limites específicos para:

- quantidade de conexões simultâneas;
- tempo máximo de leitura no servidor;
- tempo máximo de escrita no servidor;
- tempo máximo de inatividade de uma conexão;
- proteção contra clientes que enviam dados lentamente.

**Impacto:** conexões lentas ou excessivas podem consumir recursos do servidor.

**Referências:** `transport/tcp/go/connection.go`, `transport/tcp/go/framing.go`, `server/internal/transport/tcp/server.go`.

### GAP-007 — Observabilidade de erros de conexão incompleta

O servidor executa `Handler.Handle` em goroutines, mas descarta o erro retornado pelo handler.

**Impacto:** falhas de processamento de conexões não são registradas de maneira estruturada pelo servidor.

**Referência:** `server/internal/transport/tcp/server.go`.

### GAP-008 — Multiplexação de requisições não implementada

O driver Go utiliza sincronização para garantir que uma solicitação seja enviada e sua resposta recebida antes de iniciar a próxima operação na mesma conexão.

**Impacto:** uma conexão não permite múltiplas requisições simultâneas em trânsito, embora o servidor atenda várias conexões simultaneamente.

**Referência:** `drivers/go/client.go`.

### GAP-009 — Resultado indeterminado sem recuperação server-side

O driver Go identifica situações nas quais o resultado de uma operação não pode ser confirmado pelo cliente.

Quando o cancelamento acontece antes do início do I/O, o driver retorna o erro de contexto sem invalidar a conexão.

Quando o cancelamento ou uma falha de comunicação acontece durante o I/O, o servidor pode já ter executado o comando, mesmo que o cliente não tenha recebido a resposta.

Nesse cenário, o driver interrompe a conexão e retorna `OperationOutcomeUnknownError`, preservando o `RequestID` e a causa original.

Entretanto, o servidor ainda não oferece:

- identificador estável de operação para deduplicação;
- registro de operações executadas e seus resultados;
- consulta posterior do resultado de uma operação;
- reenvio seguro com garantia de não duplicação;
- recuperação de resultados após desconexão.

**Impacto:** aplicações precisam tratar explicitamente resultados indeterminados. Repetir automaticamente uma operação após esse erro pode executar o comando novamente. O driver não realiza retry automático.

**Referência:** `drivers/go/client.go`.

### GAP-010 — Driver Java não implementado e validado

A arquitetura permite evolução para outros drivers, mas não há implementação e validação de driver Java consideradas parte do escopo funcional atual.

**Impacto:** a integração por Java não deve ser considerada requisito implementado.

### GAP-011 — Idempotência e deduplicação não implementadas

O protocolo possui `request_id`, utilizado para correlacionar requisições e respostas. Entretanto, esse identificador não constitui uma chave de idempotência.

O servidor não mantém registro de operações executadas, não identifica reenvios de uma mesma operação lógica e não reproduz resultados anteriores.

**Impacto:** em caso de perda de resposta ou resultado indeterminado, o cliente não consegue reenviar uma operação com garantia de que ela será executada no máximo uma vez.

A implementação futura deverá definir:

- identificador estável da operação, independente do identificador de transporte;
- escopo e período de retenção dos identificadores;
- armazenamento do resultado associado à operação;
- comportamento para identificadores repetidos com conteúdo divergente;
- política de concorrência para requisições com o mesmo identificador;
- relação entre deduplicação, persistência e recuperação após reinicialização.

**Referências:** `protocol/proto/memorydb/v1/memorydb.proto`, `drivers/go/client.go`.

## 8. Histórico de validação técnica

Os registros disponíveis indicam uma validação integral dos módulos em 15/11/2026, com os seguintes resultados:

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

Os cenários adicionais contemplaram:

- falha durante escrita parcial;
- servidor executando SET antes de a resposta ser perdida;
- cancelamento durante a leitura da resposta final;
- preservação do identificador e da causa do resultado indeterminado.

**Observação:** a validação integral de 15/11 comprova o estado dos módulos naquela execução. As validações posteriores comprovam cenários adicionais do driver Go, mas não substituem uma nova execução completa de todos os módulos após as alterações.

Para atualizar a evidência do estado atual, executar novamente os testes e a análise estática em cada módulo do workspace.

## 9. Critério geral de conclusão do escopo atual

O escopo funcional inicial do MemoryDB é considerado implementado quando:

1. PING, SET, GET e DELETE funcionam conforme os contratos definidos.
2. O engine valida requisições e argumentos.
3. O armazenamento preserva a distinção entre chave ausente e valor vazio.
4. O protocolo Protobuf representa requisições e respostas.
5. O transporte TCP delimita mensagens corretamente.
6. O servidor atende múltiplos clientes e conexões persistentes.
7. O driver Go permite consumir as operações sem implementar manualmente o protocolo.
8. O driver Go oferece operações context-aware e sinaliza resultados indeterminados quando não é possível confirmar o desfecho de uma operação.
9. O servidor oferece encerramento gracioso dentro das limitações documentadas.
10. Os testes automatizados e a análise estática passam nos módulos do workspace.

Os GAPs documentados neste arquivo não impedem a conclusão do escopo inicial, pois representam evoluções planejadas e limitações conhecidas da implementação atual.

Este documento não substitui o roadmap:

- `REQUISITOS.md` registra o comportamento implementado, os critérios de aceite e as lacunas.
- `memorydb-roadmap.md` registra as evoluções planejadas.
- `ARQUITETURA.md` registra a organização arquitetural e as dependências.
- `COMPONENTES.md` registra responsabilidades e contratos dos componentes.