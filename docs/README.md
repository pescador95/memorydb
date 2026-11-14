# Documentação do MemoryDB

## Sumário executivo

MemoryDB é um servidor chave-valor em memória implementado em Go; sua composição está em [cmd/memorydb/main.go](../cmd/memorydb/main.go).
Clientes se comunicam por TCP usando frames com prefixo de tamanho e mensagens Protocol Buffers ([framing.go](../internal/transport/tcp/framing.go), [memorydb.proto](../api/proto/memorydb.proto)).
As operações implementadas são PING, SET, GET e DELETE; SET com TTL positivo é rejeitado como recurso não suportado ([engine.go](../internal/engine/engine.go)).
O storage compartilha dados entre conexões da mesma instância e protege o mapa contra acesso concorrente ([storage.go](../internal/storage/storage.go)).
Não há persistência, autenticação ou TLS implementados; os dados são perdidos ao reiniciar o processo ([storage.go](../internal/storage/storage.go), [server.go](../internal/transport/tcp/server.go)).

## Documentos

- [Arquitetura técnica](./tecnica/ARQUITETURA.md) — visão de camadas, fluxos, contrato TCP/Protobuf, modelo de dados, dependências e desvios arquiteturais.
- [Catálogo de componentes](./tecnica/COMPONENTES.md) — componentes, camadas, tipos, arquivos, responsabilidades e dependências.
- [Requisitos retroativos](./requisitos/REQUISITOS.md) — FR/NFR, Gherkin, bordas, lacunas e rastreabilidade para código/testes.
- [Manual do usuário](./usuario/MANUAL.md) — inicialização, cliente de demonstração, operações, erros e limitações.

## Lacunas priorizadas

1. **P1 — Automatizar e consolidar integração ponta a ponta:** validar PING/SET/GET/DELETE, múltiplas conexões e desconexões com listener TCP real; executar `go test ./...` e `go test -race ./...`. A pendência está registrada no [roadmap](../memorydb-roadmap.md).
2. **P1 — Melhorar robustez operacional:** não descartar erros de `Handler.Handle`, adicionar timeouts de leitura/escrita e avaliar limite de conexões/clientes lentos. Evidências: [server.go](../internal/transport/tcp/server.go), [connection.go](../internal/transport/tcp/connection.go) e [roadmap](../memorydb-roadmap.md).
3. **P1 — Definir segurança para implantação:** implementar ou documentar condições seguras de uso antes de expor o TCP a redes não confiáveis; TLS, autenticação e autorização não existem atualmente. Evidência: [roadmap](../memorydb-roadmap.md).
4. **P2 — Completar contrato externo:** documentar limites de chaves/valores, condições dos erros, semântica de `ttl_ms` e regras de evolução/compatibilidade do Protobuf. O campo TTL existe, mas a execução está pendente em [engine.go](../internal/engine/engine.go) e [memorydb.proto](../api/proto/memorydb.proto); tarefas constam no [roadmap](../memorydb-roadmap.md).
5. **P2 — Introduzir uma Port de armazenamento se houver necessidade de trocar backend:** o [Engine](../internal/engine/engine.go) depende diretamente de `*storage.Storage`; o roadmap registra a abstração como pendente.
6. **P3 — Avaliar persistência e desempenho após consolidar o MVP:** persistência e benchmarks estão planejados, não implementados; consulte o [roadmap](../memorydb-roadmap.md).
