# Documentação do MemoryDB

## Sumário executivo

MemoryDB é um servidor chave-valor em memória, implementado em Go e organizado como um workspace com módulos independentes. O executável do servidor está em `server/cmd/memorydb/main.go`.

Clientes se comunicam por TCP utilizando frames com prefixo de tamanho e mensagens Protocol Buffers. O framing é implementado de forma independente da serialização, no módulo `transport/tcp/go`, enquanto o contrato externo está definido em `protocol/proto/memorydb/v1/memorydb.proto`.

As operações implementadas são PING, SET, GET e DELETE. O campo `ttl_ms` existe no contrato, mas valores positivos são rejeitados como recurso ainda não suportado.

O armazenamento compartilha dados entre as conexões da mesma instância e protege o mapa contra acesso concorrente. Os valores são mantidos exclusivamente em memória e perdidos quando o processo é encerrado.

A arquitetura separa armazenamento, execução de comandos, protocolo interno, codec, transporte TCP, servidor e drivers. O projeto possui um driver Go com operações básicas, suporte a `context.Context`, serialização de operações concorrentes e tratamento de falhas de comunicação e protocolo.

O driver também identifica situações em que uma operação pode ter sido executada pelo servidor, mas sua resposta não foi confirmada pelo cliente. Nesses casos, retorna `OperationOutcomeUnknownError`, preservando o identificador da solicitação e a causa da falha.

O servidor atende múltiplos clientes simultaneamente. Entretanto, o driver Go executa uma operação por vez em cada conexão, sem multiplexação de requisições simultâneas.

Não há persistência, autenticação, autorização, isolamento por tenant ou TLS implementados. O serviço não deve ser exposto a redes não confiáveis sem mecanismos adicionais de proteção.

## Documentos

- [Arquitetura técnica](./tecnica/ARQUITETURA.md) — visão das camadas, fluxos, contrato TCP/Protobuf, modelo de dados, módulos e dependências.
- [Catálogo de componentes](./tecnica/COMPONENTES.md) — componentes, responsabilidades, interfaces, arquivos e dependências.
- [Requisitos e critérios de aceite](./requisitos/REQUISITOS.md) — requisitos funcionais e não funcionais, cenários Gherkin, casos de borda e rastreabilidade para código e testes.
- [Manual do usuário](./usuario/MANUAL.md) — pré-requisitos, inicialização, cliente de demonstração, operações, utilização do driver Go, erros e limitações.

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

Cada módulo possui seu próprio `go.mod`. O workspace permite desenvolver e testar os módulos locais de forma integrada, sem exigir que estejam publicados como dependências externas.

# Lacunas e próximos passos

As prioridades abaixo representam evoluções possíveis da implementação atual. Não indicam funcionalidades já disponíveis nem estabelecem, por si só, uma ordem definitiva de implementação.

## P1 — Robustez operacional

### Tratamento e observabilidade de erros

O servidor ainda descarta erros retornados por `Handler.Handle`. É necessário definir uma estratégia de registro e observabilidade desses erros.

A evolução deve considerar:

- Registro estruturado de falhas de leitura, escrita e processamento.
- Diferenciação entre encerramentos esperados e falhas de conexão.
- Informações suficientes para diagnóstico sem expor dados sensíveis.
- Métricas de erros e encerramentos de conexão.

### Timeouts e conexões

Implementar políticas configuráveis de timeout de leitura e escrita no servidor, além de avaliar:

- Limites de conexões simultâneas.
- Tempo máximo de inatividade.
- Proteção contra clientes lentos.
- Comportamento diante de conexões que não concluem o envio de um frame.

O driver Go já oferece operações baseadas em `context.Context`, permitindo controlar o cancelamento e o prazo das operações no lado cliente. Isso não substitui políticas operacionais de timeout e proteção no servidor.

### Encerramento gracioso

Avaliar o comportamento de conexões que permanecem ativas durante o shutdown e definir uma política de encerramento forçado após o timeout.

A implementação atual fecha o listener e aguarda as conexões ativas, mas não força seu fechamento quando o contexto expira.

Referências:

- `server/internal/transport/tcp/server.go`
- `server/internal/transport/tcp/handler.go`
- `transport/tcp/go/connection.go`
- `memorydb-roadmap.md`

## P1 — Segurança para implantação

Definir condições seguras de uso antes de disponibilizar o servidor em redes não confiáveis.

Ainda não existem mecanismos de:

- TLS para proteção da comunicação.
- Autenticação para identificação de clientes.
- Autorização para controle das operações permitidas.

Esses recursos precisam ser avaliados e implementados antes de cenários de implantação que exijam proteção das comunicações e controle de acesso.

Também deverá ser avaliada a necessidade de isolamento lógico dos dados por cliente ou tenant.

Referência: `memorydb-roadmap.md`.

## P2 — Completar e estabilizar o contrato externo

Documentar e definir formalmente:

- Limites de tamanho de chaves e valores.
- Códigos e condições de erro.
- Semântica e comportamento esperado de `ttl_ms`.
- Regras de evolução e compatibilidade do schema Protobuf.
- Comportamento esperado diante de mensagens inválidas e versões incompatíveis.

O contrato atual está em `protocol/proto/memorydb/v1/memorydb.proto`, e a execução dos comandos está em `server/internal/engine/engine.go`.

A evolução do contrato deve preservar a compatibilidade entre o servidor e os drivers sempre que possível.

## P2 — Avaliar abstração do armazenamento

Atualmente, o Engine depende diretamente de `*storage.Storage`.

Uma interface de armazenamento poderá ser introduzida quando existir uma necessidade concreta de substituir ou adicionar implementações de backend, como armazenamento persistente ou distribuído.

A abstração deve ser introduzida sem criar complexidade desnecessária para o MVP.

Referências:

- `server/internal/engine/engine.go`
- `server/internal/storage/storage.go`
- `memorydb-roadmap.md`

## P2 — Evoluir os drivers e a confiabilidade das operações

O driver Go possui operações básicas, suporte a `context.Context`, serialização de operações concorrentes e tratamento de falhas de comunicação e protocolo.

Próximos passos incluem avaliar:

- Reconexão e recuperação de falhas.
- Evolução e estabilização da API pública do cliente.
- Implementação e validação de drivers em outras linguagens.
- Compatibilidade entre diferentes implementações de clientes.
- Estratégias para lidar com resultados desconhecidos após interrupções de comunicação.
- Mecanismos de idempotência e deduplicação no servidor.

O cancelamento de operações durante a comunicação pode invalidar a conexão, pois o resultado da operação no servidor pode ser desconhecido. O driver sinaliza essa condição por meio de `OperationOutcomeUnknownError`.

O `request_id` atual permite correlacionar solicitações e respostas, mas não funciona como identificador de idempotência. O servidor ainda não possui deduplicação, registro de operações executadas ou recuperação de resultados.

A reconexão automática e a repetição transparente de comandos não fazem parte da implementação atual. Uma futura estratégia de retry deverá considerar a semântica de idempotência para evitar execuções duplicadas.

Referências:

- `drivers/go/`
- `protocol/proto/memorydb/v1/memorydb.proto`
- `server/internal/engine/`
- `memorydb-roadmap.md`

## P3 — Persistência e desempenho

Persistência, recuperação de dados e benchmarks devem ser avaliados após a consolidação das funcionalidades essenciais do MVP.

Essas evoluções precisam considerar os impactos sobre:

- O modelo de armazenamento.
- A consistência e a durabilidade dos dados.
- O contrato externo.
- A arquitetura modular.
- O comportamento concorrente.
- A recuperação após falhas.
- A relação entre confirmação de escrita e durabilidade.

A implementação atual não possui persistência em disco nem metas mensuráveis de latência e throughput.

Referência: `memorydb-roadmap.md`.

## Validação técnica

Na validação registrada em 15/11/2026, foram executados os testes automatizados com detector de condições de corrida e a análise estática nos módulos do workspace:

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

Os testes automatizados abrangem, entre outros aspectos:

- Operações do engine e do storage.
- Concorrência no armazenamento.
- Codificação e decodificação Protobuf.
- Framing TCP, fragmentação, limites e escritas parciais.
- Atendimento de múltiplas conexões.
- Compartilhamento de armazenamento entre clientes.
- Operações e concorrência no driver Go.
- Cancelamento, deadlines e falhas de comunicação.
- Resultados indeterminados após falhas de comunicação.
- Encerramento do servidor.

Todos os comandos mencionados terminaram sem falhas nas respectivas execuções registradas.

**Observação:** a validação completa de 15/11 comprova o estado dos módulos naquela execução. A validação posterior comprova os cenários adicionais do driver Go, mas não substitui uma nova execução completa de todos os módulos após as alterações. Alterações futuras no código também exigem nova execução para manter essas evidências atualizadas.

## Validação ponta a ponta

Em 15/11/2026, foi executado um teste manual de integração utilizando o servidor real e a aplicação cliente de demonstração.

Fluxo validado:

1. Estabelecimento da conexão TCP.
2. Execução de PING.
3. Armazenamento de uma chave por SET.
4. Recuperação do valor por GET.
5. Remoção da chave por DELETE.
6. Nova consulta por GET, confirmando que a chave não existe mais.

Resultado observado:

```text
MemoryDB conectado com sucesso!
SET: chave "user:1" armazenada
GET: memorydb
DELETE: chave removida = true
GET após DELETE: chave encontrada = false
```

Essa execução valida a integração entre o servidor, o transporte TCP, o protocolo Protobuf e o driver Go.

Trata-se de uma verificação manual complementar aos testes automatizados, não de uma substituição deles.

## Estado geral do projeto

A implementação atual contempla o núcleo funcional do MemoryDB:

- Servidor TCP com conexões persistentes.
- Protocolo Protobuf com contrato definido.
- Framing independente da serialização.
- Engine com PING, SET, GET e DELETE.
- Storage em memória com proteção concorrente.
- Driver Go com suporte a contextos.
- Identificação de resultados indeterminados no driver.
- Atendimento concorrente de múltiplos clientes.
- Shutdown gracioso com contexto.
- Testes automatizados e validação ponta a ponta.

As lacunas descritas neste documento representam evoluções futuras e limitações conhecidas. Elas não devem ser confundidas com funcionalidades já implementadas.

O roadmap registra o planejamento de desenvolvimento; esta documentação apresenta a arquitetura, o comportamento e o estado verificável da implementação atual.