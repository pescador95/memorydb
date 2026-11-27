# Requisitos e Critérios de Aceite — MemoryDB

Este documento registra os requisitos funcionais e as características não funcionais verificáveis na implementação atual do MemoryDB.

A documentação é retrospectiva: os requisitos são descritos a partir do comportamento implementado no código e dos testes identificados no repositório.

O documento também registra limitações e lacunas que devem ser consideradas em evoluções futuras. Funcionalidades planejadas, mas ainda não implementadas, não são apresentadas como capacidades disponíveis.

## 1. Objetivo

O objetivo deste documento é:

- Identificar os comportamentos funcionais disponíveis.
- Registrar os requisitos não funcionais e as características técnicas verificáveis.
- Relacionar requisitos aos componentes responsáveis.
- Identificar os testes que dão evidência aos comportamentos descritos.
- Registrar limitações de implementação e cobertura de testes.
- Diferenciar funcionalidades implementadas de evoluções futuras.

Os estados utilizados possuem os seguintes significados:

| Estado | Significado |
|---|---|
| Implementado e testado | Comportamento presente no código e coberto por testes identificados para os cenários descritos |
| Implementado, cobertura parcial | Comportamento presente no código, mas sem cobertura de teste suficiente para todos os cenários descritos |
| Implementado, validação histórica | Há registro de execução bem-sucedida em uma versão específica, sem representar necessariamente uma validação completa do estado atual |
| Implementado | Comportamento presente no código, sem declaração específica sobre cobertura de testes |
| Parcial | Parte do comportamento está implementada, mas há limitações conhecidas |
| Não implementado | Não há implementação funcional identificada |
| Não identificado | Não foi identificada especificação, meta ou evidência suficiente para afirmar sua existência |

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
- Módulo Storage independente, reutilizável como biblioteca Go.
- Integração do Storage ao Engine por interface e injeção de dependência.
- Driver cliente em Go.
- Operações context-aware no driver Go.
- Sinalização de resultados indeterminados por `OperationOutcomeUnknownError`.
- Idempotência server-side para as operações `SET` e `DELETE`, baseada em `OperationID`.
- Deduplicação de operações enquanto seus registros permanecem retidos na instância do servidor.
- Reprodução de respostas de operações anteriormente concluídas.
- Controle de concorrência, retenção e capacidade dos registros de idempotência.
- Encerramento gracioso e forçado do servidor.
- Controle do número máximo de conexões TCP simultâneas.
- Timeouts de leitura e escrita no processamento das conexões TCP.
- Dimensionamento de recursos por perfis, com detecção de CPU e memória disponíveis no ambiente.
- Limites lógicos de Storage, idempotência e processamento, além de limites de conexões, requisições em processamento e tamanho de frame.
- Workspace Go composto por módulos independentes.

### 2.1. Limitações do escopo

A implementação atual não oferece:

- Persistência dos dados em disco.
- Recuperação dos dados após reinicialização.
- Persistência dos registros de idempotência.
- TTL funcional para as chaves armazenadas.
- Autenticação ou autorização.
- TLS.
- Isolamento por tenant.
- Replicação ou distribuição entre instâncias.
- Multiplexação de requisições na mesma conexão.
- Timeout específico de inatividade de uma conexão.
- Garantia de idempotência após reinicialização, perda do processo ou expiração do registro.
- Retry automático no driver.
- Driver Java implementado e validado.
- Limite rígido de memória do processo ou limite de CPU aplicado pelo runtime/sistema operacional.

Os limites de leitura, escrita e quantidade de conexões TCP são implementados. Eles não devem ser confundidos com limites de inatividade, controle de taxa de requisições ou políticas completas de isolamento entre clientes.

O módulo Storage independente não representa um serviço ou processo separado. Ele é incorporado ao processo do servidor como biblioteca Go e executado por chamadas locais, no mesmo espaço de endereçamento e heap.

## 3. Requisitos funcionais

### FR-001 — Responder ao comando PING

**Descrição:** Para uma solicitação PING válida, o engine deve retornar sucesso com resultado PONG e preservar o `request_id`.

**Componentes:** `Engine` e modelo interno de protocolo.

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

O fluxo de processamento de SET exige um `OperationID` válido. Essa validação é realizada pelo Handler/Executor antes da execução do comando pelo Engine.

O Engine utiliza uma abstração de Storage definida em seu próprio pacote. A implementação concreta é fornecida pelo executável do servidor por injeção de dependência.

**Componentes:** `Handler`, `idempotency.Executor`, `Engine` e módulo Storage.

**Evidências:**

- `server/internal/transport/tcp/handler.go` — encaminhamento de SET pelo fluxo de idempotência.
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/storage.go` — interface consumida pelo Engine.
- `storage/go/storage.go` — implementação de `Storage.Set`.
- `server/cmd/memorydb/main.go` — instanciação e injeção da implementação concreta.
- `server/internal/engine/engine_test.go` — `TestSetAndGet`.
- `storage/go/storage_test.go` — `TestSetOverwritesExistingValue`.

**Critérios de aceite**

```gherkin
Cenário: Gravar um valor
  Dado que a chave "name" não está vazia
  E que ttl_ms é zero
  E que a solicitação contém um OperationID válido
  Quando SET for executado pelo handler
  Então o engine deve armazenar o valor
  E a resposta deve indicar sucesso
  E um GET posterior deve encontrar os mesmos bytes

Cenário: Substituir valor existente
  Dado que a chave "name" contém "old"
  E que SET utiliza um OperationID válido
  Quando SET gravar "new" para a mesma chave
  Então GET deve retornar "new"
```

**Estado:** Implementado e testado.

### FR-003 — Rejeitar TTL ainda não suportado

**Descrição:** SET com `ttl_ms` maior que zero deve ser rejeitado com `UNSUPPORTED_FEATURE`, sem gravar a entrada.

O campo existe no contrato externo, mas a expiração automática de entradas não está implementada.

A solicitação deve possuir um `OperationID` válido, cuja validação ocorre no fluxo do Handler/Executor antes da execução do comando. Após essa validação, o Handler encaminha a operação ao Engine, que deve rejeitar o TTL positivo com o erro `UNSUPPORTED_FEATURE`, sem alterar o estado do armazenamento.

**Componentes:** Contrato Protobuf, `Handler`, `idempotency.Executor` e `Engine`.

**Evidências:**

- `protocol/proto/memorydb/v1/memorydb.proto` — campo `ttl_ms`.
- `server/internal/engine/engine.go` — validação de `TTLMillis`.
- `server/internal/engine/engine_test.go` — `TestRejectsUnsupportedTTL`.
- `server/internal/idempotency/fingerprint.go` — inclusão de TTL no fingerprint de SET.
- `server/internal/idempotency/fingerprint_test.go` — testes do cálculo de fingerprint.

**Critérios de aceite**

```gherkin
Cenário: Rejeitar SET com TTL positivo
  Dado que SET recebe uma chave não vazia
  E que ttl_ms é maior que zero
  E que a solicitação contém um OperationID válido
  Quando o handler validar o OperationID e encaminhar a operação
  Então o engine deve responder com success=false
  E o código do erro deve ser UNSUPPORTED_FEATURE
  E a entrada não deve ser gravada
```

**Estado:** Implementado e testado quanto à rejeição.

**Limitação:** A funcionalidade de expiração continua não implementada. O valor de TTL também integra o fingerprint da operação, evitando que o mesmo `OperationID` seja reutilizado com parâmetros semanticamente diferentes.

### FR-004 — Consultar chave existente ou ausente

**Descrição:** GET deve retornar os bytes e `found=true` para uma chave existente.

Para uma chave inexistente, deve retornar `found=false`, sem confundir ausência com valor vazio.

**Componentes:** `Engine` e módulo Storage.

**Evidências:**

- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/storage.go` — interface consumida pelo Engine.
- `storage/go/storage.go` — implementação de `Storage.Get`.
- `server/internal/engine/engine_test.go` — `TestSetAndGet` e `TestGetMissingKey`.
- `storage/go/storage_test.go` — `TestEmptyValue`.
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

DELETE exige um `OperationID` válido. Repetições da mesma operação lógica devem reproduzir o resultado original enquanto o registro de idempotência estiver retido.

**Componentes:** `Handler`, `idempotency.Executor`, `Engine` e módulo Storage.

**Evidências:**

- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/storage.go` — interface consumida pelo Engine.
- `storage/go/storage.go` — implementação de `Storage.Delete`.
- `server/internal/engine/engine_test.go` — `TestDelete`.
- `storage/go/storage_test.go` — `TestDelete`.
- `server/internal/transport/tcp/handler_test.go` — testes de DELETE e idempotência.
- `server/internal/integration/server_test.go` — `TestServerSetGetDelete`.

**Critérios de aceite**

```gherkin
Cenário: Remover chave existente
  Dado que a chave "name" foi gravada
  E que DELETE utiliza um OperationID válido
  Quando DELETE remover "name"
  Então a resposta deve indicar deleted=true
  E um GET posterior deve indicar found=false

Cenário: Remover chave ausente
  Dado que a chave "missing" não existe
  E que DELETE utiliza um OperationID válido
  Quando DELETE remover "missing"
  Então a resposta deve ser bem-sucedida
  E deve indicar deleted=false
```

**Estado:** Implementado e testado.

### FR-006 — Validar requisições e argumentos

**Descrição:** O engine deve rejeitar:

- Solicitação nula.
- Comando ausente.
- Comando desconhecido.
- Comando representado por ponteiro tipado nulo.
- Chave vazia em SET, GET ou DELETE.

O fluxo de processamento também deve rejeitar `OperationID` ausente ou com tamanho diferente de 16 bytes nas operações SET e DELETE.

As validações são distribuídas entre os componentes: o Engine valida a estrutura da solicitação e os argumentos dos comandos; o Handler/Executor valida o identificador de idempotência antes de encaminhar as operações mutáveis.

Os erros de validação devem ser representados por:

| Condição | Código |
|---|---|
| Solicitação ou comando inválido | `INVALID_REQUEST` |
| Chave vazia | `INVALID_ARGUMENT` |
| `OperationID` ausente ou inválido | `INVALID_OPERATION_ID` |

Os erros de conflito, capacidade e resultado indeterminado relacionados à execução idempotente são especificados no FR-012.

**Componentes:** `Engine`, `Handler`, `idempotency.Executor` e funções de construção de respostas de erro.

**Evidências:**

- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/command.go` — `errorResponse`.
- `server/internal/engine/engine_test.go` — `TestRejectsNilRequest`, `TestRejectsMissingCommand` e `TestRejectsEmptyKey`.
- `server/internal/transport/tcp/handler.go` — validação de OperationID e conversão de erros de idempotência.
- `server/internal/transport/tcp/handler_test.go` — testes de identificadores inválidos.
- `server/internal/idempotency/executor_test.go` — testes do executor.

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

Cenário: Rejeitar OperationID inválido
  Dado que SET ou DELETE recebeu um OperationID ausente ou com tamanho incorreto
  Quando o handler processar a solicitação
  Então deve responder com INVALID_OPERATION_ID
  E não deve executar a mutação
```

**Estado:** Implementado, cobertura parcial.

**Lacunas de testes:** O teste identificado para chave vazia cobre SET. GET e DELETE possuem validação no código, mas não foram identificados testes específicos equivalentes para essas operações.

### FR-007 — Correlacionar respostas com solicitações

**Descrição:** O engine deve copiar o `request_id` da solicitação para a resposta.

O codec deve preservar o identificador durante a serialização e desserialização.

O identificador permite correlacionar uma resposta com sua solicitação correspondente. Não representa uma chave de idempotência e não impede, por si só, a execução repetida de uma operação.

Nas respostas reproduzidas pelo mecanismo de idempotência, o `RequestID` deve ser atualizado para o identificador da tentativa atual, mantendo a correlação entre a requisição recebida e a resposta enviada.

**Componentes:** `Engine`, modelo interno, codec Protobuf, `Handler` e `idempotency.Executor`.

**Evidências:**

- `protocol/proto/memorydb/v1/memorydb.proto` — envelopes `Request` e `Response`.
- `server/internal/engine/engine.go` — `Engine.Execute`.
- `server/internal/engine/engine_test.go` — `TestPing`.
- `server/internal/codec/protobuf/codec_test.go` — `TestRequestRoundTrip` e `TestResponseRoundTrip`.
- `server/internal/idempotency/executor_test.go` — testes de reprodução de resposta.
- `server/internal/transport/tcp/handler_test.go` — teste de replay com atualização do RequestID.

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

Cenário: Atualizar RequestID em resposta reproduzida
  Dado que uma operação já foi executada com determinado OperationID
  Quando uma nova tentativa utilizar o mesmo OperationID e fingerprint
  E possuir outro RequestID
  Então a resposta reproduzida deve utilizar o RequestID da tentativa atual
  E deve preservar o resultado semântico da operação original
```

**Estado:** Implementado e testado.

### FR-008 — Delimitar mensagens TCP

**Descrição:** Cada frame deve utilizar:

- Cabeçalho de 4 bytes.
- Tamanho `uint32`.
- Ordem de bytes Big Endian.
- Payload mínimo de 1 byte.
- Payload máximo de 1 MiB.

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

O servidor deve aceitar conexões de múltiplos clientes simultaneamente. As conexões de uma mesma instância devem compartilhar a mesma implementação de Storage e o mesmo executor de idempotência.

O módulo Storage é uma biblioteca Go carregada pelo processo do servidor. O compartilhamento ocorre por meio da mesma instância injetada no Engine, sem comunicação RPC, IPC ou tráfego de rede entre Engine e Storage.

O driver Go deve serializar o ciclo de requisição e resposta por conexão, garantindo a correspondência entre cada solicitação e sua resposta.

**Componentes:** `Handler`, `Server`, transporte TCP, `Engine`, módulo Storage, `idempotency.Executor` e driver Go.

**Evidências:**

- `server/internal/transport/tcp/handler.go` — `Handler.Handle`.
- `server/internal/transport/tcp/server.go` — `Server.Serve`.
- `server/internal/engine/storage.go` — interface consumida pelo Engine.
- `server/cmd/memorydb/main.go` — instanciação e injeção do Storage.
- `storage/go/storage.go` — implementação do armazenamento.
- `transport/tcp/go/connection.go` — `Connection`.
- `drivers/go/client.go` — `Client.execute`.
- `server/internal/transport/tcp/handler_test.go` — `TestHandlerProcessesMultipleRequests`.
- `server/internal/transport/tcp/server_test.go` — `TestServerAcceptsMultipleClients`.
- `server/internal/integration/server_test.go` — testes de compartilhamento de storage e clientes concorrentes.
- `server/internal/idempotency/executor_test.go` — testes de concorrência para identificadores repetidos.

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

Cenário: Compartilhar registros de idempotência
  Dado que dois clientes estão conectados à mesma instância
  Quando um deles executar uma operação mutável
  E o outro repetir a mesma operação lógica com o mesmo OperationID
  Então o servidor deve reconhecer o registro compartilhado
  E não deve executar novamente a mutação
```

**Estado:** Implementado e testado.

**Limitação:** Não existe multiplexação de requisições simultâneas na mesma conexão. O driver Go executa um ciclo de requisição e resposta por vez.

### FR-010 — Encerrar servidor graciosamente e forçadamente

**Descrição:** `Shutdown` deve impedir novas conexões, fechar o listener e aguardar a conclusão das conexões ativas.

Se o contexto expirar antes da conclusão, o servidor deve fechar as conexões ainda ativas, aguardar a conclusão das goroutines dos handlers e retornar o erro do contexto, associado a eventual erro no fechamento do listener.

O executável deve solicitar encerramento ao receber SIGINT ou SIGTERM, utilizando o timeout configurado para o shutdown.

O fechamento das conexões ativas interrompe operações de I/O TCP bloqueadas. Isso não representa cancelamento genérico de qualquer operação que um handler possa executar fora dessas operações de rede.

**Componentes:** `Server` e executável do servidor.

**Evidências:**

- `server/internal/transport/tcp/server.go` — `Server.Shutdown`.
- `server/cmd/memorydb/main.go` — inicialização e tratamento de sinais.
- `server/internal/transport/tcp/server_test.go` — `TestServerShutdownWaitsForActiveConnections`.
- `server/internal/transport/tcp/server_test.go` — `TestServerClosesAcceptedConnectionDuringShutdown`.
- `server/internal/integration/server_test.go` — testes de shutdown com conexões ativas.

**Critérios de aceite**

```gherkin
Cenário: Aguardar conexão ativa durante shutdown
  Dado que o servidor tem uma conexão ativa
  Quando Shutdown for chamado
  Então o listener deve parar de aceitar conexões
  E Shutdown deve aguardar a conexão terminar

Cenário: Fechar conexões quando o contexto expirar
  Dado que o servidor tem uma conexão ativa
  E que o contexto de shutdown expira antes da conclusão
  Quando Shutdown atingir o prazo do contexto
  Então deve fechar as conexões ainda ativas
  E aguardar a conclusão das goroutines dos handlers
  E retornar o erro do contexto

Cenário: Encerrar após sinal do sistema operacional
  Dado que o executável do servidor está em execução
  Quando receber SIGINT ou SIGTERM
  Então deve solicitar o encerramento gracioso
  E deve respeitar o timeout configurado
```

**Estado:** Implementado, cobertura parcial.

**Limitações:**

- O fechamento forçado interrompe operações de I/O TCP nas conexões ativas, mas não representa cancelamento genérico de qualquer operação que um handler possa executar.
- O fluxo de sinais do executável não possui teste automatizado específico identificado.
- Após fechar as conexões, o servidor ainda aguarda a conclusão das goroutines dos handlers antes de retornar.

### FR-011 — Controlar operações do driver por contexto

**Descrição:** O driver Go deve oferecer métodos baseados em `context.Context` para controlar cancelamento e deadlines das operações.

`DialContext` deve permitir controlar o estabelecimento inicial da conexão. O contexto utilizado para estabelecer a conexão não permanece associado ao cliente após a conexão ser criada.

O comportamento do cancelamento depende da fase da operação:

1. Se o contexto for cancelado enquanto a operação aguarda acesso ao mecanismo de serialização, a operação ainda não iniciou o I/O. O driver retorna o erro de contexto e mantém a conexão utilizável.
2. Se o contexto for cancelado durante o I/O, o driver interrompe a comunicação fechando a conexão. O resultado pode ser indeterminado, pois o servidor pode já ter executado o comando.
3. Se a comunicação e a recepção da resposta forem concluídas antes da interrupção, o resultado é processado normalmente.

O erro `OperationOutcomeUnknownError` deve preservar o identificador da solicitação e a causa original, permitindo inspeção por `errors.As` e `errors.Is`.

Esse erro do driver representa uma incerteza causada pela interrupção da comunicação. É distinto do código estruturado `OPERATION_OUTCOME_UNKNOWN`, que pode ser retornado pelo servidor quando o Executor não consegue determinar com segurança o resultado da operação.

Quando o resultado for indeterminado em uma operação SET ou DELETE, a aplicação poderá repetir a mesma operação lógica utilizando o mesmo `OperationID`, desde que os parâmetros semânticos permaneçam iguais e o registro ainda esteja retido na instância do servidor.

**Componente:** Driver Go, com suporte do mecanismo de idempotência no servidor para recuperação por repetição.

**Evidências:**

- `drivers/go/client.go` — `DialContext`, métodos context-aware e `Client.execute`.
- `drivers/go/client_test.go` — testes de escrita parcial, execução no servidor seguida de perda da resposta e cancelamento durante a leitura da resposta.
- `drivers/go/client_test.go` — `TestClientSetReturnsUnknownOutcomeAfterServerExecution`, que comprova a sinalização de resultado indeterminado após a execução no servidor.
- `server/internal/idempotency/executor_test.go` — testes de reprodução do resultado original para operações repetidas com o mesmo identificador e fingerprint.
- `server/internal/transport/tcp/handler_test.go` — testes de integração do replay e atualização do RequestID.

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

Cenário: Repetir operação indeterminada com o mesmo OperationID
  Dado que uma operação SET ou DELETE retornou resultado indeterminado
  E que a aplicação preservou o OperationID original
  Quando a aplicação repetir a mesma operação lógica com o mesmo OperationID
  E os parâmetros semânticos forem idênticos
  E o registro ainda estiver retido no servidor
  Então o servidor deverá reproduzir o resultado original
  E não deverá executar novamente a mutação
```

**Estado:** Implementado, cobertura parcial.

**Limitações:**

- O controle por contexto não garante que uma operação cancelada durante I/O deixou de ser executada no servidor.
- A recuperação por repetição depende de a operação ser SET ou DELETE, de o cliente reutilizar o mesmo `OperationID` e de o registro permanecer disponível na mesma instância do servidor.
- O driver não realiza retry automático.
- Não há garantia de recuperação após reinicialização, perda do processo ou expiração do registro.
- A recuperação por repetição está implementada no servidor e possui cobertura nos testes do Executor e do Handler. Não foi identificado teste ponta a ponta que comprove a sequência completa de resultado indeterminado no driver seguida de repetição e recuperação do resultado original.

### FR-012 — Executar operações mutáveis com idempotência

**Descrição:** O servidor deve oferecer deduplicação para as operações `SET` e `DELETE`, utilizando um identificador estável de operação (`OperationID`) independente do identificador de requisição (`RequestID`).

O `OperationID` identifica uma operação lógica. O `RequestID` identifica uma tentativa de requisição e correlaciona sua resposta.

O servidor calcula um fingerprint dos parâmetros semanticamente relevantes da operação e associa o resultado terminal ao `OperationID`.

**Contrato implementado:**

| Propriedade | Definição |
|---|---|
| Identificador | `OperationID` de 128 bits |
| Representação interna | `[16]byte` no driver e `[]byte` no modelo de protocolo |
| Representação externa | Campo Protobuf `bytes`, com exatamente 16 bytes |
| Escopo | Global na instância do servidor |
| Operações protegidas | `SET` e `DELETE` |
| Retenção | 24 horas após a conclusão da operação |
| Capacidade no perfil `mid` | 100.000 registros |
| Capacidade configurável | Não identificada como configuração externa na implementação atual |
| Armazenamento | Memória |
| Evicção antecipada | Não ocorre para registros ainda válidos |
| Fingerprint | SHA-256 com esquema versionado |
| Persistência | Não implementada |

`SET` e `DELETE` rejeitam solicitações sem `OperationID` ou com identificador de tamanho inválido.

O `RequestID` continua identificando cada tentativa de requisição. O `OperationID` deve ser reutilizado quando a aplicação repetir a mesma operação lógica.

O fingerprint considera:

- O tipo da operação.
- A chave.
- O valor, no caso de SET.
- O TTL, no caso de SET.

O próprio `OperationID` não integra o fingerprint.

**Comportamentos implementados:**

| Situação | Resultado |
|---|---|
| Primeiro uso de um `OperationID` válido | Reservar o identificador e executar a operação |
| Mesmo identificador e mesmo fingerprint, após conclusão | Reproduzir o resultado armazenado sem executar novamente |
| Mesmo identificador e fingerprint diferente | Rejeitar por conflito |
| Requisições concorrentes com mesmo identificador e fingerprint | Aguardar a execução original e receber resultado semanticamente equivalente |
| Capacidade esgotada para identificador inédito | Rejeitar antes da execução, com erro específico |
| Identificador expirado | Não há mais garantia de deduplicação para esse registro |
| Requisição repetida com outro `RequestID` | Reproduzir o resultado com o `RequestID` da tentativa atual |

A resposta reproduzida contém o `RequestID` da tentativa atual, preservando a correlação entre a requisição recebida e a resposta enviada. O resultado semântico corresponde ao resultado armazenado da operação original.

A capacidade máxima conta registros em andamento e concluídos. A rejeição por capacidade não reserva o identificador nem executa a mutação. O identificador poderá ser tentado novamente quando houver capacidade disponível.

O prazo de retenção começa após a conclusão da operação. Registros em andamento não expiram por essa janela.

Respostas terminais estruturadas e reproduzíveis, inclusive erros determinísticos de aplicação, são armazenadas.

Uma reserva somente pode ser liberada após falha quando houver comprovação de que a mutação não foi executada. Um resultado indeterminado não pode ser tratado como uma falha comprovadamente anterior à execução.

O executor realiza a execução da operação fora de sua seção crítica, evitando manter o mutex durante a execução do Engine. Requisições concorrentes para o mesmo identificador aguardam o resultado da execução original.

**Limites da garantia:**

A garantia é de no máximo uma execução por `OperationID` enquanto o registro estiver retido na mesma instância do servidor.

Os registros são exclusivamente em memória. Portanto, a deduplicação não sobrevive à reinicialização ou perda do processo.

Essa garantia não representa *exactly-once*. A durabilidade exigiria persistência atômica da mutação e do registro de deduplicação.

A expiração do registro após 24 horas também encerra a garantia de deduplicação para aquele identificador. Uma repetição posterior poderá ser tratada como uma nova operação.

**Componentes:**

- Schema Protobuf e tipos gerados.
- Modelo interno de protocolo.
- Handler e fluxo de processamento do servidor.
- Pacote `server/internal/idempotency`.
- Engine.
- Driver Go.

**Evidências:**

- `protocol/proto/memorydb/v1/memorydb.proto` — campos `operation_id`.
- `protocol/generated/go/memorydb/v1/memorydb.pb.go` — tipos Protobuf gerados.
- `server/internal/protocol/message.go` — `SetCommand` e `DeleteCommand`.
- `server/internal/codec/protobuf/codec.go` — conversão dos identificadores.
- `server/internal/idempotency/executor.go` — execução, reserva, replay, concorrência, retenção e capacidade.
- `server/internal/idempotency/fingerprint.go` — cálculo do fingerprint.
- `server/internal/idempotency/executor_test.go` — testes do executor.
- `server/internal/idempotency/fingerprint_test.go` — testes de fingerprint.
- `server/internal/transport/tcp/handler.go` — integração do executor ao fluxo de requisições.
- `server/internal/transport/tcp/handler_test.go` — testes de integração do handler com idempotência.
- `server/internal/integration/server_test.go` — testes de integração do servidor.
- `drivers/go/operation_id.go` — tipo `OperationID` e geração de identificadores.
- `drivers/go/client.go` — envio do identificador nas operações mutáveis.
- `drivers/go/client_test.go` — testes do driver.

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

Cenário: Reutilizar identificador após rejeição por capacidade
  Dado que uma operação foi rejeitada porque a capacidade estava esgotada
  E que seu identificador não foi reservado
  Quando houver capacidade disponível
  E a aplicação reenviar a operação com o mesmo OperationID e fingerprint
  Então o servidor deverá poder processar a operação normalmente
```

**Estado:** Implementado, cobertura parcial.

**Observação sobre cobertura:** Os cenários de idempotência descritos possuem cobertura nos testes identificados do servidor e do driver. A sequência ponta a ponta de resultado indeterminado no driver seguida de repetição e recuperação do resultado original não possui teste específico identificado.

**Observação de compatibilidade:** Tornar `OperationID` obrigatório para `SET` e `DELETE` altera o comportamento esperado dos clientes existentes. Os drivers devem ser atualizados de forma coordenada com o servidor.

### FR-013 — Dimensionar e validar recursos no startup

**Objetivo:** permitir que o servidor calcule os limites dos componentes a partir de um perfil de recursos e da capacidade detectada ou explicitamente informada.

**Critérios de aceite**

```gherkin
Cenário: Utilizar o perfil padrão
  Dado que MEMORYDB_RESOURCE_PROFILE não foi definida
  Quando o servidor carregar a configuração de recursos
  Então deverá utilizar o perfil mid

Cenário: Utilizar um preset fixo
  Dado que o operador selecionou low, mid ou high
  Quando a configuração for calculada
  Então os valores do preset deverão ser utilizados
  E a configuração deverá ser rejeitada se exceder a capacidade detectada

Cenário: Dimensionar automaticamente
  Dado que o operador selecionou auto
  Quando o pacote detectar CPU e memória disponíveis
  Então deverá calcular os limites a partir da capacidade detectada

Cenário: Utilizar configuração customizada
  Dado que o operador selecionou custom
  E informou MEMORYDB_RESOURCE_CPU e MEMORYDB_RESOURCE_MEMORY
  Quando a configuração for calculada
  Então deverá utilizar esses valores como entradas do algoritmo comum de dimensionamento
  E rejeitar valores maiores que a capacidade detectada

Cenário: Aplicar overrides
  Dado que o operador informou overrides de limites
  Quando a configuração final for validada
  Então deverá rejeitar valores inválidos ou inconsistentes
  E os quatro orçamentos de memória deverão somar o orçamento do processo

Cenário: Detectar recursos em Linux
  Dado que o servidor está em Linux
  Quando os metadados de cgroups estiverem disponíveis
  Então deverá considerar os limites aplicáveis de CPU e memória do cgroup atual e seus ancestrais
  E comparar os limites com os recursos disponíveis no host

Cenário: Plataforma sem detector
  Dado que o sistema operacional não possui implementação de detecção
  Quando a configuração de recursos for carregada
  Então deverá retornar erro de plataforma não suportada
```

**Estado:** Implementado e testado no pacote `server/resources`, com testes específicos de plataforma.

**Limite:** os orçamentos são controles lógicos por componente, não limites rígidos de heap/RSS ou de CPU aplicados pelo sistema operacional. A detecção Windows ainda não consulta Job Objects.

## 4. Requisitos não funcionais e características verificáveis

Esta seção registra características observáveis da implementação atual. Itens não implementados representam lacunas, não garantias oferecidas pelo sistema.

| ID | Requisito ou característica | Estado | Evidência |
|---|---|---|---|
| NFR-001 | Operações concorrentes de armazenamento devem ser sincronizadas. | Implementado e testado | `storage/go/storage.go`, `storage/go/storage_test.go` |
| NFR-002 | Valores devem ser tratados como bytes, inclusive dados binários. | Implementado | `memorydb.proto`, modelo interno e codec |
| NFR-003 | Frames devem respeitar limite de 1 MiB e codificação Big Endian. | Implementado e testado | `transport/tcp/go/framing.go`, `framing_test.go` |
| NFR-004 | O servidor deve interromper a aceitação, aguardar conexões ativas e fechá-las quando o contexto de shutdown expirar. | Implementado e testado | `server/internal/transport/tcp/server.go`, `server_test.go` |
| NFR-005 | Dados devem sobreviver à reinicialização do processo. | Não implementado | `storage/go/storage.go` |
| NFR-006 | Comunicação deve oferecer criptografia TLS. | Não implementado | `server/cmd/memorydb/main.go` |
| NFR-007 | O servidor deve impor timeouts de leitura/escrita e limites de conexões, requisições em processamento e payload de frame. | Implementado e testado | `server/internal/transport/tcp/handler.go`, `handler_resource_test.go`, `server.go`, `server_test.go` |
| NFR-008 | O projeto deve possuir metas mensuráveis de latência e throughput. | Não identificado | Não foram identificadas metas ou benchmarks de performance |
| NFR-009 | Falhas de processamento de conexões devem ser observáveis em logs. | Não implementado | `server/internal/transport/tcp/server.go` |
| NFR-010 | Execução de comandos, storage, protocolo, codec e transporte devem possuir responsabilidades separadas. | Implementado | Pacotes internos do módulo `server` e módulo independente `storage/go` |
| NFR-011 | O transporte TCP deve ser reutilizável sem dependência direta do Protobuf. | Implementado | `transport/tcp/go`, `drivers/go`, handler do servidor |
| NFR-012 | Aplicações Go devem consumir o serviço por meio de uma API cliente. | Implementado e testado | `drivers/go/client.go`, `drivers/go/client_test.go` |
| NFR-013 | Os módulos Go devem permitir desenvolvimento e testes independentes no workspace. | Implementado | `go.work` e arquivos `go.mod` dos sete módulos |
| NFR-014 | O código deve passar pelos testes com detector de condições de corrida e análise estática. | Validado na execução registrada de 27/11/2026 | Registros por módulo descritos na seção 8; a validação é específica daquela execução |
| NFR-015 | O registro de idempotência deve respeitar capacidade de acordo com o perfil/override, retenção padrão de 24 horas e ausência de evicção antecipada de registros válidos. | Implementado e testado | `server/resources`, `server/internal/idempotency/executor.go`, `executor_test.go` |
| NFR-016 | A execução de operações com o mesmo `OperationID` deve ser sincronizada, evitando execuções concorrentes duplicadas. | Implementado e testado | `server/internal/idempotency/executor.go`, `executor_test.go` |
| NFR-017 | O executor de idempotência deve calcular fingerprints determinísticos para operações mutáveis. | Implementado e testado | `server/internal/idempotency/fingerprint.go`, `fingerprint_test.go` |
| NFR-018 | A configuração de recursos deve ser calculada e validada no startup, respeitando capacidade detectada e consistência dos orçamentos. | Implementado e testado | `server/resources`, `server/cmd/memorydb/resource_validation.go` |
| NFR-019 | Os limites lógicos de Storage, Idempotência e Processamento devem ser aplicados pelos respectivos componentes. | Implementado e testado | `storage/go/storage.go`, `server/internal/idempotency/executor.go`, `server/internal/transport/tcp/handler.go` |

### 4.1. Observações sobre os requisitos não funcionais

**NFR-001 — Sincronização do armazenamento**

A implementação concreta do Storage utiliza `map[string]Entry` protegida por `sync.RWMutex`. As operações de leitura e escrita são sincronizadas. Os métodos de armazenamento também preservam o isolamento dos dados binários por meio da cópia dos slices de bytes.

A implementação está no módulo independente `storage/go`, não mais dentro do módulo `server`.

**NFR-004 — Shutdown**

O servidor fecha o listener e aguarda as conexões ativas. Se o contexto expirar, fecha as conexões restantes e aguarda a conclusão das goroutines dos handlers.

O executável utiliza timeout configurável, com padrão de 10 segundos.

O fechamento das conexões interrompe operações de I/O TCP, mas não equivale ao cancelamento genérico de qualquer operação executada pelo handler.

**NFR-007 — Limites operacionais e de admissão**

O servidor implementa:

- Timeout de leitura TCP, com padrão de 30 segundos.
- Timeout de escrita TCP, com padrão de 10 segundos.
- Limite de conexões TCP definido pelo perfil de recursos (1.024 no perfil `mid`).
- Limite de requisições em processamento (256 no perfil `mid`).
- Limite de payload por frame (256 KiB no perfil `mid`, sem ultrapassar o máximo de 1 MiB do transporte).
- Reserva lógica de processamento antes da alocação do payload.

Os valores podem ser ajustados por variáveis de ambiente:

| Variável | Finalidade | Padrão |
|---|---|---|
| `MEMORYDB_READ_TIMEOUT` | Timeout de leitura TCP | 30 segundos |
| `MEMORYDB_WRITE_TIMEOUT` | Timeout de escrita TCP | 10 segundos |
| `MEMORYDB_MAX_CONNECTIONS` | Número máximo de conexões TCP simultâneas | Depende do perfil; 1.024 em `mid` |
| `MEMORYDB_MAX_IN_FLIGHT_REQUESTS` | Máximo de requisições em processamento | Depende do perfil; 256 em `mid` |
| `MEMORYDB_MAX_FRAME_PAYLOAD_SIZE` | Payload máximo do frame | Depende do perfil; 256 KiB em `mid` |
| `MEMORYDB_RESOURCE_PROFILE` | Perfil de dimensionamento | `mid` |
| `MEMORYDB_SHUTDOWN_TIMEOUT` | Prazo para encerramento do servidor | 10 segundos |

Os timeouts são aplicados individualmente às operações de leitura e escrita, não como um prazo global para toda a vida da conexão.

O limite de conexões representa conexões TCP simultâneas admitidas, não quantidade de requisições ou operações por conexão. Conexões excedentes são fechadas sem serem admitidas para processamento.

Essas proteções não substituem mecanismos ainda não implementados, como timeout de inatividade, autenticação, TLS e observabilidade estruturada de erros.

**NFR-009 — Observabilidade**

A goroutine criada por `Server.Serve()` descarta o erro retornado por `Handler.Handle`. Portanto, falhas individuais de conexão não são registradas de forma estruturada nesse ponto.

O isolamento dessas falhas impede que um erro individual do handler seja propagado como falha do loop de aceitação, mas a ausência de registro dificulta diagnóstico e monitoramento.

**NFR-018 e NFR-019 — Dimensionamento e contabilização de recursos**

O pacote público `server/resources` pertence ao módulo `server` e expõe API Go para detectar recursos, carregar perfis, aplicar overrides e validar a configuração final.

A memória do processo é distribuída em 55% para Storage, 15% para Idempotência, 15% para Processamento e 15% para Runtime. A soma dos quatro orçamentos deve ser igual ao orçamento total do processo.

A contabilização possui semânticas distintas:
- Storage contabiliza bytes lógicos de chave e valor;
- Idempotência contabiliza uma reserva-base estimada e dados variáveis da resposta;
- Processamento reserva memória lógica para frame e buffers antes da alocação do payload;
- Runtime recebe o restante, mas ainda não possui aplicação por `GOMEMLIMIT`.

Portanto, `ProcessMemoryBudget` não é um limite rígido de heap ou RSS. A capacidade de CPU também é utilizada no dimensionamento e validação, mas não configura `GOMAXPROCS` ou limite de CPU do sistema operacional.

No Linux, a detecção considera cgroups v1/v2 e a hierarquia de ancestrais. No Windows, a detecção utiliza recursos do host e ainda não consulta Windows Job Objects.

Consulte `docs/tecnica/RECURSOS.md` para os valores dos perfis e todas as variáveis de ambiente.

**NFR-013 — Organização dos módulos Go**

O workspace é composto por sete módulos independentes:

| Módulo | Diretório | Responsabilidade |
|---|---|---|
| API | `api` | Contratos e tipos públicos compartilhados |
| Driver Go | `drivers/go` | Cliente Go para comunicação com o servidor |
| Protobuf gerado | `protocol/generated/go` | Tipos Go gerados a partir do contrato Protobuf |
| Servidor | `server` | Executável, Engine, protocolo interno, codec, handler, idempotência e configuração do servidor |
| Shared | `shared` | Configurações e definições compartilhadas |
| Storage | `storage/go` | Implementação independente do armazenamento em memória |
| Transporte TCP | `transport/tcp/go` | Framing e abstrações de conexão TCP reutilizáveis |

O módulo Storage possui seu próprio `go.mod` e pode ser desenvolvido e testado como módulo Go independente.

A independência do módulo não implica isolamento de processo: o servidor importa a biblioteca e instancia o Storage no mesmo processo em que executa o Engine.

A interface consumida pelo Engine é definida em `server/internal/engine/storage.go`. A implementação concreta é criada no executável e injetada no Engine.

**NFR-014 — Validação**

A aprovação em testes é uma evidência associada a uma versão do código e a uma execução específica. Não representa garantia permanente para alterações futuras.

A validação histórica de 22/11/2026 e a validação direcionada de 25/11/2026 estão separadas na seção 8.

**NFR-015 a NFR-017 — Idempotência**

A capacidade de 100.000 registros corresponde ao valor do perfil `mid`. O limite pode ser alterado por `MEMORYDB_IDEMPOTENCY_MAX_ENTRIES`; `MEMORYDB_IDEMPOTENCY_CAPACITY` permanece aceito como alias legado. A retenção padrão é de 24 horas e pode ser alterada por `MEMORYDB_IDEMPOTENCY_RETENTION`.

A retenção de 24 horas começa após a conclusão da operação. Registros em andamento não expiram por essa janela.

Os registros de idempotência são mantidos em memória e não oferecem durabilidade após reinicialização.

## 5. Casos de borda e comportamento observado

| Caso | Comportamento atual | Evidência |
|---|---|---|
| Solicitação nula | Retorna `INVALID_REQUEST` no engine | `engine.go`, `engine_test.go` |
| Comando ausente | Retorna `INVALID_REQUEST` | `engine.go`, `engine_test.go` |
| Comando tipado nulo | Rejeitado como requisição inválida | `engine.go`, `engine_test.go` |
| Chave vazia | SET, GET e DELETE rejeitam com `INVALID_ARGUMENT` | `engine.go`, `engine_test.go` |
| TTL maior que zero | SET retorna `UNSUPPORTED_FEATURE` | `engine.go`, `engine_test.go` |
| Valor vazio | Pode ser armazenado; GET informa `found=true` | `storage/go/storage.go`, `storage_test.go` |
| Chave inexistente | GET retorna `found=false`; DELETE retorna `deleted=false` | `engine.go`, `engine_test.go` |
| Substituição de valor | SET substitui o valor anterior | `storage/go/storage_test.go` |
| Mutação de slice pelo chamador | SET e GET copiam bytes | `storage/go/storage.go`, `storage_test.go` |
| Frame com tamanho zero | Rejeitado pelo framing | `framing.go`, `framing_test.go` |
| Frame acima de 1 MiB | Rejeitado pelo framing | `framing.go`, `framing_test.go` |
| Frame truncado | Cabeçalho ou payload incompleto retorna erro | `framing_test.go` |
| Escrita parcial | O framing tenta completar a escrita ou retorna erro | `framing_test.go` |
| Protobuf inválido | Decode retorna erro | `codec.go`, `codec_test.go` |
| Comando ou resultado ausente no codec | Estrutura inválida é rejeitada | `codec.go`, `codec_test.go` |
| Cliente desconecta por EOF | Handler encerra sem tratar EOF como falha de aplicação | `handler.go`, testes de integração |
| Concorrência no storage | Operações são sincronizadas por `RWMutex` | `storage/go/storage.go`, `storage_test.go` |
| Múltiplos clientes | Conexões distintas compartilham o storage da instância | Testes do servidor e integração |
| Desconexão durante operação | Driver propaga falha de comunicação | `client.go`, `client_test.go` |
| Cancelamento antes do I/O | Retorna erro de contexto sem invalidar a conexão | `client.go`, `client_test.go` |
| Cancelamento durante o I/O | Interrompe conexão e sinaliza resultado indeterminado | `client.go`, `client_test.go` |
| Escrita parcial seguida de falha | Driver não presume que a operação deixou de ser executada | `client_test.go` |
| Servidor executa SET, mas resposta não chega | Driver pode retornar resultado indeterminado. Uma repetição com o mesmo OperationID e fingerprint pode recuperar o resultado original enquanto o registro estiver retido; essa sequência completa não possui teste ponta a ponta identificado | `TestClientSetReturnsUnknownOutcomeAfterServerExecution`, testes de idempotência do Executor e Handler |
| Identificador da solicitação | `request_id` correlaciona solicitação e resposta; não deduplica | `memorydb.proto`, `client.go` |
| OperationID ausente ou inválido | SET e DELETE são rejeitados antes da mutação | `handler.go`, `handler_test.go` |
| Mesmo OperationID e mesmo fingerprint | Resultado original é reproduzido sem nova execução | `executor_test.go`, `handler_test.go` |
| Mesmo OperationID e fingerprint diferente | Requisição é rejeitada por conflito | `executor_test.go`, `handler_test.go` |
| Requisições concorrentes com mesmo OperationID | Requisição duplicada aguarda a execução original | `executor_test.go` |
| Capacidade de idempotência esgotada | Novas operações com identificador inédito são rejeitadas antes da execução | `executor_test.go`, `handler_test.go` |
| Registro de idempotência expirado | Identificador deixa de possuir garantia de deduplicação | `executor_test.go` |
| Registro em andamento | Não expira pela janela de retenção de operações concluídas | `executor_test.go` |
| Resposta reproduzida | Mantém o resultado semântico e atualiza o RequestID para a tentativa atual | `executor_test.go`, `handler_test.go` |
| Reinicialização do servidor | Dados e registros de idempotência são perdidos | `storage/go/storage.go`, `idempotency/executor.go` |
| Conexões simultâneas acima do limite | Conexões excedentes são fechadas sem serem admitidas para processamento | `server.go`, `server_test.go` |
| Timeout de leitura | Handler aplica deadline antes de cada leitura de frame | `handler.go`, `handler_test.go` |
| Timeout de escrita | Handler aplica deadline antes de cada escrita de resposta | `handler.go`, `handler_test.go` |
| Shutdown com conexão ativa | Servidor aguarda a conclusão da conexão antes de retornar | `server.go`, `server_test.go` |
| Shutdown com contexto expirado | Servidor fecha conexões ativas e aguarda a conclusão dos handlers | `server.go`, `server_test.go` |
| Nova conexão durante shutdown | Conexão aceita durante a transição para shutdown é fechada, sem iniciar processamento | `server.go`, `server_test.go` |
| Storage como módulo independente | Implementação possui módulo Go próprio, consumido pelo servidor como biblioteca no mesmo processo | `storage/go/go.mod`, `server/go.mod`, `server/cmd/memorydb/main.go` |
| Injeção do Storage | Executável instancia a implementação concreta e a fornece ao Engine por sua interface | `server/internal/engine/storage.go`, `server/internal/engine/engine.go`, `server/cmd/memorydb/main.go` |
| Perfil de recursos ausente | Carregador utiliza `mid` | `server/resources/env.go`, `loader_test.go` |
| Perfil preset acima da capacidade detectada | Carregador rejeita a configuração | `server/resources/loader.go`, `loader_test.go` |
| CPU/memória custom acima da capacidade detectada | Carregador rejeita a configuração | `server/resources/loader.go`, `loader_test.go` |
| Limite de cgroup v2 ilimitado | Detecção utiliza capacidade do host quando não há limite finito | `server/resources/detector_linux.go`, `detector_linux_limits_test.go` |
| Limite de memória cgroup v2 finito | Capacidade disponível considera limite menos uso atual | `server/resources/detector_linux_memory.go`, testes Linux de recursos |
| Variáveis canônica e legada de capacidade idempotente divergentes | Carregador retorna conflito | `server/resources/env.go`, `env_edge_cases_test.go` |
| Soma dos orçamentos de memória inconsistente | Validação rejeita a configuração | `server/resources/validator.go`, `validator_test.go` |

## 6. Rastreabilidade requisito–componente–teste

| Requisito | Componente principal | Testes identificados | Cobertura |
|---|---|---|---|
| FR-001 | Engine | `TestPing` | Coberto |
| FR-002 | Handler / Executor / Engine / Storage | `TestSetAndGet`, `TestSetOverwritesExistingValue` | Coberto para gravação e substituição; validação de OperationID coberta no FR-006 |
| FR-003 | Handler / Executor / Engine | `TestRejectsUnsupportedTTL`, testes de fingerprint | Coberto para rejeição |
| FR-004 | Engine / Storage | `TestGetMissingKey`, `TestEmptyValue`, `TestServerEmptyValueAndMissingKey` | Coberto |
| FR-005 | Handler / Engine / Storage | `TestDelete`, `TestServerSetGetDelete`, testes de idempotência | Coberto |
| FR-006 | Engine / Handler / Executor | `TestRejectsEmptyKey`, `TestRejectsNilRequest`, `TestRejectsMissingCommand`, testes de identificador inválido | Parcial: faltam testes específicos de chave vazia em GET e DELETE |
| FR-007 | Engine / Codec / Executor | `TestPing`, `TestRequestRoundTrip`, `TestResponseRoundTrip`, testes de replay | Coberto |
| FR-008 | Framing TCP | Testes de endian, fragmentação, limites, truncamento e escrita parcial | Coberto |
| FR-009 | Handler / Server / Driver / Executor | `TestHandlerProcessesMultipleRequests`, `TestServerAcceptsMultipleClients`, testes de integração e concorrência do executor | Coberto para conexões distintas e requisições sequenciais |
| FR-010 | Server / Executável | `TestServerShutdownWaitsForActiveConnections`, `TestServerClosesAcceptedConnectionDuringShutdown` e testes de integração | Coberto para espera graciosa e fechamento forçado; fluxo de sinais sem teste específico identificado |
| FR-011 | Driver Go / Executor / Handler | Testes de escrita parcial, execução no servidor com perda da resposta, cancelamento durante leitura e replay no servidor | Parcial: cancelamento e sinalização de resultado indeterminado cobertos; recuperação por repetição validada no servidor, sem teste ponta a ponta com o driver |
| FR-012 | Protobuf / Handler / Executor / Driver Go | `executor_test.go`, `fingerprint_test.go`, `handler_test.go`, `server_test.go`, `client_test.go` e testes de integração | Parcial: cenários de idempotência cobertos; recuperação ponta a ponta após resultado indeterminado sem teste específico identificado |
| FR-013 | `server/resources` / Configuração / Executável | `calculator_test.go`, `loader_test.go`, `env_test.go`, testes de detecção Linux/Windows e `resource_validation_test.go` | Coberto para perfis, cálculo, detecção e validação; caminhos específicos dependem da plataforma de execução |

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

**Referência:** `storage/go/storage.go`.

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

### GAP-006 — Controles operacionais complementares ainda incompletos

A implementação atual oferece controles básicos para limitar o consumo de recursos por conexões TCP:

- Limite de conexões simultâneas, com padrão de 1.024.
- Timeout de leitura, com padrão de 30 segundos.
- Timeout de escrita, com padrão de 10 segundos.

Esses valores são configuráveis por variáveis de ambiente.

Entretanto, ainda não existe timeout específico de inatividade de uma conexão. Os timeouts de leitura e escrita limitam operações individuais de I/O, mas não estabelecem um prazo máximo para toda a permanência de uma conexão.

**Impacto:** uma conexão pode permanecer aberta por tempo indeterminado se continuar realizando operações dentro dos prazos individuais. Os limites atuais não representam uma política completa de controle de utilização por cliente.

**Referências:** `server/internal/config/config.go`, `server/internal/transport/tcp/handler.go`, `server/internal/transport/tcp/server.go`, `shared/config/defaults.go`.

### GAP-007 — Observabilidade de erros de conexão incompleta

O servidor executa `Handler.Handle` em goroutines, mas descarta o erro retornado pelo handler.

**Impacto:** falhas de processamento de conexões não são registradas de maneira estruturada pelo servidor.

**Referência:** `server/internal/transport/tcp/server.go`.

### GAP-008 — Multiplexação de requisições não implementada

O driver Go utiliza sincronização para garantir que uma solicitação seja enviada e sua resposta recebida antes de iniciar a próxima operação na mesma conexão.

**Impacto:** uma conexão não permite múltiplas requisições simultâneas em trânsito, embora o servidor atenda várias conexões simultaneamente.

**Referência:** `drivers/go/client.go`.

### GAP-009 — Durabilidade e recuperação da idempotência não implementadas

A implementação atual oferece idempotência para SET e DELETE por meio de registros mantidos em memória.

Enquanto o registro estiver retido na mesma instância do servidor, uma repetição com o mesmo `OperationID` e fingerprint reproduz o resultado original sem executar novamente a mutação.

Entretanto, os registros de idempotência não são persistidos.

O servidor ainda não oferece:

- Persistência dos registros de idempotência.
- Recuperação desses registros após reinicialização.
- Garantia de deduplicação entre instâncias distintas.
- Consulta independente do resultado por `OperationID`.
- Garantia de deduplicação após a expiração do registro.
- Persistência atômica da mutação e do respectivo registro de idempotência.

**Impacto:** a recuperação de resultados indeterminados depende de a aplicação repetir a operação com o mesmo `OperationID`, parâmetros semânticos idênticos e registro ainda disponível na mesma instância. Após reinicialização ou expiração, não existe garantia de que a operação não será executada novamente.

Uma evolução com persistência deverá tratar atomicamente a mutação e o registro de idempotência para evitar divergências entre o estado dos dados e o estado de deduplicação.

**Referências:** `server/internal/idempotency/executor.go`, `storage/go/storage.go`.

### GAP-010 — Driver Java não implementado e validado

A arquitetura permite evolução para outros drivers, mas não há implementação e validação de driver Java consideradas parte do escopo funcional atual.

**Impacto:** a integração por Java não deve ser considerada requisito implementado.

### GAP-011 — Configuração operacional da idempotência não exposta

**Estado: resolvido na implementação atual.**

A capacidade e a retenção do Executor agora são carregadas pelo pacote `server/resources` e aplicadas durante a composição do servidor.

- `MEMORYDB_IDEMPOTENCY_MAX_ENTRIES` configura o máximo de registros.
- `MEMORYDB_IDEMPOTENCY_CAPACITY` permanece como alias legado.
- `MEMORYDB_IDEMPOTENCY_MEMORY_BUDGET` configura o orçamento lógico.
- `MEMORYDB_IDEMPOTENCY_RETENTION` configura a retenção.

Se os nomes canônico e legado forem informados simultaneamente com valores diferentes, o carregamento retorna erro de conflito.

**Referências:** `server/resources/env.go`, `server/cmd/memorydb/main.go`, `server/internal/idempotency/executor.go`.

### GAP-012 — Limites rígidos de CPU e memória do processo

A configuração de recursos calcula um orçamento de memória para o processo e uma capacidade de CPU, mas não aplica limites globais do runtime ou do sistema operacional.

`Runtime.MemoryBudget` não é aplicado por `GOMEMLIMIT` ou `debug.SetMemoryLimit`. `ProcessMemoryBudget` não representa um limite rígido de heap ou RSS. `CPUCapacity` não altera `GOMAXPROCS`, afinidade ou quota de CPU.

No Windows, a detecção ainda não consulta limites de Windows Job Objects.

**Impacto:** os orçamentos e limites lógicos reduzem e organizam o consumo previsto por componente, mas não garantem que o processo permaneça abaixo de um teto global de memória ou CPU.

**Referências:** `server/resources`, `server/internal/config/config.go`, `server/cmd/memorydb/main.go`.

## 8. Histórico de validação técnica

### 8.1. Validação registrada em 22/11/2026

Em 22/11/2026, foram executados testes, análise estática e testes com detector de condições de corrida nos cinco módulos do workspace existentes naquela versão.

A validação de testes e análise estática utilizou:

```bash
for module in server transport/tcp/go protocol/generated/go drivers/go api; do
    (
        cd "$module" &&
        go test ./... &&
        go vet ./...
    ) || exit 1
done
```

Os resultados foram aprovados em todos os módulos.

A validação com detector de condições de corrida utilizou:

```bash
for module in server transport/tcp/go protocol/generated/go drivers/go api; do
    (
        cd "$module" &&
        go test -race -count=20 ./...
    ) || exit 1
done
```

Os resultados também foram aprovados em todos os módulos.

| Módulo | `go test ./...` | `go vet ./...` | `go test -race -count=20 ./...` |
|---|---|---|---|
| `server` | Aprovado | Aprovado | Aprovado |
| `transport/tcp/go` | Aprovado | Aprovado | Aprovado |
| `protocol/generated/go` | Aprovado — sem testes próprios | Aprovado | Aprovado — sem testes próprios |
| `drivers/go` | Aprovado | Aprovado | Aprovado |
| `api` | Aprovado | Aprovado | Aprovado — sem testes próprios |

A validação também contemplou:

- Testes de concorrência, replay, conflito, capacidade e expiração do executor de idempotência.
- Testes de cálculo de fingerprint.
- Testes de validação de `OperationID` no handler.
- Testes de reprodução de resposta com atualização do `RequestID`.
- Testes de integração das operações mutáveis.
- Testes do driver para resultados indeterminados.
- Testes de cancelamento durante I/O.
- Testes de preservação da causa original do erro.

A formatação foi verificada com `gofmt -l`, sem arquivos pendentes de formatação. `git diff --check` também foi executado sem apontar erros.

**Observação:** os resultados registrados comprovam o estado dos módulos naquela execução, em 22/11/2026. Não representam garantia permanente para alterações futuras.

Como o repositório utiliza um workspace com módulos independentes e não possui um módulo Go na raiz, os comandos de teste devem ser executados nos diretórios de cada módulo. A execução de `go test ./...` diretamente na raiz do workspace não é a forma de validação adotada.

### 8.2. Validação das mudanças de ciclo de vida e timeouts — 25/11/2026

Em 25/11/2026, após a implementação do controle de conexões, timeouts de leitura e escrita e fechamento forçado durante shutdown, foram executados testes direcionados no pacote TCP do servidor.

Comandos executados:

```bash
gofmt -w server/internal/transport/tcp/server_test.go

cd server

go test -count=20 ./internal/transport/tcp \
    -run '^TestServerClosesAcceptedConnectionDuringShutdown$'

go test -count=20 ./internal/transport/tcp

go test -race -count=1 ./internal/transport/tcp

cd ..
```

**Resultado:** todos os comandos executados nessa validação foram aprovados.

Essa validação confirma especificamente os testes repetidos do fechamento de conexões durante shutdown, os testes do pacote de transporte TCP do servidor e a execução desse pacote com detector de condições de corrida.

**Limite da evidência:** essa execução direcionada não substitui uma nova execução integral de todos os módulos do workspace após as mudanças de 25/11/2026. A validação histórica de 22/11/2026 permanece registrada separadamente.

### 8.3. Extração do Storage para módulo independente — 25/11/2026

Em 25/11/2026, a implementação do armazenamento foi extraída do módulo `server` para o módulo Go independente `storage/go`.

A alteração passou a incluir:

- `storage/go/go.mod`, declarando o módulo `github.com/pescador95/memorydb/storage/go`.
- Implementação do armazenamento em `storage/go/storage.go`.
- Tipo de entrada em `storage/go/entry.go`.
- Testes em `storage/go/storage_test.go`.
- Inclusão do módulo no workspace `go.work`.
- Interface consumida pelo Engine em `server/internal/engine/storage.go`.
- Injeção da implementação concreta pelo executável em `server/cmd/memorydb/main.go`.

O Storage continua sendo executado dentro do processo do servidor. A extração estabelece uma fronteira de módulo Go e reutilização de biblioteca, não uma fronteira de processo ou serviço.

**Referência de versão:** commit `6a8d8a3` — `refactor(storage): extraido armazenamento para módulo independente`.

**Limite da evidência:** a existência do módulo e sua integração arquitetural estão registradas no código dessa versão. Os resultados das validações históricas das seções anteriores não devem ser interpretados como execução dos testes do novo módulo, pois ele ainda não existia naquela validação. A validação integral do workspace de sete módulos deve ser registrada separadamente quando executada nessa versão.

### 8.4. Dimensionamento de recursos e pipeline integral — 27/11/2026

Em 27/11/2026, o pipeline completo `./scripts/test.sh` foi executado no WSL Ubuntu após as alterações no pacote `server/resources`.

Todas as etapas concluíram com sucesso:

- formatação;
- análise estática (`go vet`);
- testes unitários;
- testes com detector de condições de corrida;
- compilação;
- cobertura;
- análise de blocos não cobertos.

Cobertura observada:

| Módulo | Cobertura |
|---|---:|
| `api` | 97,6% |
| `drivers/go` | 95,3% |
| `protocol/generated/go` | 0,0% |
| `server` | 95,1% |
| `shared` | 100,0% |
| `transport/tcp/go` | 98,3% |
| `storage/go` | 98,2% |

O pacote `server/resources` apresentou 96,3% de cobertura dentro do módulo `server`. O arquivo Protobuf gerado não possui testes próprios e, por isso, aparece com cobertura zero.

**Escopo da evidência:** a execução comprova o pipeline Linux informado em WSL Ubuntu. Testes protegidos por `//go:build windows` não são executados nessa plataforma. Uma validação Windows deve ser registrada separadamente.

## 9. Critério geral de conclusão do escopo atual

O escopo funcional inicial do MemoryDB é considerado implementado quando:

1. PING, SET, GET e DELETE funcionam conforme os contratos definidos.
2. O engine valida requisições e argumentos.
3. O armazenamento preserva a distinção entre chave ausente e valor vazio.
4. O protocolo Protobuf representa requisições e respostas.
5. O transporte TCP delimita mensagens corretamente.
6. O servidor atende múltiplos clientes e conexões persistentes.
7. O módulo Storage é independente como módulo Go e pode ser consumido como biblioteca.
8. O Engine consome o Storage por interface, com a implementação concreta fornecida por injeção de dependência.
9. O driver Go permite consumir as operações sem implementar manualmente o protocolo.
10. O driver Go oferece operações context-aware e sinaliza resultados indeterminados quando não é possível confirmar o desfecho de uma operação.
11. O servidor oferece encerramento gracioso e forçado, com espera pelas conexões ativas e fechamento das conexões restantes quando o contexto expira.
12. O servidor aplica timeouts de leitura e escrita e limites configuráveis de conexões, requisições em processamento e payload de frame.
13. O pacote público `server/resources` detecta capacidade, calcula perfis, aplica overrides e valida os orçamentos lógicos no startup.
14. SET e DELETE exigem `OperationID` e possuem deduplicação server-side com fingerprint, replay de respostas, controle de concorrência, retenção e capacidade.
15. O driver Go permite gerar e fornecer `OperationID`, sem realizar retries automáticos.
16. A validação automatizada registrada inclui testes, análise estática e testes com detector de condições de corrida nos módulos do workspace, respeitando as datas e os limites de cada execução documentada.

Os GAPs documentados neste arquivo não impedem a conclusão do escopo inicial, pois representam evoluções planejadas e limitações conhecidas da implementação atual.

Este documento não substitui o roadmap:

- `REQUISITOS.md` registra o comportamento implementado, os critérios de aceite e as lacunas.
- `memorydb-roadmap.md` registra as evoluções planejadas.
- `ARQUITETURA.md` registra a organização arquitetural e as dependências.
- `COMPONENTES.md` registra responsabilidades e contratos dos componentes.