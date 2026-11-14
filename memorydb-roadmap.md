# MemoryDB — Roadmap de implementação

**Objetivo:** construir um banco de dados chave-valor em memória, desacoplado do transporte e da serialização, com implementação inicial em Go e comunicação TCP + Protocol Buffers.

**Princípios:**
- Aprender os fundamentos implementando os componentes essenciais.
- Separar armazenamento, execução de comandos, modelo interno, serialização e transporte.
- Manter poucas dependências.
- Priorizar correção, previsibilidade, concorrência segura e testes.
- Permitir evolução de codecs, transportes e implementações de armazenamento.
- Permitir que clientes de qualquer linguagem utilizem o protocolo sem depender de um SDK proprietário.
- Evitar otimizações prematuras: medir antes de otimizar.

---

## 1. Estado atual

O projeto já possui uma implementação funcional do núcleo e do servidor TCP.

### Componentes implementados

- [x] Criar o repositório/projeto `memorydb`.
- [x] Definir Go como linguagem do servidor.
- [x] Configurar o módulo Go: `github.com/pescador95/memorydb`.
- [x] Escolher TCP como transporte inicial.
- [x] Escolher Protocol Buffers como serialização inicial.
- [x] Instalar e configurar `protoc` e `protoc-gen-go`.
- [x] Criar `api/proto/memorydb.proto`.
- [x] Gerar `api/proto/memorydb.pb.go`.
- [x] Definir um modelo interno independente de Protocol Buffers.
- [x] Criar uma interface de codec.
- [x] Implementar o codec Protobuf.
- [x] Implementar framing TCP com prefixo de tamanho.
- [x] Implementar gerenciamento de conexão TCP.
- [x] Implementar armazenamento em memória concorrente.
- [x] Implementar execução dos comandos no engine.
- [x] Implementar handler de conexões.
- [x] Implementar servidor TCP com múltiplos clientes.
- [x] Implementar encerramento gracioso do servidor.
- [x] Criar executável do servidor.
- [x] Criar cliente de teste para operações pela rede.
- [x] Validar manualmente PING, SET e GET.
- [x] Validar compartilhamento dos dados entre conexões diferentes.
- [x] Validar que os dados são perdidos após reiniciar o servidor.

### Validação realizada

| Validação | Resultado |
|---|---|
| Testes unitários do storage | Aprovado |
| Testes unitários do engine | Aprovado |
| Testes do codec Protobuf | Aprovado |
| Testes de framing TCP | Aprovado |
| Testes de comunicação TCP | Aprovado |
| Testes do handler | Aprovado |
| Testes do servidor | Aprovado |
| Testes de concorrência com `-race` nos componentes testados | Aprovado |
| Execução de `go test ./...` | Aprovado |
| Comunicação manual cliente-servidor | Aprovado |
| Persistência entre conexões enquanto o servidor está ativo | Aprovado |
| Perda dos dados após reinicialização do servidor | Aprovado |

**Pendência importante:** automatizar o fluxo completo em testes de integração e consolidar a execução de `go test -race ./...` em todo o projeto.

---

## 2. Escopo do primeiro MVP

O primeiro MVP será um servidor único, em memória, sem persistência e sem distribuição.

### Comandos iniciais

| Comando | Comportamento |
|---|---|
| `PING` | Verifica se o servidor está respondendo. |
| `SET` | Armazena uma chave e seu valor. |
| `GET` | Recupera o valor de uma chave. |
| `DELETE` | Remove uma chave. |

### Regras iniciais

- Chaves são strings.
- Valores são bytes, permitindo armazenar texto ou dados binários.
- `GET` diferencia chave inexistente de valor vazio.
- `DELETE` informa se uma chave foi removida.
- `ttl_ms` já está representado no protocolo, mas sua implementação está pendente.
- O processo mantém os dados apenas na memória.
- Reiniciar o servidor perde todos os dados.
- Múltiplos clientes podem compartilhar o mesmo armazenamento enquanto utilizam a mesma instância do servidor.

---

## 3. Arquitetura atual

A arquitetura foi organizada para que os componentes possam evoluir independentemente.

```text
                    CLIENTES
                       |
                       v
                TRANSPORTE TCP
                       |
                       v
                 TCP CONNECTION
                       |
                       v
                    FRAMING
                       |
                       v
                 CODEC PROTOBUF
                       |
                       v
                MODELO INTERNO
                       |
                       v
                    ENGINE
                       |
                       v
                    STORAGE
                       |
                       v
                 MAP EM MEMÓRIA
```

### Responsabilidades

**Storage**
- Armazenar e recuperar valores.
- Controlar acesso concorrente.
- Não conhecer protocolos, codecs ou transporte.

**Engine**
- Interpretar comandos internos.
- Validar argumentos.
- Executar operações no storage.
- Produzir resultados e erros independentes do transporte.

**Protocol**
- Definir os modelos internos de requisição, resposta, comando e resultado.
- Não depender de TCP ou Protocol Buffers.

**Codec**
- Converter os modelos internos em bytes.
- Converter bytes em modelos internos.
- Permitir diferentes formatos de serialização.

**Transport**
- Gerenciar conexões.
- Ler e escrever mensagens.
- Implementar framing e comunicação.
- Não conhecer os detalhes da serialização.

**Server**
- Aceitar conexões.
- Delegar o processamento ao handler.
- Gerenciar o ciclo de vida do servidor e seu encerramento.

### Direção de evolução

A arquitetura deve permitir combinações como:

| Transporte | Codec / protocolo | Situação |
|---|---|---|
| TCP | Protocol Buffers | Implementado |
| TCP | JSON | Futuro |
| HTTP | JSON | Futuro |
| HTTP | Protocol Buffers | Futuro |
| WebSocket | JSON | Futuro |
| Redis RESP | Adaptador de protocolo | Futuro |

**Observação:** Redis RESP não é apenas um codec de serialização. É um protocolo de comunicação com convenções próprias de comandos e respostas. Uma implementação futura deverá traduzir RESP para o modelo interno do MemoryDB, mantendo o engine independente desse protocolo.

---

## 4. Roadmap por etapas

### Etapa 1 — Consolidar o contrato interno e o protocolo externo

**Situação: implementação básica concluída; documentação pendente.**

- [x] Criar `api/proto/memorydb.proto`.
- [x] Definir `Request` e `Response`.
- [x] Utilizar `oneof` para representar comandos e resultados.
- [x] Garantir que respostas carreguem o `request_id`.
- [x] Definir PING, SET, GET e DELETE.
- [x] Representar valores como `bytes`.
- [x] Representar `ttl_ms` no contrato.
- [x] Definir um modelo interno independente do Protobuf.
- [x] Implementar conversão entre Protobuf e modelo interno.
- [x] Definir códigos iniciais de erro: `INVALID_REQUEST`, `INVALID_ARGUMENT` e `UNSUPPORTED_FEATURE`.
- [ ] Documentar todos os códigos de erro e suas condições.
- [ ] Documentar limites de chaves, valores e mensagens.
- [ ] Documentar a semântica de `ttl_ms`.
- [ ] Documentar compatibilidade e evolução do protocolo.
- [ ] Documentar regras para campos removidos e números reservados no Protobuf.

**Critério de conclusão:** o contrato externo e o modelo interno possuem responsabilidades claras, e as regras de comunicação estão documentadas.

### Etapa 2 — Implementar o framing TCP

**Situação: concluída e testada.**

Cada mensagem TCP utiliza um prefixo de tamanho para delimitar o payload.

Formato implementado:

```text
+----------------------+--------------------------+
| Tamanho: 4 bytes     | Payload: N bytes         |
| uint32 Big Endian    | Protobuf                  |
+----------------------+--------------------------+
```

- [x] Implementar framing em `internal/transport/tcp/framing.go`.
- [x] Ler os quatro bytes do cabeçalho.
- [x] Interpretar o tamanho como `uint32` Big Endian.
- [x] Validar o tamanho antes de alocar memória.
- [x] Definir limite máximo inicial de 1 MiB.
- [x] Ler o payload completo com `io.ReadFull`.
- [x] Implementar escrita do cabeçalho e payload.
- [x] Tratar escritas parciais.
- [x] Tratar frames incompletos e tamanhos inválidos.
- [x] Criar testes unitários para fragmentação, frames inválidos e limites.

**Critério de conclusão:** frames são lidos e escritos corretamente mesmo quando os bytes chegam fragmentados ou as escritas são parciais.

### Etapa 3 — Implementar o storage

**Situação: implementação básica concluída e testada.**

- [x] Criar `internal/storage`.
- [x] Criar a estrutura `Entry`.
- [x] Implementar armazenamento baseado em `map[string]Entry`.
- [x] Implementar `Set`.
- [x] Implementar `Get`.
- [x] Implementar `Delete`.
- [x] Definir a semântica de valor ausente.
- [x] Proteger operações com `sync.RWMutex`.
- [x] Evitar compartilhamento indevido de slices de bytes nas operações de leitura e escrita.
- [x] Testar operações, valores vazios e sobrescrita.
- [x] Testar acesso concorrente com `-race`.
- [ ] Definir uma interface de armazenamento para permitir implementações alternativas.
- [ ] Preparar a abstração para suportar diferentes backends.

**Critério de conclusão:** o armazenamento funciona isoladamente, com segurança concorrente e sem dependências de rede ou serialização.

### Etapa 4 — Implementar o command engine

**Situação: implementação concluída e testada.**

- [x] Criar `internal/engine`.
- [x] Implementar dispatcher de comandos.
- [x] Implementar PING.
- [x] Implementar SET.
- [x] Implementar GET.
- [x] Implementar DELETE.
- [x] Converter resultados do storage em respostas internas.
- [x] Preservar `request_id`.
- [x] Validar comandos e argumentos.
- [x] Padronizar códigos de erro iniciais.
- [x] Tratar requisições nulas e comandos desconhecidos.
- [x] Testar o engine sem depender de TCP.

**Critério de conclusão:** uma requisição do modelo interno pode ser processada e produzir uma resposta correta sem envolvimento de transporte ou codec.

### Etapa 5 — Implementar codecs

**Situação: implementação Protobuf concluída.**

- [x] Criar `internal/codec`.
- [x] Definir a interface `Codec`.
- [x] Criar `internal/codec/protobuf`.
- [x] Implementar codificação de requisições.
- [x] Implementar decodificação de requisições.
- [x] Implementar codificação de respostas.
- [x] Implementar decodificação de respostas.
- [x] Tratar mensagens inválidas e comandos/resultados ausentes.
- [x] Criar testes de round-trip.
- [x] Testar valores binários.
- [x] Validar a implementação com a interface `Codec`.
- [ ] Avaliar implementação futura de codec JSON.
- [ ] Documentar como adicionar novos codecs.

**Critério de conclusão:** a serialização pode ser substituída sem alterar o modelo interno ou o engine.

### Etapa 6 — Implementar o transporte TCP e o servidor

**Situação: implementação funcional concluída; robustez adicional pendente.**

- [x] Criar `internal/transport/tcp`.
- [x] Implementar `Connection`.
- [x] Injetar o codec na conexão.
- [x] Implementar leitura e escrita de requisições e respostas.
- [x] Sincronizar leituras e escritas concorrentes.
- [x] Implementar `Handler`.
- [x] Processar múltiplas requisições sequenciais por conexão.
- [x] Tratar desconexão limpa por EOF.
- [x] Criar `Server`.
- [x] Aceitar múltiplos clientes simultaneamente.
- [x] Gerenciar o ciclo de vida das conexões.
- [x] Implementar encerramento gracioso com `context.Context`.
- [x] Fechar o listener durante o encerramento.
- [x] Aguardar conexões ativas durante o shutdown.
- [x] Testar múltiplos clientes.
- [x] Testar segunda chamada concorrente a `Serve`.
- [x] Testar encerramento aguardando conexões ativas.
- [ ] Registrar erros de processamento das conexões.
- [ ] Implementar timeouts de leitura e escrita.
- [ ] Avaliar limite de conexões simultâneas.
- [ ] Avaliar limites de recursos por conexão.

**Critério de conclusão:** o servidor aceita múltiplos clientes, processa requisições e encerra de forma controlada.

### Etapa 7 — Integrar e validar o MVP

**Situação: integração manual validada; automação pendente.**

- [x] Criar `cmd/memorydb/main.go`.
- [x] Configurar endereço por `MEMORYDB_ADDR`.
- [x] Definir endereço padrão `:6380`.
- [x] Inicializar codec, engine, handler e servidor.
- [x] Implementar tratamento de sinais de encerramento.
- [x] Implementar timeout para encerramento gracioso.
- [x] Criar `cmd/client` para testes manuais.
- [x] Validar PING pela rede.
- [x] Validar SET pela rede.
- [x] Validar GET pela rede.
- [x] Validar persistência dos dados entre conexões durante a execução do servidor.
- [x] Validar perda dos dados após reiniciar o servidor.
- [ ] Criar testes automatizados de integração usando listener TCP real.
- [ ] Testar PING, SET, GET e DELETE de ponta a ponta.
- [ ] Testar múltiplas requisições na mesma conexão.
- [ ] Testar compartilhamento de dados entre conexões diferentes.
- [ ] Testar múltiplos clientes simultâneos.
- [ ] Testar desconexão durante um frame.
- [ ] Testar reinicialização lógica com uma nova instância do engine.
- [ ] Executar `go test ./...`.
- [ ] Executar `go test -race ./...`.

**Critério de conclusão:** o fluxo completo cliente → TCP → framing → codec → modelo interno → engine → storage → resposta funciona de forma automatizada e reproduzível.

### Etapa 8 — TTL e expiração

**Situação: planejada.**

- [ ] Definir formalmente a semântica de `ttl_ms`.
- [ ] Definir `ttl_ms = 0` como ausência de expiração.
- [ ] Armazenar o instante de expiração em cada entrada.
- [ ] Fazer GET tratar entradas expiradas como inexistentes.
- [ ] Fazer SET atualizar valor e expiração atomicamente.
- [ ] Fazer DELETE funcionar normalmente com entradas expiradas.
- [ ] Definir estratégia de remoção física das entradas expiradas.
- [ ] Avaliar limpeza periódica em segundo plano.
- [ ] Evitar criar uma goroutine por chave.
- [ ] Testar expiração, atualização de TTL e ausência de expiração.
- [ ] Avaliar comportamento concorrente durante a expiração.

**Critério de conclusão:** entradas expiradas deixam de ser visíveis e são eventualmente removidas da memória.

### Etapa 9 — Robustez e segurança operacional

**Situação: parcialmente implementada; melhorias pendentes.**

- [x] Limitar o tamanho máximo de frame.
- [x] Validar mensagens e argumentos recebidos.
- [x] Tratar desconexões.
- [x] Implementar encerramento gracioso.
- [ ] Limitar conexões simultâneas.
- [ ] Aplicar timeouts de leitura e escrita.
- [ ] Tratar clientes lentos.
- [ ] Evitar crescimento ilimitado de recursos.
- [ ] Padronizar logs estruturados com `log/slog`.
- [ ] Registrar erros das conexões.
- [ ] Expor métricas básicas: conexões, comandos, erros e latência.
- [ ] Documentar que TCP sem TLS não deve ser exposto a redes não confiáveis.
- [ ] Avaliar TLS e autenticação antes de uso fora de ambiente local ou confiável.

**Critério de conclusão:** entradas malformadas, clientes lentos e desconexões não provocam travamentos ou encerramentos inesperados.

### Etapa 10 — Persistência

**Situação: planejada para depois do MVP.**

- [ ] Especificar formato de persistência.
- [ ] Avaliar AOF (Append Only File) ou log equivalente.
- [ ] Definir política de sincronização em disco.
- [ ] Implementar recuperação durante a inicialização.
- [ ] Testar encerramento inesperado e recuperação.
- [ ] Avaliar snapshots para reduzir o tempo de replay.
- [ ] Definir compatibilidade entre versões do formato persistido.

**Critério de conclusão:** os dados podem ser recuperados após reiniciar o processo, respeitando a política de durabilidade documentada.

### Etapa 11 — Desempenho e evolução

**Situação: planejada.**

- [ ] Criar benchmarks de SET, GET e DELETE.
- [ ] Medir throughput e latência.
- [ ] Medir consumo de memória.
- [ ] Medir desempenho com diferentes quantidades de clientes.
- [ ] Identificar gargalos antes de otimizar.
- [ ] Avaliar pipelining e múltiplas requisições em voo.
- [ ] Avaliar concorrência e contenção do lock.
- [ ] Avaliar alternativas de armazenamento concorrente.
- [ ] Considerar políticas de eviction somente quando houver limite de memória configurável.
- [ ] Avaliar pooling de conexões quando houver evidência de necessidade.
- [ ] Considerar replicação e sharding somente depois de estabilizar uma instância única.

**Critério de conclusão:** decisões de otimização são justificadas por benchmarks reproduzíveis.

### Etapa 12 — Evolução de protocolos e backends

**Situação: planejada.**

- [ ] Implementar codec JSON como alternativa ao Protobuf.
- [ ] Avaliar HTTP como transporte alternativo.
- [ ] Avaliar WebSocket como transporte alternativo.
- [ ] Definir interface de armazenamento.
- [ ] Avaliar implementação de backend Redis.
- [ ] Avaliar adaptador de protocolo Redis RESP.
- [ ] Garantir que novas implementações não introduzam dependências no engine.
- [ ] Criar testes de contrato para implementações alternativas.

**Critério de conclusão:** diferentes transportes, codecs e backends podem ser combinados sem acoplamento indevido entre as camadas.

## Etapa 13 — Segurança, criptografia e autenticação

**Situação: planejada.**

- [ ] Definir o modelo de autenticação do MemoryDB.
- [ ] Implementar TLS nas conexões TCP.
- [ ] Definir política de certificados e renovação.
- [ ] Implementar autenticação por credenciais.
- [ ] Armazenar senhas utilizando hashes apropriados.
- [ ] Implementar criação, atualização e revogação de credenciais.
- [ ] Associar credenciais a tenants.
- [ ] Implementar autorização por operação.
- [ ] Impedir acesso a dados de outros tenants.
- [ ] Avaliar autenticação mTLS.
- [ ] Avaliar túneis SSH para acesso administrativo.
- [ ] Implementar testes de autenticação, autorização e isolamento.
- [ ] Documentar requisitos de segurança para implantação.

**Critério de conclusão:** somente clientes autenticados e autorizados conseguem executar operações, com comunicação criptografada e isolamento entre tenants.

## Etapa 14 — Multi-tenancy e isolamento de armazenamento

**Situação: planejada.**

- [ ] Definir o modelo de multi-tenancy.
- [ ] Criar entidade ou configuração de Tenant.
- [ ] Implementar identificação do tenant por conexão autenticada.
- [ ] Criar um componente responsável por resolver o contexto do tenant.
- [ ] Criar gerenciamento de instâncias de armazenamento.
- [ ] Permitir uma instância de storage independente por tenant.
- [ ] Garantir que cada operação seja executada no storage correto.
- [ ] Impedir acesso cruzado entre tenants.
- [ ] Definir configurações individuais por tenant.
- [ ] Avaliar limites de memória por tenant.
- [ ] Avaliar limites de conexões e requisições por tenant.
- [ ] Implementar criação, ativação, suspensão e remoção de tenants.
- [ ] Testar concorrência entre tenants.
- [ ] Testar isolamento de dados e configurações.
- [ ] Avaliar instâncias dedicadas por cliente.

**Critério de conclusão:** múltiplos clientes conseguem utilizar a mesma infraestrutura sem compartilhar indevidamente dados, configurações ou permissões.

## Etapa 15 — Domínios e configuração personalizada

**Situação: planejada.**

- [ ] Definir o modelo de associação entre domínios e tenants.
- [ ] Permitir domínios personalizados.
- [ ] Implementar resolução de tenant por domínio.
- [ ] Avaliar identificação por SNI em conexões TCP com TLS.
- [ ] Avaliar suporte a HTTP como transporte alternativo.
- [ ] Definir gerenciamento e validação de domínios.
- [ ] Implementar gerenciamento de certificados TLS.
- [ ] Permitir configurações personalizadas por tenant.
- [ ] Definir políticas de domínio principal e aliases.
- [ ] Implementar testes de roteamento e isolamento por domínio.

**Critério de conclusão:** domínios configurados são associados aos respectivos tenants, com validação de identidade e roteamento seguro para o armazenamento correto.

---

## 5. Estrutura de diretórios atual

A estrutura abaixo representa a implementação existente e não uma estrutura futura idealizada.

```text
memorydb/
├── api/
│   └── proto/
│       ├── memorydb.proto
│       └── memorydb.pb.go
├── cmd/
│   ├── memorydb/
│   │   └── main.go
│   └── client/
│       └── main.go
├── internal/
│   ├── codec/
│   │   ├── codec.go
│   │   └── protobuf/
│   │       ├── codec.go
│   │       └── codec_test.go
│   ├── engine/
│   │   ├── command.go
│   │   ├── engine.go
│   │   └── engine_test.go
│   ├── protocol/
│   │   └── message.go
│   ├── storage/
│   │   ├── entry.go
│   │   ├── storage.go
│   │   └── storage_test.go
│   └── transport/
│       └── tcp/
│           ├── framing.go
│           ├── framing_test.go
│           ├── connection.go
│           ├── communication_test.go
│           ├── handler.go
│           ├── handler_test.go
│           ├── server.go
│           └── server_test.go
├── go.mod
├── go.sum
└── README.md
```

A estrutura poderá ganhar arquivos de documentação, testes de integração, outros codecs e transportes conforme o projeto evoluir.

---

## 6. Dependências

Manter o projeto enxuto.

**Dependência de execução:**

- `google.golang.org/protobuf`: serialização e desserialização das mensagens.

**Ferramentas de desenvolvimento:**

- `protoc`: compilador do arquivo `.proto`.
- `protoc-gen-go`: gerador de código Go.

**Biblioteca padrão do Go:**

- `net`: comunicação TCP.
- `encoding/binary`: interpretação do cabeçalho.
- `io`: leitura e escrita completas.
- `sync`: sincronização concorrente.
- `context`: controle de ciclo de vida.
- `os/signal`: tratamento de sinais.
- `log`: logging inicial.
- `testing`: testes e benchmarks.

Evitar frameworks, ORMs e dependências adicionais antes de existir uma necessidade concreta.

---

## 7. Acesso por outras linguagens

O objetivo é permitir que clientes em outras linguagens se comuniquem diretamente com o servidor, sem depender de SDK proprietário.

- [x] Definir contrato Protobuf.
- [x] Definir framing com prefixo de quatro bytes.
- [ ] Publicar a especificação do protocolo e framing.
- [ ] Documentar byte order e limite máximo de frame.
- [ ] Documentar comandos, respostas e erros.
- [ ] Criar exemplos mínimos de cliente em outra linguagem.
- [ ] Validar interoperabilidade com pelo menos uma segunda linguagem.

Um cliente precisa implementar o adaptador que lê e escreve o prefixo de tamanho e utiliza uma biblioteca Protobuf convencional. Isso não exige um SDK proprietário do MemoryDB.

---

## 8. Decisões adiadas intencionalmente

Não implementar antes de estabilizar e validar o MVP:

- Compatibilidade com Redis RESP.
- Pub/Sub e streaming de eventos.
- Replicação e cluster.
- Sharding.
- Eviction/LRU/LFU.
- Persistência avançada e snapshots incrementais.
- Autenticação sofisticada e controle de acesso granular.
- SDKs oficiais para várias linguagens.
- Interface web de administração.
- Otimizações prematuras de alocação, pooling e estruturas de dados.

Esses itens não estão descartados. Apenas ficam fora do ciclo atual para manter o foco no núcleo do banco.

---

## 9. Como vamos evoluir este roadmap

Ao concluir cada tarefa:

1. Atualizar o checkbox correspondente.
2. Registrar decisões técnicas que afetem etapas futuras.
3. Registrar testes executados e seus resultados.
4. Diferenciar implementação, teste automatizado e validação manual.
5. Manter novas tarefas na etapa adequada.
6. Atualizar a estrutura de diretórios quando ela mudar.
7. Não marcar como concluído algo que foi apenas planejado ou validado manualmente.
8. Só avançar quando o critério de conclusão da etapa estiver atendido.

---

## 10. Próximo passo recomendado

**Consolidar os testes de integração do MemoryDB.**

A implementação atual já permite executar o servidor e realizar operações reais pela rede. O próximo passo é transformar essa validação manual em testes automatizados.

Prioridades:

1. Criar testes de integração utilizando `net.Listen("tcp", "127.0.0.1:0")`.
2. Inicializar servidor, engine, handler e codec dentro do teste.
3. Conectar com `net.Dial` e utilizar `Connection`.
4. Validar PING, SET, GET e DELETE.
5. Validar múltiplas requisições na mesma conexão e compartilhamento entre clientes.
6. Executar `go test ./...` e `go test -race ./...`.

**Não precisamos implementar novas funcionalidades antes disso.** O objetivo é consolidar a arquitetura existente e garantir que o fluxo completo funcione de maneira reproduzível.