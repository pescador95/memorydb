# Documentação do MemoryDB

## Sumário executivo

MemoryDB é um servidor chave-valor em memória, implementado em Go e organizado como um workspace com módulos independentes. O executável do servidor está em `server/cmd/memorydb/main.go`.

Clientes se comunicam por TCP utilizando frames com prefixo de tamanho e mensagens Protocol Buffers. O framing é implementado de forma independente da serialização, no módulo `transport/tcp/go`, enquanto o contrato externo está definido em `protocol/proto/memorydb/v1/memorydb.proto`.

As operações implementadas são PING, SET, GET e DELETE. O campo `ttl_ms` existe no contrato, mas valores positivos são rejeitados como recurso ainda não suportado.

O armazenamento compartilha dados entre as conexões da mesma instância e protege o mapa contra acesso concorrente. Os valores são mantidos exclusivamente em memória e perdidos quando o processo é encerrado.

A arquitetura separa armazenamento, execução de comandos, protocolo interno, codec, transporte TCP, servidor e drivers. O projeto já possui um driver Go inicial e estrutura para evolução de clientes em outras linguagens.

Não há persistência, autenticação ou TLS implementados. O serviço não deve ser exposto a redes não confiáveis sem mecanismos adicionais de proteção.

## Documentos

- [Arquitetura técnica](./tecnica/ARQUITETURA.md) — visão das camadas, fluxos, contrato TCP/Protobuf, modelo de dados, módulos e dependências.
- [Catálogo de componentes](./tecnica/COMPONENTES.md) — componentes, responsabilidades, interfaces, arquivos e dependências.
- [Requisitos retroativos](./requisitos/REQUISITOS.md) — requisitos funcionais e não funcionais, cenários Gherkin, casos de borda e rastreabilidade para código e testes.
- [Manual do usuário](./usuario/MANUAL.md) — pré-requisitos, inicialização, cliente de demonstração, operações, erros e limitações.

## Estrutura modular

O repositório utiliza um workspace Go definido em `go.work`, composto pelos seguintes módulos:

| Módulo | Responsabilidade |
|---|---|
| `server` | Executável do servidor, engine, storage, codec e integração com TCP. |
| `transport/tcp/go` | Comunicação TCP genérica, conexão e framing, independente de serialização. |
| `protocol/generated/go` | Código Go gerado a partir do contrato Protobuf. |
| `drivers/go` | Biblioteca cliente Go para comunicação com o servidor. |
| `api` | Aplicação cliente de demonstração que utiliza o driver Go. |

A separação permite evoluir o transporte, o contrato de mensagens e os drivers sem acoplar essas responsabilidades diretamente ao armazenamento e à execução dos comandos.

## Lacunas e próximos passos

### P1 — Robustez operacional

- **Tratamento e observabilidade de erros:** o servidor ainda descarta erros retornados por `Handler.Handle`. É necessário definir uma estratégia de registro e observabilidade desses erros.
- **Timeouts e conexões:** implementar timeouts de leitura e escrita, além de avaliar limites de conexões simultâneas e proteção contra clientes lentos.
- **Encerramento gracioso:** avaliar o comportamento de conexões que permanecem ativas durante o shutdown e definir uma política de encerramento forçado após o timeout.

Referências:
- `server/internal/transport/tcp/server.go`
- `server/internal/transport/tcp/handler.go`
- `transport/tcp/go/connection.go`
- `memorydb-roadmap.md`

### P1 — Segurança para implantação

Definir condições seguras de uso antes de disponibilizar o servidor em redes não confiáveis.

Ainda não existem mecanismos de TLS, autenticação ou autorização. Esses recursos precisam ser avaliados e implementados antes de cenários de implantação que exijam proteção das comunicações e controle de acesso.

Referência: `memorydb-roadmap.md`.

### P2 — Completar e estabilizar o contrato externo

Documentar e definir formalmente:

- Limites de tamanho de chaves e valores.
- Códigos e condições de erro.
- Semântica e comportamento esperado de `ttl_ms`.
- Regras de evolução e compatibilidade do schema Protobuf.
- Comportamento esperado diante de mensagens inválidas e versões incompatíveis.

O contrato atual está em `protocol/proto/memorydb/v1/memorydb.proto`, e a execução dos comandos está em `server/internal/engine/engine.go`.

### P2 — Avaliar abstração do armazenamento

Atualmente, o Engine depende diretamente de `*storage.Storage`.

Uma interface de armazenamento poderá ser introduzida quando existir uma necessidade concreta de substituir ou adicionar implementações de backend, como armazenamento persistente ou distribuído.

A abstração deve ser introduzida sem criar complexidade desnecessária para o MVP.

Referências:
- `server/internal/engine/engine.go`
- `server/internal/storage/storage.go`
- `memorydb-roadmap.md`

### P2 — Evoluir os drivers

O driver Go já possui operações básicas e testes para comunicação, concorrência e tratamento de erros.

Próximos passos incluem avaliar:
- Timeouts e cancelamento de operações.
- Reconexão e recuperação de falhas.
- Evolução da API pública do cliente.
- Implementação e validação de drivers em outras linguagens.
- Compatibilidade entre diferentes implementações de clientes.

Referências:
- `drivers/go/`
- `drivers/java/`
- `memorydb-roadmap.md`

### P3 — Persistência e desempenho

Persistência, recuperação de dados e benchmarks devem ser avaliados após a consolidação das funcionalidades essenciais do MVP.

Essas evoluções precisam considerar os impactos sobre o modelo de armazenamento, o contrato externo e a arquitetura modular.

Referência: `memorydb-roadmap.md`.

## Estado da validação

Em 15/11/2026, os seguintes módulos passaram pela execução de testes com detector de condições de corrida e pela análise estática:

| Módulo | `go test -race -count=1 ./...` | `go vet ./...` |
|---|---|---|
| `protocol/generated/go` | Aprovado | Aprovado |
| `transport/tcp/go` | Aprovado | Aprovado |
| `drivers/go` | Aprovado | Aprovado |
| `server` | Aprovado | Aprovado |
| `api` | Aprovado | Aprovado |

A cobertura atual inclui testes de framing, codec Protobuf, engine, storage, handler TCP, servidor, integração e driver Go.

A aprovação desses testes representa o estado validado na data indicada; não significa que as lacunas operacionais, de segurança ou de funcionalidades futuras estejam resolvidas.