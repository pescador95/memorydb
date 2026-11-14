# Manual do MemoryDB

## O que o sistema faz

MemoryDB é um servidor chave-valor que mantém valores em memória durante a execução. Clientes enviam comandos PING, SET, GET e DELETE por TCP usando mensagens Protocol Buffers. Os dados são compartilhados pelos clientes conectados à mesma instância, mas não são preservados quando o processo é encerrado. Evidências: [Engine.Execute](../../internal/engine/engine.go), [Storage](../../internal/storage/storage.go), [memorydb.proto](../../api/proto/memorydb.proto).

## Iniciar e usar a demonstração

### 1. Iniciar o servidor

Na raiz do repositório, execute:

```powershell
go run ./cmd/memorydb
```

Por padrão, o servidor escuta na porta TCP `6380` em todas as interfaces. Para escolher outro endereço, defina `MEMORYDB_ADDR` antes de iniciar. Exemplo no PowerShell:

```powershell
$env:MEMORYDB_ADDR = "127.0.0.1:6380"
go run ./cmd/memorydb
```

O endereço padrão e a variável de ambiente são definidos em [cmd/memorydb/main.go](../../cmd/memorydb/main.go).

### 2. Executar o cliente de demonstração

Com o servidor ativo, abra outro terminal na raiz do repositório e execute:

```powershell
go run ./cmd/client
```

O cliente conecta a `127.0.0.1:6380` e realiza PING, SET, GET, DELETE e um GET final para confirmar que a chave foi removida. A sequência é definida em [cmd/client/main.go](../../cmd/client/main.go).

Saída esperada para a implementação atual:

```text
PING: PONG
SET: user:1 = memorydb
GET: user:1 found = true
GET: user:1 = memorydb
DELETE: user:1 removed = true
GET: user:1 found = false

CRUD validation completed successfully
```

As linhas são impressas pelo cliente de demonstração; detalhes de formatação podem variar se o código for alterado.

## Funcionalidades disponíveis

O MemoryDB não oferece interface web nem endpoints HTTP. O cliente atual é uma demonstração Go; outros clientes precisam implementar o framing TCP e o contrato Protobuf publicados em [memorydb.proto](../../api/proto/memorydb.proto).

| Comando | Quando usar | Entrada do contrato | Resultado esperado |
|---|---|---|---|
| PING | Verificar se o servidor responde. | `Request { request_id, ping: {} }` | `Response { request_id, success: true, pong: {} }` |
| SET | Armazenar ou substituir um valor. | `Request { request_id, set: { key, value, ttl_ms: 0 } }` | `Response { request_id, success: true, set: {} }` |
| GET | Consultar um valor. | `Request { request_id, get: { key } }` | `success: true`; `get.found` informa se existe. Se encontrado, `get.value` contém os bytes. |
| DELETE | Remover uma chave. | `Request { request_id, delete: { key } }` | `success: true`; `delete.deleted` informa se havia uma chave para remover. |

Os exemplos acima representam os campos do schema Protobuf, não um formato JSON aceito pelo servidor. Chaves são strings e valores são bytes; `request_id` deve ser copiado da solicitação para a resposta. Evidência: [memorydb.proto](../../api/proto/memorydb.proto).

### Exemplo observado no cliente

O cliente de demonstração executa esta sequência lógica:

```text
PING
SET key="user:1", value=bytes("memorydb"), ttl_ms=0
GET key="user:1"  -> found=true, value=bytes("memorydb")
DELETE key="user:1" -> deleted=true
GET key="user:1"  -> found=false
```

Essa sequência está implementada em [`crud`](../../cmd/client/main.go) e nas funções auxiliares `ping`, `set`, `get` e `delete`.

## Erros comuns

| Situação | O que acontece | Como resolver |
|---|---|---|
| Servidor não está ativo ou endereço/porta não correspondem | O cliente não consegue estabelecer a conexão e encerra com erro de conexão. | Inicie `cmd/memorydb`; confirme `MEMORYDB_ADDR` e que o cliente está conectando ao mesmo endereço. |
| SET, GET ou DELETE com chave vazia | O servidor retorna `success=false`, código `INVALID_ARGUMENT` e mensagem `key cannot be empty`. | Envie uma chave não vazia. Evidência: [Engine.Execute](../../internal/engine/engine.go). |
| SET com `ttl_ms` maior que zero | O servidor retorna `success=false`, código `UNSUPPORTED_FEATURE` e mensagem `TTL is not supported yet`. | Use `ttl_ms: 0`; expiração ainda não é suportada. Evidência: [Engine.Execute](../../internal/engine/engine.go). |
| Frame com tamanho zero, maior que 1 MiB ou incompleto | A operação de framing falha; o handler encerra a conexão sem resposta de erro de aplicação. | Verifique o prefixo de 4 bytes Big Endian, o tamanho declarado e se todos os bytes foram enviados. Evidência: [framing.go](../../internal/transport/tcp/framing.go), [handler.go](../../internal/transport/tcp/handler.go). |
| Payload Protobuf inválido ou comando/result ausente | A codificação/decodificação retorna erro e a conexão de servidor é encerrada; não há mensagem de erro de aplicação nesse caso. | Confira o schema e envie um comando Protobuf suportado. Evidência: [protobuf/codec.go](../../internal/codec/protobuf/codec.go), [handler.go](../../internal/transport/tcp/handler.go). |
| Chave não existe em GET | Não é uma falha: resposta bem-sucedida com `found=false`. | Use `found` para distinguir ausência de um valor vazio. Evidência: [Engine.Execute](../../internal/engine/engine.go). |
| DELETE de chave inexistente | Não é uma falha: resposta bem-sucedida com `deleted=false`. | Grave a chave antes se precisar removê-la. Evidência: [Engine.Execute](../../internal/engine/engine.go). |

Erros do protocolo TCP (por exemplo, conexão recusada ou conexão encerrada) podem conter mensagens específicas do sistema operacional. O cliente de demonstração mostra esses erros no terminal; não existe catálogo próprio para essas mensagens. Evidência: [cmd/client/main.go](../../cmd/client/main.go).

## Limitações conhecidas

- **Sem persistência:** os valores são perdidos ao parar ou reiniciar o servidor. Evidência: [Storage](../../internal/storage/storage.go).
- **TTL indisponível:** `ttl_ms` aparece no contrato, mas TTL positivo retorna `UNSUPPORTED_FEATURE`. Evidência: [memorydb.proto](../../api/proto/memorydb.proto), [Engine.Execute](../../internal/engine/engine.go).
- **Sem autenticação e TLS:** o código TCP não autentica clientes nem criptografa a conexão. Não exponha o serviço a redes não confiáveis. Evidência: [Server](../../internal/transport/tcp/server.go); itens futuros em [memorydb-roadmap.md](../../memorydb-roadmap.md).
- **Limite de frame:** mensagens TCP têm payload máximo de 1 MiB. Evidência: [framing.go](../../internal/transport/tcp/framing.go).
- **Sem timeout de leitura/escrita ou cota de conexões:** o código não define esses limites; clientes que não encerram conexões podem afetar o encerramento gracioso. Evidência: [Connection](../../internal/transport/tcp/connection.go), [Server](../../internal/transport/tcp/server.go).
- **Cliente de demonstração fixo:** conecta a `127.0.0.1:6380`; não lê o endereço da variável `MEMORYDB_ADDR`. Evidência: [cmd/client/main.go](../../cmd/client/main.go).

## Perguntas frequentes

### O que significa GET retornar `found=false`?

A chave não existe no armazenamento. Isso é diferente de uma chave existente cujo valor tem zero bytes, que retorna `found=true`. Evidência: [Storage.Get](../../internal/storage/storage.go), [TestEmptyValue](../../internal/storage/storage_test.go).

### Os dados ficam disponíveis para mais de um cliente?

Sim, enquanto os clientes estiverem conectados à mesma instância do servidor. Cada processo executa um storage em memória próprio. Evidência: [engine.New](../../internal/engine/engine.go), [Server.Serve](../../internal/transport/tcp/server.go).

### Os dados continuam disponíveis após reiniciar?

Não. O storage é em memória e não existe persistência implementada. Evidência: [Storage](../../internal/storage/storage.go).

### Posso usar TTL?

Não. O campo está no contrato, mas qualquer `ttl_ms` positivo é rejeitado. Use zero para gravar sem expiração. Evidência: [memorydb.proto](../../api/proto/memorydb.proto), [Engine.Execute](../../internal/engine/engine.go).

### O servidor oferece API REST ou aceita texto/JSON diretamente?

Não. O servidor usa TCP com frames de tamanho prefixado e payload Protobuf; o schema não define HTTP ou JSON. Evidência: [framing.go](../../internal/transport/tcp/framing.go), [memorydb.proto](../../api/proto/memorydb.proto).
