# MemoryDB — Roadmap de implementação

## 1. Objetivo

Construir um banco de dados chave-valor em memória, desacoplado do transporte de rede e da serialização dos dados, com implementação inicial em Go e comunicação TCP utilizando Protocol Buffers.

O projeto tem como objetivo principal aprender e implementar os fundamentos de um banco de dados, mantendo uma arquitetura modular que permita adicionar novos protocolos, transportes, linguagens e funcionalidades sem acoplar o núcleo do sistema a essas tecnologias.

## 2. Princípios arquiteturais

- Separar armazenamento, execução de comandos, modelo interno, serialização e transporte.
- Manter o núcleo do banco independente de TCP, Protobuf e de qualquer linguagem cliente.
- Tratar o protocolo de comunicação como um contrato independente da implementação do servidor.
- Permitir que diferentes drivers consumam o mesmo protocolo.
- Priorizar simplicidade, previsibilidade e testes antes de adicionar complexidade.
- Implementar os fundamentos antes de avançar para funcionalidades distribuídas.
- Manter documentação técnica, requisitos e decisões arquiteturais atualizados.
- Tratar falhas de comunicação e resultados indeterminados como situações explícitas do contrato dos clientes.
- Não implementar retries automáticos sem definir previamente garantias de idempotência.
- Diferenciar confirmação de execução em memória de confirmação de persistência durável.

## 3. Escopo do MVP

### Funcionalidades

- `PING`: verificar a disponibilidade do servidor.
- `SET`: armazenar um valor associado a uma chave.
- `GET`: recuperar o valor associado a uma chave.
- `DELETE`: remover uma chave.
- Suporte a chaves do tipo string.
- Suporte a valores binários (`bytes`).
- Diferenciar chave inexistente de chave existente com valor vazio.
- Identificar as requisições por meio de `request_id`.
- Suportar múltiplos clientes conectados simultaneamente.

### Limitações assumidas no MVP

- Armazenamento exclusivamente em memória.
- Uma única instância do servidor.
- Sem persistência em disco.
- Sem replicação ou distribuição de dados.
- Sem autenticação ou autorização.
- Sem implementação de TTL, embora o campo já exista no contrato Protobuf.
- Sem multiplexação de requisições em uma mesma conexão.
- Uma requisição por vez por conexão, com resposta correspondente.
- Sem deduplicação de operações no servidor.
- Sem garantia de recuperação do resultado de uma operação cuja resposta tenha sido perdida.

O servidor pode atender múltiplas conexões simultaneamente. Dentro de cada conexão, entretanto, o processamento das requisições é sequencial.

Os dados são perdidos quando o processo do servidor é encerrado.

## 4. Arquitetura implementada

O fluxo atual de comunicação é:

```text
Cliente
   |
   v
Driver Go
   |
   v
TCP Transport
   |
   v
Framing
   |
   v
Codec (Protobuf)
   |
   v
Modelo interno de protocolo
   |
   v
Engine
   |
   v
Storage
```

O driver Java permanece como uma implementação futura, devendo consumir o mesmo contrato de comunicação.

### Responsabilidades

| Componente | Responsabilidade |
|---|---|
| Drivers | Expor uma API de cliente para aplicações consumidoras. |
| TCP Transport | Gerenciar conexões TCP e comunicação de bytes. |
| Framing | Delimitar mensagens transmitidas pelo TCP. |
| Codec | Serializar e desserializar mensagens. |
| Protocol | Representar requisições e respostas no modelo interno. |
| Engine | Validar e executar comandos. |
| Storage | Armazenar, consultar e remover dados. |
| Server | Inicializar os componentes e gerenciar o ciclo de vida do servidor. |
| API / CLI | Disponibilizar uma interface de utilização do banco. |

### Regras de desacoplamento

- O Storage não conhece TCP, Protobuf ou drivers.
- A Engine não conhece o formato de serialização nem o transporte.
- O modelo interno de protocolo não depende dos tipos gerados pelo Protobuf.
- O Codec é responsável por converter entre o contrato externo e o modelo interno.
- O transporte TCP trabalha com mensagens em bytes, sem conhecer o significado dos comandos.
- Os drivers implementam o contrato de comunicação sem depender da implementação interna do servidor.

## 5. Estrutura atual do repositório

```text
memorydb/
├── .github/
│   ├── copilot_instructions.md
│   └── documentation.md
│
├── api/
│   ├── go.mod
│   └── client/
│       └── main.go
│
├── docs/
│   ├── README.md
│   ├── requisitos/
│   │   └── REQUISITOS.md
│   ├── tecnica/
│   │   ├── ARQUITETURA.md
│   │   └── COMPONENTES.md
│   └── usuario/
│       └── MANUAL.md
│
├── drivers/
│   ├── go/
│   │   ├── client.go
│   │   ├── client_test.go
│   │   ├── go.mod
│   │   └── go.sum
│   └── java/
│
├── protocol/
│   ├── docs/
│   ├── generated/
│   │   └── go/
│   │       ├── go.mod
│   │       └── memorydb/v1/
│   │           └── memorydb.pb.go
│   ├── proto/
│   │   └── memorydb/v1/
│   │       └── memorydb.proto
│   └── scripts/
│
├── scripts/
│   ├── go.sh
│   └── java.sh
│
├── server/
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/
│   │   └── memorydb/
│   │       └── main.go
│   └── internal/
│       ├── codec/
│       │   ├── codec.go
│       │   └── protobuf/
│       │       ├── codec.go
│       │       └── codec_test.go
│       ├── engine/
│       │   ├── command.go
│       │   ├── engine.go
│       │   └── engine_test.go
│       ├── integration/
│       │   └── server_test.go
│       ├── protocol/
│       │   └── message.go
│       ├── storage/
│       │   ├── entry.go
│       │   ├── storage.go
│       │   └── storage_test.go
│       └── transport/
│           └── tcp/
│               ├── communication_test.go
│               ├── handler.go
│               ├── handler_test.go
│               ├── server.go
│               └── server_test.go
│
├── transport/
│   └── tcp/
│       └── go/
│           ├── go.mod
│           ├── connection.go
│           ├── framing.go
│           └── framing_test.go
│
├── go.work
├── .gitignore
└── memorydb-roadmap.md
```

## 6. Organização dos módulos Go

O projeto utiliza Go Workspace para desenvolvimento local com múltiplos módulos independentes.

| Módulo | Caminho | Responsabilidade |
|---|---|---|
| Protocolo gerado | `protocol/generated/go` | Tipos Go gerados a partir do contrato Protobuf. |
| Transporte TCP | `transport/tcp/go` | Conexão TCP e framing. |
| Driver Go | `drivers/go` | Cliente Go para comunicação com o servidor. |
| Servidor | `server` | Engine, Storage, Codec, protocolo interno e inicialização. |
| API | `api` | Aplicação cliente de demonstração. |

O arquivo `go.work` reúne os módulos para facilitar o desenvolvimento dentro do repositório.

Os módulos continuam independentes e poderão ser publicados e consumidos separadamente no futuro. Consumidores externos não precisam utilizar o `go.work` do repositório para importar os módulos publicados.

## 7. Contrato de comunicação

### Transporte TCP

O TCP fornece um fluxo contínuo de bytes, sem preservar os limites das mensagens enviadas pela aplicação.

Para delimitar as mensagens, o MemoryDB utiliza framing com prefixo de tamanho fixo:

- Os primeiros 4 bytes representam o tamanho do payload.
- O tamanho é codificado como `uint32` em Big Endian.
- O prefixo não faz parte do tamanho do payload.
- O tamanho máximo de um frame é de 1 MiB.
- Frames vazios e acima do limite são rejeitados.
- A leitura utiliza `io.ReadFull` para garantir a leitura completa do frame.
- A escrita trata escritas parciais.
- A conexão permite leitura e escrita simultâneas, com sincronização independente para cada direção.
- O fechamento da conexão interrompe operações de I/O bloqueadas.

### Protocol Buffers

O contrato está definido em:

`protocol/proto/memorydb/v1/memorydb.proto`

O contrato contempla:

- `Request`
- `Response`
- `PingCommand`
- `PongResult`
- `SetCommand`
- `SetResult`
- `GetCommand`
- `GetResult`
- `DeleteCommand`
- `DeleteResult`
- `ErrorResult`

O campo `request_id` permite correlacionar requisições e respostas.

O `request_id` não representa uma chave de idempotência. Atualmente, não existe deduplicação de operações no servidor nem garantia de que uma requisição repetida será reconhecida como a mesma operação.

O campo `ttl_ms` já está definido em `SetCommand`, mas sua funcionalidade ainda não foi implementada.

Os arquivos gerados não devem ser editados manualmente. Alterações devem ser realizadas no arquivo `.proto` e posteriormente regeneradas.

## 8. Estado atual da implementação

### Fase 1 — Fundamentos do armazenamento

**Status: concluída**

- [x] Definição do modelo de dados.
- [x] Armazenamento chave-valor em memória.
- [x] Operação de escrita (`SET`).
- [x] Operação de leitura (`GET`).
- [x] Operação de remoção (`DELETE`).
- [x] Diferenciação entre chave inexistente e valor vazio.
- [x] Testes unitários do Storage.

### Fase 2 — Engine e modelo interno

**Status: concluída**

- [x] Definição do modelo interno de requisições e respostas.
- [x] Definição dos comandos.
- [x] Implementação da Engine.
- [x] Integração da Engine com o Storage.
- [x] Validação de comandos.
- [x] Tratamento de comandos não suportados.
- [x] Rejeição de TTL positivo enquanto a funcionalidade não estiver implementada.
- [x] Testes unitários da Engine.

### Fase 3 — Contrato Protobuf

**Status: concluída**

- [x] Definição do contrato `.proto`.
- [x] Definição de requisições e respostas.
- [x] Definição dos comandos e resultados.
- [x] Inclusão de `request_id`.
- [x] Inclusão de tratamento de erros.
- [x] Geração dos tipos Go.
- [x] Separação dos arquivos gerados em módulo próprio.

### Fase 4 — Transporte TCP e framing

**Status: concluída**

- [x] Criação do módulo independente de transporte TCP.
- [x] Implementação do prefixo de tamanho.
- [x] Codificação Big Endian.
- [x] Limite máximo de tamanho dos frames.
- [x] Leitura completa de frames.
- [x] Tratamento de escritas parciais.
- [x] Sincronização de leitura e escrita.
- [x] Gerenciamento de conexões.
- [x] Interrupção de I/O bloqueado pelo fechamento da conexão.
- [x] Validação de argumentos inválidos no framing.
- [x] Testes de framing e comunicação.

### Fase 5 — Codec e integração com o servidor

**Status: concluída**

- [x] Definição da interface de Codec.
- [x] Implementação do Codec Protobuf.
- [x] Conversão entre Protobuf e modelo interno.
- [x] Implementação do Handler TCP.
- [x] Integração entre transporte, Codec, Engine e Storage.
- [x] Processamento sequencial de múltiplas requisições na mesma conexão.
- [x] Tratamento de desconexões.
- [x] Testes unitários do Codec e Handler.

### Fase 6 — Servidor TCP

**Status: concluída**

- [x] Inicialização do servidor.
- [x] Configuração do endereço de escuta.
- [x] Suporte a múltiplas conexões.
- [x] Compartilhamento do Storage entre clientes.
- [x] Execução concorrente de conexões.
- [x] Proteção contra inicialização duplicada.
- [x] Encerramento gracioso com fechamento do listener.
- [x] Espera pelo encerramento das conexões ativas, respeitando o contexto de shutdown.
- [x] Tratamento de sinais do sistema operacional no executável.
- [x] Configuração do endereço por variável de ambiente (`MEMORYDB_ADDR`).

O encerramento gracioso não representa o fechamento forçado de todas as conexões ativas quando o prazo do contexto expira. Essa política permanece como possível evolução operacional.

### Fase 7 — Driver Go

**Status: concluída**

- [x] Criação de módulo independente.
- [x] Implementação de conexão com o servidor.
- [x] Implementação de `Dial`.
- [x] Implementação de `DialContext`.
- [x] Implementação de `New` para construção a partir de uma conexão existente.
- [x] Implementação de `PING`.
- [x] Implementação de `SET`.
- [x] Implementação de `GET`.
- [x] Implementação de `DELETE`.
- [x] Implementação de variantes com `context.Context`.
- [x] Tratamento de erros do servidor.
- [x] Validação do `request_id` nas respostas.
- [x] Tratamento de respostas Protobuf inválidas.
- [x] Sincronização de requisições concorrentes.
- [x] Gerenciamento de encerramento da conexão.
- [x] Definição de estados internos do cliente (`open`, `closed` e `failed`).
- [x] Diferenciação entre erros de aplicação e falhas de transporte ou protocolo.
- [x] Tratamento de cancelamento e deadlines durante operações.
- [x] Representação explícita de resultados indeterminados por `OperationOutcomeUnknownError`.
- [x] Preservação da causa original do erro por meio de `Unwrap`.
- [x] Disponibilização do `RequestID` associado à operação de resultado desconhecido.
- [x] Testes de escrita parcial, perda de resposta, cancelamento e concorrência.
- [x] Testes de corrida e análise estática do módulo.

O driver Go utiliza o módulo TCP compartilhado, evitando duplicar a implementação de framing.

As operações concorrentes na mesma instância do cliente são serializadas. O driver não implementa multiplexação de requisições em uma mesma conexão.

O `DialContext` utiliza o contexto para estabelecer a conexão inicial. As operações posteriores recebem seus próprios contextos.

O comportamento de cancelamento é diferenciado conforme a fase da operação:

- **Antes da execução:** se o contexto for cancelado enquanto a operação aguarda acesso à conexão, a operação não começa e a conexão permanece disponível.
- **Durante o I/O:** se o cancelamento ocorrer enquanto a requisição ou resposta estiver sendo transmitida, a conexão é encerrada e o cliente passa para o estado `failed`.
- **Resultado indeterminado:** quando não é possível confirmar se o servidor executou a operação, o driver retorna `OperationOutcomeUnknownError`, contendo o `RequestID` e a causa original.

O resultado indeterminado é importante porque o servidor pode executar uma operação, mas a resposta pode ser perdida antes de chegar ao cliente.

O `request_id` permite identificar e correlacionar a operação, mas não permite recuperá-la nem impedir sua execução duplicada.

O driver não implementa retries automáticos. Repetir uma operação após um resultado indeterminado pode executar o comando novamente.

### Fase 8 — API cliente e validação integrada

**Status: concluída**

- [x] Criação do módulo `api`.
- [x] Utilização do driver Go pela aplicação cliente.
- [x] Testes de integração entre cliente e servidor.
- [x] Testes de múltiplos clientes.
- [x] Testes de operações concorrentes.
- [x] Testes de desconexão de clientes.
- [x] Testes do encerramento do servidor com conexões ativas.
- [x] Validação manual do fluxo completo de operações.

### Validação técnica

**Validação integrada anterior — 15/11/2026**

Foi realizada uma validação dos módulos do workspace utilizando testes com detector de condições de corrida e análise estática.

Comandos executados:

```bash
for module in protocol/generated/go transport/tcp/go drivers/go server api; do
    echo "========== TESTANDO $module =========="
    (cd "$module" && go test -race -count=1 ./...) || exit 1
    (cd "$module" && go vet ./...) || exit 1
done
```

Também foram executados testes isolados no servidor:

```bash
cd server

go test -race -count=1 ./internal/transport/tcp
go test -race -count=1 ./internal/integration
go vet ./...
```

Todos os comandos reportados nessa validação concluíram sem falhas.

Foi realizada também uma validação manual ponta a ponta, iniciando o servidor e utilizando a aplicação cliente para executar `PING`, `SET`, `GET`, `DELETE` e uma leitura posterior à remoção.

**Validação posterior do driver Go — 22/11/2026**

Após a evolução do tratamento de cancelamento e resultados indeterminados, foram executados:

```bash
go test -v -count=1 ./drivers/go
go test -race -count=1 ./drivers/go
go test -race -count=20 ./drivers/go
go vet ./drivers/go
```

Os testes e a análise estática do módulo `drivers/go` concluíram sem falhas.

Essa validação posterior está restrita ao driver Go. Ela não representa uma nova execução integral dos testes de todos os módulos.

## 9. Próximas fases

### Fase 9 — Consolidação da arquitetura e documentação

**Status: em andamento**

- [x] Revisar a documentação de arquitetura em relação à implementação atual.
- [x] Revisar a documentação dos componentes.
- [x] Atualizar os requisitos funcionais e não funcionais.
- [x] Documentar o contrato de framing TCP.
- [x] Documentar o fluxo de comunicação entre driver e servidor.
- [x] Documentar a utilização do Go Workspace.
- [x] Documentar como executar testes nos módulos.
- [x] Revisar as decisões arquiteturais e registrar as justificativas relevantes.

A documentação deve refletir o comportamento efetivamente implementado, incluindo as limitações atuais do servidor, do transporte e dos drivers.

Novas decisões arquiteturais e mudanças de contrato deverão continuar sendo documentadas conforme o projeto evoluir.

### Fase 10 — Driver Java

**Status: planejada**

- [ ] Definir a estrutura do módulo Java.
- [ ] Gerar as classes Java a partir do contrato Protobuf.
- [ ] Implementar conexão TCP.
- [ ] Implementar framing compatível com o driver Go.
- [ ] Implementar serialização e desserialização Protobuf.
- [ ] Implementar `PING`, `SET`, `GET` e `DELETE`.
- [ ] Implementar tratamento de erros.
- [ ] Definir o gerenciamento de conexões e operações concorrentes.
- [ ] Avaliar suporte a cancelamento e deadlines.
- [ ] Definir como representar resultados indeterminados.
- [ ] Criar testes unitários.
- [ ] Criar testes de integração com o servidor Go.
- [ ] Validar interoperabilidade entre os drivers Go e Java.

O driver Java deverá implementar o mesmo contrato de comunicação, sem depender de detalhes internos da implementação Go.

O tratamento de resultados indeterminados deverá preservar a mesma semântica do driver Go: quando não for possível confirmar a execução de uma operação, o consumidor deverá receber uma indicação explícita, sem retry automático implícito.

### Fase 11 — TTL (Time To Live)

**Status: planejada**

- [ ] Definir formalmente a semântica do TTL.
- [ ] Definir o comportamento para TTL igual a zero.
- [ ] Definir o comportamento para chaves expiradas.
- [ ] Armazenar metadados de expiração nas entradas.
- [ ] Implementar expiração durante operações de leitura.
- [ ] Avaliar mecanismo de limpeza periódica.
- [ ] Garantir segurança em operações concorrentes.
- [ ] Implementar testes de expiração.
- [ ] Expor TTL nos drivers.

A implementação deverá considerar precisão, custo de limpeza e impacto sobre a concorrência do Storage.

### Fase 12 — Robustez, confiabilidade e experiência de utilização

**Status: planejada**

#### 12.1. Robustez operacional

- [ ] Definir e documentar configurações operacionais adicionais.
- [ ] Padronizar logs estruturados.
- [ ] Melhorar observabilidade de erros de processamento no servidor.
- [ ] Revisar e evoluir mensagens e códigos de erro.
- [ ] Avaliar timeouts de conexão e operações no lado do servidor.
- [ ] Avaliar limites de conexões simultâneas.
- [ ] Avaliar limites de tamanho de chave e valor.
- [ ] Definir política para encerramento forçado de conexões durante shutdown.
- [ ] Avaliar proteção contra clientes que mantêm conexões abertas sem enviar requisições completas.
- [ ] Avaliar limites de recursos por conexão.
- [ ] Criar testes de falhas de rede e encerramento inesperado.

#### 12.2. Confiabilidade e idempotência

**Especificação do contrato: aprovada em 22/11/2026.**

**Implementação: planejada.**

O contrato inicial de idempotência foi definido com as seguintes características:

| Decisão | Contrato aprovado |
|---|---|
| Identificador | `OperationID` de 128 bits |
| Identificador de requisição | `RequestID` independente, utilizado para correlação |
| Comandos protegidos | `SET` e `DELETE` |
| Escopo | Global, dentro de uma instância do servidor |
| Retenção | 24 horas após a conclusão da operação |
| Capacidade padrão | 100.000 registros |
| Configuração de capacidade | Configurável na inicialização |
| Armazenamento | Exclusivamente em memória |
| Evicção antecipada | Não permitida para registros ainda válidos |
| Identificador ausente | Rejeitar `SET` e `DELETE` sem `OperationID` |
| Persistência entre reinicializações | Não garantida |

O `OperationID` deve permanecer igual entre tentativas da mesma operação lógica. O `RequestID` deve ser gerado para cada requisição e continuar sendo utilizado para correlacionar a resposta à tentativa correspondente.

O contrato externo deverá representar o `OperationID` como `bytes`, exigindo exatamente 16 bytes. A validação deve rejeitar identificadores ausentes ou com tamanho diferente.

O fingerprint deve considerar o comando e todos os parâmetros semanticamente relevantes. Para os comandos iniciais:

- `SET`: comando, chave, valor e parâmetros semânticos adicionais, incluindo `ttl_ms`;
- `DELETE`: comando, chave e parâmetros semânticos adicionais, caso existam.

O comportamento para identificadores repetidos será:

1. Mesmo `OperationID` e mesmo fingerprint: não executar novamente; reproduzir o resultado armazenado.
2. Mesmo `OperationID` e fingerprint diferente: rejeitar a requisição por conflito.
3. Mesmo `OperationID` recebido simultaneamente: as requisições concorrentes devem aguardar a execução original e receber resultado semanticamente equivalente.
4. Respostas reproduzidas devem utilizar o `RequestID` da tentativa atual, preservando a correlação da requisição, enquanto o resultado da operação permanece equivalente ao resultado armazenado.

A capacidade máxima contabiliza registros em andamento e concluídos. Quando a capacidade estiver esgotada, uma operação com `OperationID` inédito deve ser rejeitada com erro específico, sem executar a mutação e sem expulsar registros válidos. A tentativa rejeitada por capacidade não reserva o identificador; poderá ser repetida posteriormente.

O período de retenção começa quando a operação alcança um resultado terminal. Registros em andamento não expiram por essa janela. Resultados estruturados e reproduzíveis, inclusive rejeições determinísticas de aplicação, devem ser armazenados para replay.

Uma reserva só pode ser liberada após uma falha quando houver comprovação de que a mutação não foi executada. Se o resultado da execução for indeterminado, o registro não pode ser liberado de forma que permita executar novamente a mesma operação sem proteção.

A garantia inicial será limitada a **no máximo uma execução por `OperationID` enquanto o respectivo registro permanecer retido na mesma instância do MemoryDB**. Como os registros são voláteis, a garantia não sobrevive a reinicialização, encerramento inesperado ou perda do processo.

Essa implementação não deve ser denominada *exactly-once*. Para oferecer proteção após reinicialização, será necessário persistir atomicamente a mutação e o registro de deduplicação.

#### 12.2.1. Etapas de implementação

- [x] Definir a diferença entre `request_id` e `operation_id`.
- [x] Definir os comandos inicialmente protegidos.
- [x] Definir o escopo e a representação do identificador.
- [x] Definir o comportamento para identificadores repetidos com o mesmo conteúdo.
- [x] Definir o comportamento para identificadores repetidos com conteúdo diferente.
- [x] Definir o tratamento de requisições concorrentes.
- [x] Definir retenção, capacidade e política de não evicção antecipada.
- [x] Definir o comportamento quando a capacidade estiver esgotada.
- [x] Definir a limitação de durabilidade dos registros em memória.
- [ ] Atualizar o schema Protobuf e os tipos gerados.
- [ ] Atualizar o modelo interno de protocolo.
- [ ] Implementar o gerenciador de idempotência.
- [ ] Integrar o gerenciador ao fluxo de execução do servidor.
- [ ] Implementar códigos de erro para identificador inválido, conflito e capacidade esgotada.
- [ ] Implementar testes de replay de resultados.
- [ ] Implementar testes de conflito de fingerprint.
- [ ] Implementar testes de concorrência para identificadores iguais.
- [ ] Implementar testes de expiração e capacidade.
- [ ] Atualizar o driver Go para receber e reutilizar `OperationID`.
- [ ] Testar a recuperação de uma operação após perda de resposta, reutilizando o mesmo `OperationID`.
- [ ] Documentar a semântica pública e os limites da garantia.

O driver não deverá gerar um novo `OperationID` automaticamente a cada tentativa de recuperação. A aplicação consumidora deverá poder fornecer e reutilizar o identificador associado à mesma operação lógica.

O driver continuará sem retry automático implícito. A evolução permitirá que a aplicação decida repetir uma operação utilizando o mesmo `OperationID`.

A implementação deverá preservar a distinção entre operação não iniciada, operação rejeitada e resultado indeterminado. Não liberar reservas quando não houver comprovação de que a mutação deixou de ocorrer.

#### 12.3. Observabilidade

- [ ] Definir métricas de conexões ativas.
- [ ] Medir quantidade de operações por comando.
- [ ] Medir latência das operações.
- [ ] Medir erros por categoria.
- [ ] Monitorar uso de memória.
- [ ] Avaliar integração com Prometheus.
- [ ] Avaliar instrumentação com OpenTelemetry.
- [ ] Criar dashboards e alertas operacionais.

A observabilidade deverá permitir identificar gargalos, falhas de comunicação, saturação de recursos e comportamento anormal do servidor.

#### 12.4. Experiência de utilização

- [ ] Melhorar a API pública dos drivers conforme novos requisitos.
- [ ] Avaliar estratégia de reconexão do cliente.
- [ ] Criar exemplos de utilização.
- [ ] Criar ferramenta CLI para interação com o servidor.
- [ ] Documentar configurações e cenários operacionais.

A fase deverá priorizar robustez operacional e previsibilidade. O suporte a contextos, deadlines, serialização de operações concorrentes, gerenciamento de estados e identificação de resultados indeterminados já faz parte do driver Go atual e não constitui uma pendência de implementação básica.

### Fase 13 — Segurança

**Status: planejada**

- [ ] Implementar suporte a TLS.
- [ ] Definir autenticação de clientes.
- [ ] Definir autorização de operações.
- [ ] Avaliar isolamento por usuário ou tenant.
- [ ] Definir política de rotação de credenciais.
- [ ] Avaliar proteção contra abuso de conexões e requisições.
- [ ] Documentar configurações e requisitos de segurança.

TLS deverá ser a camada responsável pela proteção criptográfica da comunicação. SSH poderá ser considerado futuramente para administração ou criação de túneis, mas não será tratado como substituto do TLS.

### Fase 14 — Protocolos e transportes alternativos

**Status: planejada**

- [ ] Avaliar suporte a outros protocolos de serialização.
- [ ] Avaliar suporte a novos transportes.
- [ ] Avaliar um adaptador compatível com Redis RESP.
- [ ] Separar a semântica de comandos do protocolo externo.
- [ ] Mapear comandos externos para o modelo interno.
- [ ] Criar testes de compatibilidade.

Um adaptador RESP deverá converter os comandos Redis para operações da Engine, sem introduzir dependência do RESP no núcleo do banco.

### Fase 15 — Persistência

**Status: futura**

- [ ] Avaliar mecanismos de persistência.
- [ ] Definir formato de armazenamento em disco.
- [ ] Avaliar snapshots.
- [ ] Avaliar log de operações (WAL).
- [ ] Definir recuperação após falhas.
- [ ] Definir consistência entre memória e disco.
- [ ] Definir política de confirmação de escrita.
- [ ] Diferenciar confirmação de escrita em memória e confirmação durável.
- [ ] Avaliar integridade dos registros e recuperação de operações incompletas.
- [ ] Criar testes de recuperação.

A persistência deverá ser projetada considerando a relação entre execução de comandos, armazenamento em memória, gravação em disco e confirmação enviada ao cliente.

A semântica de confirmação deverá deixar explícito se uma operação foi apenas aplicada em memória ou se também foi registrada de forma durável.

### Fase 16 — Distribuição e escalabilidade

**Status: futura**

- [ ] Estudar particionamento de dados.
- [ ] Estudar replicação.
- [ ] Definir estratégia de consistência.
- [ ] Avaliar descoberta de nós.
- [ ] Avaliar coordenação entre instâncias.
- [ ] Avaliar balanceamento de requisições.
- [ ] Avaliar tolerância a falhas.
- [ ] Criar testes distribuídos.

Essa fase depende de decisões específicas de consistência, disponibilidade e particionamento. Não faz parte do MVP.

## 10. Diretrizes de desenvolvimento

- Implementar uma etapa por vez.
- Evitar antecipar funcionalidades de fases futuras.
- Manter os módulos independentes.
- Não introduzir dependências desnecessárias no núcleo.
- Criar testes para comportamentos novos e casos de falha.
- Executar testes com `-race` nas alterações relacionadas à concorrência.
- Executar `go vet` para verificar problemas estáticos.
- Atualizar a documentação quando uma decisão arquitetural ou contrato mudar.
- Não editar manualmente arquivos gerados pelo Protobuf.
- Preservar compatibilidade entre os drivers e o contrato de comunicação.
- Registrar limitações conhecidas em vez de implementar comportamentos implícitos.
- Separar funcionalidades implementadas de evoluções planejadas.
- Validar alterações nos módulos afetados e, quando necessário, executar a suíte completa do workspace.
- Não introduzir retries automáticos sem garantias de idempotência documentadas.
- Não tratar `request_id` como identificador de deduplicação.
- Diferenciar erros de aplicação, falhas de transporte, falhas de protocolo e resultados indeterminados.
- Preservar a rastreabilidade entre requisitos, implementação, testes e documentação.

## 11. Critérios de conclusão do MVP

O MVP será considerado concluído quando:

- [x] O servidor inicializar e aceitar conexões TCP.
- [x] O protocolo de framing funcionar corretamente.
- [x] A serialização Protobuf funcionar.
- [x] A Engine executar os comandos básicos.
- [x] O Storage manter os dados enquanto o processo estiver ativo.
- [x] O driver Go executar todas as operações previstas.
- [x] Múltiplos clientes compartilharem o mesmo armazenamento.
- [x] O encerramento gracioso do servidor funcionar.
- [x] Os testes unitários e de integração passarem.
- [x] Os módulos passarem pelo `go test -race`.
- [x] Os módulos passarem pelo `go vet`.

**Situação atual:** os critérios técnicos do MVP estão atendidos pela implementação e pelas validações realizadas. A validação integrada dos módulos foi registrada em 15/11/2026, e a evolução posterior do driver Go foi validada separadamente em 22/11/2026.

A consolidação documental permanece em andamento até a conclusão da revisão do manual e das instruções de desenvolvimento.

O projeto pode avançar para as próximas evoluções sem depender de novas funcionalidades para considerar o MVP atual concluído.

## 12. Visão geral do progresso

| Fase | Descrição | Situação |
|---|---|---|
| 1 | Storage | Concluída |
| 2 | Engine e modelo interno | Concluída |
| 3 | Contrato Protobuf | Concluída |
| 4 | TCP e framing | Concluída |
| 5 | Codec e integração | Concluída |
| 6 | Servidor TCP | Concluída |
| 7 | Driver Go | Concluída |
| 8 | API e integração | Concluída |
| 9 | Consolidação e documentação | Em andamento |
| 10 | Driver Java | Planejada |
| 11 | TTL | Planejada |
| 12 | Robustez, confiabilidade e utilização | Planejada |
| 13 | Segurança | Planejada |
| 14 | Protocolos alternativos | Planejada |
| 15 | Persistência | Futura |
| 16 | Distribuição e escalabilidade | Futura |