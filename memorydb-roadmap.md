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

Os dados são perdidos quando o processo do servidor é encerrado.

## 4. Arquitetura planejada

O fluxo principal de comunicação é:

```text
Cliente
  |
  v
Driver Go / Driver Java
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

Os módulos continuam independentes e poderão ser publicados e consumidos separadamente no futuro.

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
- [x] Testes de framing e comunicação.

### Fase 5 — Codec e integração com o servidor

**Status: concluída**

- [x] Definição da interface de Codec.
- [x] Implementação do Codec Protobuf.
- [x] Conversão entre Protobuf e modelo interno.
- [x] Implementação do Handler TCP.
- [x] Integração entre transporte, Codec, Engine e Storage.
- [x] Processamento de múltiplas requisições na mesma conexão.
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
- [x] Encerramento gracioso.
- [x] Espera pelo encerramento das conexões ativas.
- [x] Tratamento de sinais do sistema operacional.
- [x] Configuração do endereço por variável de ambiente (`MEMORYDB_ADDR`).

### Fase 7 — Driver Go

**Status: concluída**

- [x] Criação de módulo independente.
- [x] Implementação de conexão com o servidor.
- [x] Implementação de `PING`.
- [x] Implementação de `SET`.
- [x] Implementação de `GET`.
- [x] Implementação de `DELETE`.
- [x] Tratamento de erros do servidor.
- [x] Validação do `request_id` nas respostas.
- [x] Tratamento de respostas Protobuf inválidas.
- [x] Sincronização de requisições concorrentes.
- [x] Gerenciamento de encerramento da conexão.
- [x] Testes unitários e de falhas de comunicação.

O driver Go utiliza o módulo TCP compartilhado, evitando duplicar a implementação de framing.

### Fase 8 — API cliente e validação integrada

**Status: concluída**

- [x] Criação do módulo `api`.
- [x] Utilização do driver Go pela aplicação cliente.
- [x] Testes de integração entre cliente e servidor.
- [x] Testes de múltiplos clientes.
- [x] Testes de operações concorrentes.
- [x] Testes de desconexão de clientes.
- [x] Testes do encerramento do servidor com conexões ativas.

### Validação técnica atual

**Status: aprovada**

Todos os módulos foram submetidos a testes com o detector de condições de corrida e à análise estática.

Comandos executados:

```bash
for module in protocol/generated/go transport/tcp/go drivers/go server api; do
    (cd "$module" && go test -race -count=1 ./...) || exit 1
done
```

```bash
for module in protocol/generated/go transport/tcp/go drivers/go server api; do
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

Todos os comandos concluíram sem falhas.

## 9. Próximas fases

### Fase 9 — Consolidação da arquitetura e documentação

**Status: em andamento**

- [ ] Revisar a documentação de arquitetura em relação à implementação atual.
- [ ] Revisar a documentação dos componentes.
- [ ] Atualizar os requisitos funcionais e não funcionais.
- [ ] Atualizar o manual de utilização.
- [ ] Documentar o contrato de framing TCP.
- [ ] Documentar o fluxo de comunicação entre driver e servidor.
- [ ] Documentar a utilização do Go Workspace.
- [ ] Documentar como executar testes em cada módulo.
- [ ] Revisar as decisões arquiteturais e registrar as justificativas relevantes.

### Fase 10 — Driver Java

**Status: planejada**

- [ ] Definir a estrutura do módulo Java.
- [ ] Gerar as classes Java a partir do contrato Protobuf.
- [ ] Implementar conexão TCP.
- [ ] Implementar framing compatível com o driver Go.
- [ ] Implementar serialização e desserialização Protobuf.
- [ ] Implementar `PING`, `SET`, `GET` e `DELETE`.
- [ ] Implementar tratamento de erros.
- [ ] Criar testes unitários.
- [ ] Criar testes de integração com o servidor Go.
- [ ] Validar interoperabilidade entre os drivers Go e Java.

O driver Java deverá implementar o mesmo contrato de comunicação, sem depender de detalhes internos da implementação Go.

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

### Fase 12 — Robustez e experiência de utilização

**Status: planejada**

- [ ] Definir configuração por variáveis de ambiente.
- [ ] Padronizar logs.
- [ ] Melhorar mensagens e códigos de erro.
- [ ] Avaliar timeouts de conexão e operações.
- [ ] Avaliar limites de conexões simultâneas.
- [ ] Avaliar limites de tamanho de chave e valor.
- [ ] Melhorar a API pública dos drivers.
- [ ] Criar exemplos de utilização.
- [ ] Criar ferramenta CLI para interação com o servidor.

### Fase 13 — Segurança

**Status: planejada**

- [ ] Implementar suporte a TLS.
- [ ] Definir autenticação de clientes.
- [ ] Definir autorização de operações.
- [ ] Avaliar isolamento por usuário ou tenant.
- [ ] Definir política de rotação de credenciais.
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
- [ ] Criar testes de recuperação.

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

**Situação atual:** os critérios técnicos do MVP estão atendidos pela implementação e pelas validações executadas. A consolidação documental e a revisão final da arquitetura permanecem como trabalho da Fase 9.

---

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
| 12 | Robustez e utilização | Planejada |
| 13 | Segurança | Planejada |
| 14 | Protocolos alternativos | Planejada |
| 15 | Persistência | Futura |
| 16 | Distribuição e escalabilidade | Futura |