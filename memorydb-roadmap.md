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
- Manter os módulos Go independentes, com responsabilidades e contratos explícitos.
- Permitir que a implementação concreta do Storage seja reutilizada por outros executáveis sem transformá-la em um serviço separado.

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
- Idempotência inicial limitada a SET e DELETE, com registros exclusivamente em memória.
- Sem garantia de recuperação de resultados após reinicialização ou perda do processo.
- Sem idempotência para PING e GET.

O servidor pode atender múltiplas conexões simultaneamente. Dentro de cada conexão, entretanto, o processamento das requisições é sequencial.

As operações concorrentes recebidas por uma mesma instância do driver Go são serializadas. O protocolo e o servidor não implementam multiplexação de requisições em uma única conexão.

Os dados são perdidos quando o processo do servidor é encerrado.

Os registros de idempotência também são mantidos exclusivamente em memória e não sobrevivem à reinicialização ou à perda do processo.

A implementação do Storage é um módulo Go independente, mas permanece incorporada ao processo do servidor. Engine e Storage compartilham o mesmo espaço de endereçamento e heap, comunicando-se por chamadas locais, sem RPC, IPC ou tráfego de rede entre esses componentes.

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
Handler TCP
   |
   +--------------------------+
   |                          |
   v                          v
Executor de              Execução normal
idempotência              (PING / GET)
   |
   v
Engine
   |
   | interface Storage
   v
Storage (módulo independente)
```

O Storage é uma biblioteca incorporada ao executável do servidor. A separação em módulo Go permite reutilização e evolução independente, mas não cria uma fronteira de processo ou serviço.

A implementação concreta do Storage fica em `storage/go`. A Engine declara a interface consumidora em `server/internal/engine/storage.go` e recebe a implementação por injeção de dependência.

O driver Java permanece como uma implementação futura, devendo consumir o mesmo contrato de comunicação.

### Responsabilidades

| Componente | Responsabilidade |
|---|---|
| Drivers | Expor uma API de cliente para aplicações consumidoras. |
| TCP Transport | Gerenciar conexões TCP e comunicação de bytes. |
| Framing | Delimitar mensagens transmitidas pelo TCP. |
| Codec | Serializar e desserializar mensagens. |
| Protocol | Representar requisições e respostas no modelo interno. |
| Engine | Validar e executar comandos, consumindo a interface Storage. |
| Storage | Armazenar, consultar e remover dados por meio de uma implementação independente. |
| Idempotency Executor | Controlar a execução de operações mutáveis e reproduzir resultados registrados. |
| Server | Inicializar os componentes e gerenciar o ciclo de vida do servidor. |
| API / CLI | Disponibilizar uma interface de utilização do banco. |

### Regras de desacoplamento

- O Storage não conhece TCP, Protobuf ou drivers.
- A Engine não conhece o formato de serialização nem o transporte.
- A Engine define a interface `Storage` que consome, sem depender do pacote concreto de implementação.
- A implementação concreta do Storage é injetada na Engine durante a inicialização do servidor.
- O Storage é executado no mesmo processo do servidor; sua independência é de módulo e biblioteca, não de serviço.
- O modelo interno de protocolo não depende dos tipos gerados pelo Protobuf.
- O Codec é responsável por converter entre o contrato externo e o modelo interno.
- O transporte TCP trabalha com mensagens em bytes, sem conhecer o significado dos comandos.
- Os drivers implementam o contrato de comunicação sem depender da implementação interna do servidor.
- O mecanismo de idempotência fica no servidor e não deve introduzir dependência de transporte ou serialização no Storage.
- O Executor de idempotência utiliza a Engine para executar as operações protegidas.

## 5. Estrutura atual do repositório

A estrutura abaixo representa os principais diretórios e arquivos da versão revisada. Os testes seguem ao lado dos pacotes de implementação; a listagem omite arquivos auxiliares de teste para manter o diagrama legível.

```text
memorydb/
├── api/
│   ├── go.mod
│   └── client/
│       ├── main.go
│       └── internal/config/
├── docs/
│   ├── README.md
│   ├── requisitos/REQUISITOS.md
│   ├── tecnica/
│   │   ├── ARQUITETURA.md
│   │   ├── COMPONENTES.md
│   │   ├── RECURSOS.md
│   │   └── TESTES.md
│   └── usuario/MANUAL.md
├── drivers/
│   ├── go/
│   │   ├── go.mod
│   │   ├── client.go
│   │   ├── config.go
│   │   └── operation_id.go
│   └── java/
├── protocol/
│   ├── generated/go/
│   │   ├── go.mod
│   │   └── memorydb/v1/memorydb.pb.go
│   ├── proto/memorydb/v1/memorydb.proto
│   └── docs/
├── scripts/
│   ├── generation/
│   │   ├── go.sh
│   │   └── java.sh
│   ├── test/
│   │   ├── common.sh
│   │   ├── format.sh
│   │   ├── vet.sh
│   │   ├── unit.sh
│   │   ├── race.sh
│   │   ├── build.sh
│   │   ├── coverage.sh
│   │   └── uncovered.sh
│   └── test.sh
├── server/
│   ├── go.mod
│   ├── cmd/memorydb/
│   │   ├── main.go
│   │   └── resource_validation.go
│   ├── resources/
│   │   ├── calculator.go
│   │   ├── cgroup_linux.go
│   │   ├── detector_linux.go
│   │   ├── detector_linux_memory.go
│   │   ├── detector_windows.go
│   │   ├── env.go
│   │   ├── loader.go
│   │   ├── profile.go
│   │   └── validator.go
│   └── internal/
│       ├── codec/
│       ├── config/
│       ├── engine/
│       ├── idempotency/
│       ├── integration/
│       ├── protocol/
│       └── transport/tcp/
├── shared/
│   ├── go.mod
│   └── config/
├── storage/go/
│   ├── go.mod
│   ├── entry.go
│   └── storage.go
├── transport/tcp/go/
│   ├── go.mod
│   ├── connection.go
│   └── framing.go
├── .gitattributes
├── .gitignore
├── go.work
├── go.work.sum
└── memorydb-roadmap.md
```

`server/resources` é um pacote Go público dentro do módulo `server`; não é um módulo Go adicional. O pacote oferece API de detecção, cálculo e validação de recursos.

O diretório `server/internal/resources` não faz parte da estrutura atual. A implementação pública está em `server/resources`.

O Storage continua no módulo independente `storage/go`, enquanto sua interface consumidora permanece em `server/internal/engine/storage.go`. Ambos são executados no mesmo processo do servidor.

## 6. Organização dos módulos Go

O projeto utiliza Go Workspace para desenvolvimento local com múltiplos módulos independentes.

| Módulo | Caminho | Responsabilidade |
|---|---|---|
| Protocolo gerado | `protocol/generated/go` | Tipos Go gerados a partir do contrato Protobuf. |
| Transporte TCP | `transport/tcp/go` | Conexão TCP e framing. |
| Driver Go | `drivers/go` | Cliente Go para comunicação com o servidor. |
| Servidor | `server` | Engine, Codec, protocolo interno, idempotência, handlers e inicialização. |
| API | `api` | Aplicação cliente de demonstração. |
| Configurações compartilhadas | `shared` | Valores padrão e configurações compartilhadas entre módulos. |
| Storage Go | `storage/go` | Implementação independente do armazenamento em memória. |

O módulo `storage/go` declara o módulo:

`github.com/pescador95/memorydb/storage/go`

A interface consumida pela Engine está definida em:

`server/internal/engine/storage.go`

A implementação concreta é criada na inicialização do servidor e injetada na Engine. Dessa forma, o módulo Server depende do contrato de Storage, enquanto a implementação concreta permanece fora do módulo Server.

O pacote `shared/config` mantém configurações e valores padrão compartilhados pelo projeto, evitando duplicação de definições comuns.

O arquivo `go.work` reúne os sete módulos para facilitar o desenvolvimento dentro do repositório.

Os módulos continuam independentes e poderão ser publicados e consumidos separadamente no futuro. Consumidores externos não precisam utilizar o `go.work` do repositório para importar os módulos publicados.

A independência do módulo Storage não significa execução isolada: no estado atual, ele é carregado e executado dentro do processo do servidor, por chamadas locais de métodos Go.

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

O servidor aplica deadlines de leitura e escrita às operações de I/O TCP. Esses limites são configuráveis e não representam um timeout global de execução de comandos.

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

O campo `request_id` permite correlacionar requisições e respostas, mas não representa uma chave de idempotência.

As operações SET e DELETE utilizam o campo `operation_id`, representado por bytes e validado para conter exatamente 16 bytes.

O `OperationID` identifica uma operação lógica e permite ao servidor reconhecer tentativas repetidas enquanto o registro correspondente permanecer retido na mesma instância.

O mecanismo não se aplica a PING e GET.

O campo `ttl_ms` já está definido em `SetCommand`, mas sua funcionalidade ainda não foi implementada. Valores positivos são rejeitados pela Engine enquanto o suporte a TTL não estiver disponível.

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
- [x] Extração da implementação para módulo Go independente.
- [x] Definição da interface consumidora na Engine.
- [x] Injeção da implementação concreta do Storage na Engine.

### Fase 2 — Engine e modelo interno

**Status: concluída**

- [x] Definição do modelo interno de requisições e respostas.
- [x] Definição dos comandos.
- [x] Implementação da Engine.
- [x] Integração da Engine com o Storage por meio de interface.
- [x] Validação de comandos.
- [x] Tratamento de comandos não suportados.
- [x] Rejeição de TTL positivo enquanto a funcionalidade não estiver implementada.
- [x] Testes unitários da Engine.
- [x] Injeção de dependência da implementação de Storage.

### Fase 3 — Contrato Protobuf

**Status: concluída**

- [x] Definição do contrato `.proto`.
- [x] Definição de requisições e respostas.
- [x] Definição dos comandos e resultados.
- [x] Inclusão de `request_id`.
- [x] Inclusão de tratamento de erros.
- [x] Inclusão de `operation_id` nas operações SET e DELETE.
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
- [x] Espera pelo encerramento dos handlers associados às conexões ativas.
- [x] Tratamento de sinais do sistema operacional no executável.
- [x] Configuração do endereço por variável de ambiente (`MEMORYDB_ADDR`).
- [x] Configuração de timeout de leitura TCP.
- [x] Configuração de timeout de escrita TCP.
- [x] Limite configurável de conexões TCP simultâneas.
- [x] Registro e acompanhamento das conexões ativas.
- [x] Fechamento forçado das conexões ativas quando o contexto de shutdown expira.
- [x] Espera pelo encerramento dos handlers após o fechamento das conexões.
- [x] Testes de limites de conexões e encerramento do servidor.
- [x] Testes de timeout de leitura e escrita.

O servidor implementa encerramento gracioso e forçado. Durante o shutdown, novas conexões deixam de ser aceitas, o listener é fechado e o servidor aguarda o encerramento dos handlers ativos.

Caso o contexto expire, as conexões TCP ativas são fechadas para interromper operações de I/O bloqueadas, e o servidor aguarda o encerramento das respectivas goroutines.

O fechamento forçado interrompe operações de I/O TCP, mas não representa um mecanismo genérico de cancelamento de qualquer operação interna que possa ser executada futuramente pelo Handler.

O limite de conexões representa a quantidade máxima de conexões TCP simultâneas admitidas pelo servidor. Não representa um limite de operações por conexão nem implementa multiplexação.

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
- [x] Implementação do tipo `OperationID`.
- [x] Geração de identificadores por `NewOperationID()`.
- [x] Atualização de SET e DELETE para receberem `OperationID`.
- [x] Suporte à reutilização do identificador em novas tentativas.
- [x] Compatibilidade com o mecanismo de idempotência do servidor.

O driver Go utiliza o módulo TCP compartilhado, evitando duplicar a implementação de framing.

As operações concorrentes na mesma instância do cliente são serializadas. O driver não implementa multiplexação de requisições em uma mesma conexão.

O `DialContext` utiliza o contexto para estabelecer a conexão inicial. As operações posteriores recebem seus próprios contextos.

O comportamento de cancelamento é diferenciado conforme a fase da operação:

- **Antes da execução:** se o contexto for cancelado enquanto a operação aguarda acesso à conexão, a operação não começa e a conexão permanece disponível.
- **Durante o I/O:** se o cancelamento ocorrer enquanto a requisição ou resposta estiver sendo transmitida, a conexão é encerrada e o cliente passa para o estado `failed`.
- **Resultado indeterminado:** quando não é possível confirmar se o servidor executou a operação, o driver retorna `OperationOutcomeUnknownError`, contendo o `RequestID` e a causa original.

O resultado indeterminado é importante porque o servidor pode executar uma operação, mas a resposta pode ser perdida antes de chegar ao cliente.

O `request_id` permite identificar e correlacionar a operação, mas não permite recuperá-la nem impedir sua execução duplicada.

O driver não implementa retries automáticos. Repetir uma operação após um resultado indeterminado pode executar o comando novamente caso a aplicação não reutilize o mesmo `OperationID`.

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

**Validação de robustez do servidor — 25/11/2026**

Após a implementação de limites de conexões, timeouts de leitura e escrita e encerramento forçado, foram realizadas validações direcionadas:

- Execução do script completo `./scripts/test.sh`, incluindo formatação, análise estática, testes unitários, detector de condições de corrida, build e cobertura.
- Testes repetidos de encerramento de conexões ativas.
- Execução repetida dos testes do pacote TCP.
- Testes do pacote TCP com detector de condições de corrida.

Os comandos direcionados executados incluíram:

```bash
gofmt -w server/internal/transport/tcp/server_test.go

cd server

go test -count=20 ./internal/transport/tcp -run '^TestServerClosesAcceptedConnectionDuringShutdown$'
go test -count=20 ./internal/transport/tcp
go test -race -count=1 ./internal/transport/tcp
```

As validações reportadas concluíram sem falhas.

A validação completa e as validações direcionadas devem ser interpretadas conforme seus respectivos escopos. Os testes direcionados não substituem uma nova execução integral de todos os módulos.

**Validação integral de recursos — 27/11/2026**

Após a implementação do pacote público `server/resources`, foi executado no WSL Ubuntu:

```bash
./scripts/test.sh
```

As etapas `format`, `vet`, `unit`, `race`, `build`, `coverage` e `uncovered` concluíram com sucesso.

Cobertura observada:

| Módulo | Cobertura |
|---|---:|
| `api` | 97,6% |
| `drivers/go` | 95,3% |
| `protocol/generated/go` | 0,0% (código gerado sem testes próprios) |
| `server` | 95,1% |
| `shared` | 100,0% |
| `transport/tcp/go` | 98,3% |
| `storage/go` | 98,2% |

O pacote `server/resources` apresentou 96,3% de cobertura. Essa execução valida o caminho Linux; os testes com build tag Windows precisam de execução específica em Windows.

**Validação da extração do Storage — 25/11/2026**

A extração do armazenamento para módulo independente foi integrada ao workspace no commit `6a8d8a3`.

A alteração contempla:

- [x] Criação do módulo `storage/go`.
- [x] Transferência da implementação concreta do Storage para o novo módulo.
- [x] Transferência dos testes do Storage para o novo módulo.
- [x] Definição da interface consumidora em `server/internal/engine/storage.go`.
- [x] Injeção da implementação concreta na Engine durante a inicialização do servidor.
- [x] Atualização do `go.work` para incluir o novo módulo.
- [x] Atualização dos scripts de teste para considerar o novo módulo.
- [x] Atualização dos testes da Engine e de integração.

A extração preserva a execução local da implementação de Storage dentro do processo do servidor. Não foi introduzida comunicação entre processos ou serviço de armazenamento separado.

### Fase 9 — Consolidação da arquitetura e documentação

**Status: concluída em 25/11/2026 e complementada em 27/11/2026**

A documentação foi revisada e atualizada após a extração do Storage para módulo Go independente, mantendo a distinção entre independência de módulo e isolamento de processo. Em 27/11/2026, foi complementada para descrever os perfis de recursos, a detecção por sistema operacional, os orçamentos lógicos, os limites efetivamente aplicados e as limitações ainda existentes.

**Concluído**

- [x] Revisar a documentação de arquitetura em relação à implementação atual.
- [x] Revisar a documentação dos componentes.
- [x] Documentar os perfis, a detecção, os orçamentos e a API pública de recursos em `docs/tecnica/RECURSOS.md`.
- [x] Atualizar os requisitos funcionais e não funcionais.
- [x] Documentar o contrato de framing TCP.
- [x] Documentar o fluxo de comunicação entre driver e servidor.
- [x] Documentar a utilização do Go Workspace.
- [x] Revisar as decisões arquiteturais e registrar as justificativas relevantes.
- [x] Atualizar o manual de utilização com o contrato de idempotência.
- [x] Atualizar o README do driver Go.
- [x] Atualizar o roadmap com o estado da implementação de idempotência.
- [x] Atualizar o README geral do projeto após a extração do Storage.
- [x] Atualizar o guia de testes para contemplar os sete módulos Go.
- [x] Atualizar o manual de utilização com a organização dos sete módulos.
- [x] Atualizar os requisitos e as evidências de implementação para os novos caminhos do Storage.
- [x] Atualizar a arquitetura e os componentes para refletir a interface consumida pelo Engine e a injeção de dependência.
- [x] Atualizar a estrutura documental e os caminhos de implementação após a extração do Storage.
- [x] Verificar a consistência entre os documentos atualizados.
- [x] Executar `git diff --check` nas alterações documentais.
- [x] Conferir se os documentos descrevem os mesmos valores padrão, contratos e limitações da implementação.

A atualização documental foi registrada no commit `811c403` — `docs: atualizado a documentação após extração do storage`.

A revisão confirmou que os documentos descrevem o Storage como módulo Go independente, reutilizável como biblioteca, mas executado no mesmo processo do servidor. A interface consumida pelo Engine permanece em `server/internal/engine/storage.go`, e a implementação concreta permanece em `storage/go`.

A documentação deve continuar refletindo o comportamento efetivamente implementado, incluindo as limitações atuais do servidor, do transporte, dos drivers e a fronteira entre os módulos Go.

Novas decisões arquiteturais e mudanças de contrato deverão continuar sendo documentadas conforme o projeto evoluir. Essa manutenção contínua não representa uma pendência da consolidação documental concluída nesta fase.

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

O driver deverá permitir que a aplicação consumidora forneça e reutilize o mesmo `OperationID` nas tentativas de uma mesma operação lógica.

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

A semântica do TTL deverá ser compatível com o contrato Protobuf existente, evitando alterações desnecessárias no formato de comunicação.

A integração do TTL com idempotência também deverá preservar o fingerprint semântico das operações SET, considerando o valor de `ttl_ms`.

### Fase 12 — Robustez, confiabilidade e experiência de utilização

**Status: parcialmente concluída**

A fase reúne evoluções operacionais e melhorias de confiabilidade que vão além dos critérios básicos do MVP.

A implementação inicial de idempotência foi concluída. Parte da robustez operacional do servidor também foi implementada, incluindo timeouts de I/O, limites de conexões e encerramento forçado.

Permanecem pendentes observabilidade, melhorias operacionais adicionais, experiência de utilização e evoluções de confiabilidade que dependem de decisões futuras.

#### 12.1. Robustez operacional

**Status: parcialmente concluída**

**Implementado**

- [x] Definir configurações operacionais de leitura e escrita TCP.
- [x] Implementar timeout configurável de leitura.
- [x] Implementar timeout configurável de escrita.
- [x] Implementar limite configurável de conexões simultâneas.
- [x] Implementar encerramento gracioso do servidor.
- [x] Implementar fechamento forçado das conexões ativas quando o contexto de shutdown expira.
- [x] Aguardar o encerramento dos handlers após o fechamento das conexões.
- [x] Implementar testes de limites de conexões e encerramento do servidor.
- [x] Implementar testes de timeout de leitura e escrita.
- [x] Implementar perfis de recursos `low`, `mid`, `high`, `auto` e `custom`.
- [x] Implementar detecção de CPU e memória no Linux, incluindo cgroups v1/v2 e limites herdados.
- [x] Implementar detecção de CPU e memória disponíveis no host Windows.
- [x] Implementar limites lógicos para Storage, Idempotência e Processamento.
- [x] Implementar limites de requisições em processamento e payload máximo de frame.
- [x] Expor configuração de recursos por variáveis de ambiente e API Go pública.
- [x] Validar a soma dos orçamentos e a compatibilidade com os recursos detectados.

**Pendências**

- [ ] Padronizar logs estruturados.
- [ ] Melhorar a observabilidade de erros de processamento no servidor.
- [ ] Revisar e evoluir mensagens e códigos de erro conforme novos requisitos.
- [ ] Avaliar a necessidade de timeout global para operações no servidor.
- [ ] Avaliar proteção contra clientes que mantêm conexões abertas sem enviar requisições completas, considerando os limites já impostos pelo timeout de leitura.
- [ ] Criar testes adicionais de falhas de rede e encerramento inesperado.

**Configurações operacionais implementadas**

| Configuração | Variável de ambiente | Valor padrão |
|---|---|---|
| Endereço de escuta | `MEMORYDB_ADDR` | `:6380` |
| Timeout de leitura TCP | `MEMORYDB_READ_TIMEOUT` | 30 segundos |
| Timeout de escrita TCP | `MEMORYDB_WRITE_TIMEOUT` | 10 segundos |
| Timeout de shutdown | `MEMORYDB_SHUTDOWN_TIMEOUT` | 10 segundos |

O timeout de conexão (`MEMORYDB_DIAL_TIMEOUT`) pertence à configuração do cliente. Os parâmetros da tabela são configurações operacionais do servidor. Os limites de conexões, requisições, frames, Storage e idempotência pertencem ao dimensionamento de recursos e são detalhados em `docs/tecnica/RECURSOS.md`.

A implementação atual oferece controle básico de recursos e tempo de I/O no servidor. Os timeouts de leitura e escrita são aplicados individualmente às operações de I/O TCP, não representando um prazo global de execução de uma operação de negócio.

O timeout de leitura também limita a espera pelo próximo frame. Portanto, conexões sem novas solicitações podem ser encerradas quando esse prazo expira.

O limite de conexões representa o número máximo de conexões TCP simultâneas admitidas. Não representa um limite de operações por conexão, não implementa multiplexação e não substitui limites adicionais de consumo de recursos.

O fechamento forçado durante o shutdown interrompe operações de I/O TCP por meio do fechamento das conexões. Não representa cancelamento genérico de qualquer operação interna que possa ser executada futuramente pelo Handler.

A evolução operacional deverá priorizar previsibilidade, controle de recursos e capacidade de diagnosticar falhas sem introduzir complexidade desnecessária no núcleo do banco.

#### 12.1.1. Dimensionamento e detecção de recursos

**Status: implementação concluída; validação integral Linux registrada em 27/11/2026.**

O pacote público `server/resources` calcula a configuração de recursos no startup e distribui os limites entre os componentes. Ele pertence ao módulo `server`, não constitui um módulo Go separado e não introduz dependências de detecção de hardware nas camadas internas.

**Perfis implementados**

- `low`, `mid` e `high`: presets determinísticos;
- `auto`: utiliza capacidade detectada;
- `custom`: recebe CPU e orçamento de memória explicitamente, respeitando os limites detectados.

**Detecção implementada**

- Linux: `runtime.NumCPU`, `MemAvailable` e limites cgroup v1/v2, incluindo hierarquia de ancestrais;
- Windows: `runtime.NumCPU` e `GlobalMemoryStatusEx`, sem consulta a Windows Job Objects;
- demais plataformas: erro de plataforma não suportada.

São descontadas margens de segurança de 10% para CPU e 20% para memória.

**Distribuição do orçamento de memória**

| Componente | Percentual |
|---|---:|
| Storage | 55% |
| Idempotência | 15% |
| Processamento | 15% |
| Runtime | 15% |

Os limites lógicos de Storage, Idempotência e Processamento são aplicados pelos componentes. A parcela Runtime ainda não é aplicada por `GOMEMLIMIT`; o orçamento total não é limite rígido de heap/RSS. A capacidade de CPU também não altera `GOMAXPROCS` nem aplica uma quota do sistema operacional.

A configuração é calculada uma vez no startup. Não há redimensionamento dinâmico.

A documentação detalhada está em `docs/tecnica/RECURSOS.md`.

#### 12.2. Confiabilidade e idempotência

**Status: implementação inicial concluída em 22/11/2026.**

A implementação inicial de idempotência foi concluída no servidor e no driver Go, conforme o contrato aprovado em 22/11/2026.

O mecanismo protege as operações mutáveis `SET` e `DELETE`, permitindo que uma aplicação repita uma tentativa utilizando o mesmo identificador lógico sem executar novamente uma mutação já registrada.

##### Contrato implementado

| Decisão | Implementação |
|---|---|
| Identificador | `OperationID` de 128 bits (16 bytes) |
| Identificador de requisição | `RequestID` independente, utilizado para correlação |
| Comandos protegidos | `SET` e `DELETE` |
| Escopo | Global, dentro de uma instância do servidor |
| Retenção | 24 horas após a conclusão da operação |
| Capacidade no perfil `mid` | 100.000 registros |
| Armazenamento | Exclusivamente em memória |
| Evicção antecipada | Não permitida para registros válidos |
| Identificador ausente ou inválido | Rejeição de `SET` e `DELETE` |
| Persistência entre reinicializações | Não garantida |

A capacidade, o orçamento lógico e a retenção são configuráveis no startup por meio de `MEMORYDB_IDEMPOTENCY_MAX_ENTRIES`, `MEMORYDB_IDEMPOTENCY_MEMORY_BUDGET` e `MEMORYDB_IDEMPOTENCY_RETENTION`. `MEMORYDB_IDEMPOTENCY_CAPACITY` permanece como alias legado para o limite de entradas.

##### Identificação e fingerprint

O `OperationID` identifica uma operação lógica e deve ser reutilizado em todas as tentativas de execução dessa mesma operação.

O `RequestID` identifica uma requisição específica e deve continuar sendo utilizado para correlacionar a resposta com a tentativa correspondente.

O fingerprint é calculado por SHA-256, considerando a versão do formato e os parâmetros semanticamente relevantes da operação:

- `SET`: comando, chave, valor e `ttl_ms`.
- `DELETE`: comando e chave.

O `OperationID` e o `RequestID` não participam do fingerprint.

##### Comportamento implementado

1. Mesmo `OperationID` e mesmo fingerprint: a operação não é executada novamente; o resultado armazenado é reproduzido.
2. Mesmo `OperationID` e fingerprint diferente: a requisição é rejeitada com erro de conflito.
3. Mesmo `OperationID` recebido simultaneamente: as requisições concorrentes aguardam a execução original e recebem resultado semanticamente equivalente.
4. Respostas reproduzidas utilizam o `RequestID` da tentativa atual, preservando a correlação.
5. Quando a capacidade estiver esgotada, novos identificadores são rejeitados antes da execução, sem expulsar registros válidos e sem reservar o identificador rejeitado.
6. O período de retenção começa após a conclusão da operação. Registros em andamento não expiram por essa janela.
7. Resultados terminais reproduzíveis, incluindo erros determinísticos, são armazenados para replay.
8. Uma reserva somente pode ser liberada quando houver comprovação de que a mutação não foi executada. Resultados indeterminados não podem liberar o identificador para uma nova execução desprotegida.

##### Arquitetura da implementação

O mecanismo foi implementado no servidor, por meio do pacote `server/internal/idempotency`, utilizando um Executor compartilhado entre os handlers da instância.

O Handler direciona `SET` e `DELETE` ao mecanismo de idempotência antes de executar os comandos na Engine. Os demais comandos, como `PING` e `GET`, continuam seguindo o fluxo normal.

O driver Go disponibiliza o tipo `OperationID` e a função `NewOperationID()`, permitindo que a aplicação consumidora gere e forneça o identificador. O driver não gera identificadores automaticamente para cada tentativa e não implementa retries automáticos.

##### Garantias e limitações

A garantia é limitada a, no máximo, uma execução por `OperationID` enquanto o registro correspondente permanecer retido na mesma instância do MemoryDB.

Os registros são mantidos exclusivamente em memória. Portanto, a garantia não sobrevive à reinicialização, ao encerramento inesperado ou à perda do processo.

A implementação não oferece garantia de `exactly-once`. Para proteção após reinicializações, será necessário persistir atomicamente a mutação e o registro de idempotência.

O mecanismo também não oferece recuperação de resultados por consulta independente: a recuperação ocorre por meio da repetição da operação com o mesmo `OperationID` e os mesmos parâmetros.

A implementação inicial de idempotência está concluída para o servidor e o driver Go. A persistência dos registros, a recuperação após reinicializações e o suporte equivalente no driver Java permanecem fora do escopo desta entrega.

#### 12.3. Observabilidade

**Status: planejada**

- [ ] Definir métricas de conexões ativas.
- [ ] Medir quantidade de operações por comando.
- [ ] Medir latência das operações.
- [ ] Medir erros por categoria.
- [ ] Monitorar uso de memória.
- [ ] Avaliar integração com Prometheus.
- [ ] Avaliar instrumentação com OpenTelemetry.
- [ ] Criar dashboards e alertas operacionais.

A observabilidade deverá permitir identificar gargalos, falhas de comunicação, saturação de recursos e comportamento anormal do servidor.

A instrumentação deverá respeitar o desacoplamento arquitetural. O núcleo da Engine e o Storage não deverão depender diretamente de ferramentas específicas de monitoramento.

#### 12.4. Experiência de utilização

**Status: planejada**

- [ ] Melhorar a API pública dos drivers conforme novos requisitos.
- [ ] Avaliar estratégia de reconexão do cliente.
- [ ] Criar exemplos de utilização.
- [ ] Criar ferramenta CLI para interação com o servidor.
- [ ] Documentar configurações e cenários operacionais.

A fase deverá priorizar robustez operacional e previsibilidade.

O suporte a contextos, deadlines, serialização de operações concorrentes, gerenciamento de estados e identificação de resultados indeterminados já faz parte do driver Go e não constitui uma pendência de implementação básica.

A reconexão automática, caso seja considerada futuramente, deverá respeitar os estados internos do cliente, os resultados indeterminados e o contrato de idempotência. Não deverá introduzir retries automáticos sem garantias explícitas.

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

A implementação de novos protocolos ou transportes deverá preservar as regras de desacoplamento e reutilizar os componentes internos sempre que possível.

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

A persistência dos dados e dos registros de idempotência deverá ser tratada de maneira consistente. Para garantir recuperação segura de operações após falhas, será necessário definir como a mutação e seu registro de idempotência serão gravados e recuperados.

A implementação deverá evitar situações em que o banco recupere uma mutação sem seu registro correspondente, ou recupere um registro de idempotência sem a mutação associada.

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

A evolução distribuída também exigirá revisar o escopo da idempotência, que atualmente é limitado a uma única instância do servidor.

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
- Avaliar impactos sobre o contrato de comunicação antes de alterar o schema Protobuf.
- Preservar a distinção entre execução em memória e confirmação durável.
- Não introduzir multiplexação sem definir previamente seu impacto sobre o protocolo, o transporte e os drivers.
- Evitar que mecanismos de observabilidade criem dependências desnecessárias no núcleo do banco.
- Manter a implementação do Storage independente do módulo Server, consumindo-a por meio de uma interface definida pela Engine.
- Não confundir independência de módulo Go com isolamento de processo ou comunicação entre serviços.

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

**Situação atual:** os critérios técnicos do MVP estão atendidos pela implementação e pelas validações realizadas.

A validação integrada dos módulos foi registrada em 15/11/2026, e a evolução posterior do driver Go foi validada separadamente em 22/11/2026. Em 25/11/2026, foram implementadas e validadas evoluções de robustez operacional do servidor, incluindo timeouts, limite de conexões e encerramento forçado.

Em 25/11/2026, a implementação concreta do Storage também foi extraída para um módulo Go independente, mantendo a execução dentro do processo do servidor e a integração por interface e injeção de dependência.

A consolidação documental da arquitetura e da extração do Storage foi concluída e registrada no commit `811c403`. A manutenção da documentação continuará acompanhando as próximas mudanças de implementação, mas não constitui pendência para a conclusão dessa fase.

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
| 9 | Consolidação e documentação | Concluída |
| 10 | Driver Java | Planejada |
| 11 | TTL | Planejada |
| 12 | Robustez, confiabilidade e utilização | Parcialmente concluída; dimensionamento de recursos concluído |
| 13 | Segurança | Planejada |
| 14 | Protocolos alternativos | Planejada |
| 15 | Persistência | Futura |
| 16 | Distribuição e escalabilidade | Futura |