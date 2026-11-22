# Manual do MemoryDB

## 1. Visão geral

MemoryDB é um servidor chave-valor em memória que permite armazenar, consultar, substituir e remover valores por meio de comandos enviados por TCP.

A comunicação utiliza mensagens Protocol Buffers (Protobuf), encapsuladas em frames com prefixo de tamanho.

A implementação atual oferece:

- Comandos PING, SET, GET e DELETE.
- Chaves do tipo `string`.
- Valores do tipo `[]byte`.
- Armazenamento compartilhado entre conexões da mesma instância.
- Atendimento concorrente de múltiplas conexões.
- Driver cliente para aplicações Go.
- Operações do driver com suporte a `context.Context`.
- Tratamento de erros de aplicação e de resultados indeterminados.

O armazenamento é exclusivamente em memória. Os dados são perdidos quando o processo do servidor termina ou é reiniciado.

## 2. Pré-requisitos

Para executar o projeto localmente:

- Go 1.27.0.
- Repositório MemoryDB clonado.
- Workspace Go disponível no arquivo `go.work` da raiz.
- Porta TCP disponível para o servidor, por padrão `6380`.

O projeto é organizado em módulos Go independentes. Os comandos de execução devem ser realizados no módulo correspondente.

## 3. Executar o servidor

### 3.1. Iniciar

Na raiz do repositório, entre no módulo `server`:

```bash
cd server
go run ./cmd/memorydb
```

Por padrão, o servidor escuta em `:6380`, ou seja, na porta 6380 em todas as interfaces de rede disponíveis.

O endereço pode ser alterado pela variável de ambiente `MEMORYDB_ADDR`.

PowerShell:

```powershell
$env:MEMORYDB_ADDR = "127.0.0.1:6380"
go run ./cmd/memorydb
```

Linux:

```bash
MEMORYDB_ADDR=127.0.0.1:6380 go run ./cmd/memorydb
```

Para remover a variável na sessão atual do PowerShell:

```powershell
Remove-Item Env:MEMORYDB_ADDR
```

O endereço padrão e a variável de ambiente são definidos em `server/cmd/memorydb/main.go`.

**Segurança:** o servidor não possui autenticação, autorização ou TLS. Escutar em todas as interfaces pode permitir que outros equipamentos alcancem o serviço, dependendo da configuração da rede e do firewall. Não exponha o MemoryDB a redes não confiáveis.

### 3.2. Confirmar a inicialização

Quando o servidor inicia corretamente, registra uma mensagem semelhante a:

```text
MemoryDB listening on [endereço]
```

Se a porta estiver ocupada ou o endereço for inválido, o processo retorna um erro de inicialização.

### 3.3. Encerrar

No terminal em que o servidor está em execução, pressione `Ctrl+C`.

O executável trata SIGINT e SIGTERM e solicita o encerramento do servidor com timeout de 10 segundos.

O shutdown fecha o listener e aguarda as conexões ativas terminarem. Se o prazo expirar, o servidor retorna erro; a implementação não força diretamente o fechamento das conexões ativas. O executável trata esse erro como falha e encerra pelo caminho de erro.

Os dados em memória são perdidos quando o processo termina.

## 4. Executar o cliente de demonstração

Com o servidor ativo, abra outro terminal na raiz do repositório:

```bash
cd api
go run ./client
```

O cliente demonstra:

1. Conexão com o servidor.
2. PING.
3. SET de uma chave.
4. GET do valor armazenado.
5. DELETE da chave.
6. GET após a exclusão para confirmar que ela não existe.

O cliente utiliza o endereço fixo `127.0.0.1:6380`, definido em `api/client/main.go`.

**Importante:** alterar `MEMORYDB_ADDR` no servidor não altera automaticamente o endereço utilizado pelo cliente de demonstração. Se o servidor estiver em outro endereço, o exemplo deverá ser ajustado.

## 5. Utilizar o driver Go em outra aplicação

O módulo `drivers/go` disponibiliza uma biblioteca cliente para aplicações Go. Ele abstrai a conexão TCP, o framing e a serialização Protobuf.

O driver não mantém o armazenamento localmente: as operações são encaminhadas ao servidor MemoryDB.

### 5.1. Instalação

O caminho de importação é:

```go
github.com/pescador95/memorydb/drivers/go
```

Crie uma aplicação Go:

```bash
mkdir minha-api
cd minha-api
go mod init minha-api
```

Após a publicação e disponibilização das versões do driver e de suas dependências, instale o módulo:

```bash
go get github.com/pescador95/memorydb/drivers/go@v0.1.0
go mod tidy
```

Substitua `v0.1.0` pela versão efetivamente publicada que deseja utilizar.

O driver depende também dos módulos de transporte TCP e protocolo Protobuf. Para que a instalação independente funcione, esses módulos precisam estar disponíveis nas versões declaradas nas dependências.

Durante o desenvolvimento dentro do repositório MemoryDB, o arquivo `go.work` resolve os módulos locais. Ele não é necessário na aplicação consumidora.

**Nota sobre distribuição:** a instalação acima pressupõe que as versões indicadas já estejam publicadas e acessíveis no mecanismo de resolução de módulos Go. A existência de uma versão declarada em um `go.mod`, isoladamente, não comprova que ela esteja publicada.

### 5.2. Importar

```go
import (
    memorydb "github.com/pescador95/memorydb/drivers/go"
)
```

O alias `memorydb` facilita a identificação dos tipos e funções da biblioteca.

### 5.3. Métodos disponíveis

| Operação | Método convencional | Método com contexto |
|---|---|---|
| Conectar | `Dial(address)` | `DialContext(ctx, address)` |
| Criar client a partir de conexão | `New(conn)` | — |
| Verificar comunicação | `Ping()` | `PingContext(ctx)` |
| Armazenar | `Set(key, value)` | `SetContext(ctx, key, value)` |
| Consultar | `Get(key)` | `GetContext(ctx, key)` |
| Excluir | `Delete(key)` | `DeleteContext(ctx, key)` |
| Encerrar | `Close()` | — |

Os métodos sem contexto utilizam `context.Background()` internamente.

`New(conn)` recebe uma conexão `net.Conn` existente e transfere sua propriedade ao client. Quando `Close()` for chamado, a conexão será encerrada.

### 5.4. Exemplo de utilização

```go
package main

import (
    "fmt"
    "log"

    memorydb "github.com/pescador95/memorydb/drivers/go"
)

func main() {
    client, err := memorydb.Dial("127.0.0.1:6380")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    if err := client.Ping(); err != nil {
        log.Fatal(err)
    }

    if err := client.Set("user:1", []byte("memorydb")); err != nil {
        log.Fatal(err)
    }

    value, found, err := client.Get("user:1")
    if err != nil {
        log.Fatal(err)
    }

    if !found {
        log.Fatal("chave não encontrada")
    }

    fmt.Printf("Valor: %s\n", value)

    deleted, err := client.Delete("user:1")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Removido: %t\n", deleted)
}
```

### 5.5. Utilização com contexto

As variantes com `context.Context` permitem definir deadlines e cancelar operações.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    memorydb "github.com/pescador95/memorydb/drivers/go"
)

func main() {
    ctx, cancel := context.WithTimeout(
        context.Background(),
        3*time.Second,
    )
    defer cancel()

    client, err := memorydb.DialContext(ctx, "127.0.0.1:6380")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    if err := client.PingContext(ctx); err != nil {
        log.Fatal(err)
    }

    if err := client.SetContext(
        ctx,
        "user:1",
        []byte("memorydb"),
    ); err != nil {
        log.Fatal(err)
    }

    value, found, err := client.GetContext(ctx, "user:1")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Encontrado: %t, valor: %s\n", found, value)
}
```

`DialContext` utiliza o contexto somente durante o estabelecimento da conexão. O contexto não fica associado ao client para controlar as operações posteriores.

Para controlar cada operação, utilize os métodos específicos que recebem contexto.

## 6. Semântica dos comandos

### 6.1. PING

Verifica se o servidor responde à solicitação.

Uma resposta válida contém o mesmo `request_id` da solicitação e o resultado `PongResult`.

O PING não comprova a disponibilidade de funcionalidades adicionais, persistência ou capacidade de processamento.

### 6.2. SET

Armazena um valor associado a uma chave.

Se a chave já existir, o valor anterior será substituído.

Chaves são strings e valores são bytes. O armazenamento realiza uma cópia dos bytes recebidos.

O campo `ttl_ms` existe no contrato Protobuf, mas a implementação atual não oferece expiração por TTL:

- `ttl_ms = 0`: armazenamento sem expiração.
- `ttl_ms > 0`: retorna `UNSUPPORTED_FEATURE`.

### 6.3. GET

Consulta o valor associado a uma chave.

O resultado contém:

- `found`: indica se a chave existe.
- `value`: bytes armazenados quando a chave existe.

Uma chave existente com valor vazio retorna `found=true`. Uma chave inexistente retorna `found=false`.

Não utilize somente o tamanho do valor para determinar se uma chave existe.

### 6.4. DELETE

Remove uma chave do armazenamento.

O resultado `deleted` informa se uma chave foi efetivamente removida.

Excluir uma chave inexistente não é erro de aplicação: a operação retorna sucesso com `deleted=false`.

### 6.5. Sequência lógica

```text
PING

SET key="user:1"
    value=bytes("memorydb")
    ttl_ms=0

GET key="user:1"
    found=true
    value=bytes("memorydb")

DELETE key="user:1"
    deleted=true

GET key="user:1"
    found=false
```

Essa representação ilustra a sequência lógica dos comandos. Não é um formato de texto ou JSON aceito diretamente pelo servidor.

## 7. Protocolo de comunicação

O servidor não oferece API HTTP, REST ou comandos de texto diretamente pelo TCP.

A comunicação utiliza:

1. Conexão TCP.
2. Frame com prefixo de tamanho.
3. Payload serializado em Protocol Buffers.
4. Solicitação e resposta correlacionadas por `request_id`.

O contrato externo está definido em:

`protocol/proto/memorydb/v1/memorydb.proto`

### 7.1. Estrutura do frame

| Campo | Tamanho | Descrição |
|---|---:|---|
| Prefixo | 4 bytes | Tamanho do payload como `uint32` Big Endian |
| Payload | 1 byte a 1 MiB | Mensagem Protobuf serializada |

O prefixo informa somente o tamanho do payload, sem incluir os quatro bytes do próprio prefixo.

O payload máximo permitido é de 1 MiB.

Como TCP é um fluxo de bytes, uma mensagem pode ser recebida em partes. O transporte implementa leitura completa do frame e trata escritas parciais.

O framing é independente de Protobuf. A serialização é responsabilidade do codec utilizado pelo servidor e pelo driver.

### 7.2. Envelope Protobuf

Uma solicitação possui:

- `request_id`: identificador da solicitação.
- `command`: um dos comandos suportados.

Uma resposta possui:

- `request_id`: identificador correspondente à solicitação.
- `success`: indica sucesso ou rejeição.
- `result`: resultado do comando ou erro estruturado.

O `request_id` é utilizado para correlação entre solicitação e resposta. Ele não funciona como chave de idempotência e não oferece deduplicação no servidor.

### 7.3. Clientes em outras linguagens

O protocolo e o schema Protobuf são independentes da linguagem.

Um cliente em outra linguagem pode implementar o mesmo contrato, desde que respeite:

- O framing TCP.
- O prefixo de tamanho em Big Endian.
- O limite máximo do payload.
- A serialização Protobuf.
- Os tipos de solicitação e resposta.
- A correlação por `request_id`.

A existência do contrato não significa que já exista um driver implementado e validado para todas as linguagens.

## 8. Concorrência e ciclo de vida do client

O driver Go permite que uma instância de `Client` seja compartilhada entre goroutines.

Entretanto, as operações são serializadas por conexão. Uma operação precisa terminar antes que outra utilize a mesma conexão.

Características:

- O servidor atende múltiplas conexões simultaneamente.
- O driver permite chamadas concorrentes sobre o mesmo client.
- As operações em uma mesma conexão são executadas sequencialmente.
- Não há multiplexação de múltiplas solicitações simultâneas na mesma conexão.
- O driver não oferece pool de conexões integrado.

O client possui três estados internos:

| Estado | Comportamento |
|---|---|
| `open` | Disponível para executar operações. |
| `closed` | Encerrado por `Close()`. |
| `failed` | Invalidado após falha de comunicação ou protocolo. |

Clientes nos estados `closed` e `failed` não devem ser reutilizados.

Após uma falha que invalide o client, crie uma nova conexão com `Dial` ou `DialContext`.

## 9. Tratamento de erros

O driver diferencia erros estruturados retornados pelo servidor de falhas de comunicação.

### 9.1. ServerError

`ServerError` representa um erro estruturado recebido do servidor.

O tipo disponibiliza:

- `Code`: código do erro.
- `Message`: descrição do erro.

Exemplos de códigos utilizados pela implementação:

| Código | Significado |
|---|---|
| `INVALID_REQUEST` | Solicitação inválida ou comando ausente/não suportado. |
| `INVALID_ARGUMENT` | Argumento inválido, como chave vazia. |
| `UNSUPPORTED_FEATURE` | Funcionalidade não suportada, como TTL positivo. |

Quando o servidor retorna uma resposta estruturada de erro, o driver disponibiliza `ServerError` e não invalida automaticamente a conexão.

### 9.2. OperationOutcomeUnknownError

`OperationOutcomeUnknownError` representa uma situação em que o client não consegue determinar se o servidor executou a operação.

Pode ocorrer quando:

- O cliente envia um comando e a conexão é interrompida antes de receber a resposta.
- O contexto é cancelado durante uma operação de I/O.
- O servidor executa o comando, mas a resposta não chega ao cliente.
- A resposta recebida não pode ser validada ou correlacionada com a solicitação.

O erro disponibiliza:

- `RequestID`: identificador da solicitação.
- `Cause`: causa da interrupção ou falha.

O tipo implementa `Unwrap()`, permitindo utilizar `errors.Is` e `errors.As`.

Exemplo:

```go
var outcomeErr *memorydb.OperationOutcomeUnknownError

if errors.As(err, &outcomeErr) {
    log.Printf(
        "Resultado desconhecido para request %d: %v",
        outcomeErr.RequestID,
        outcomeErr.Cause,
    )
}
```

Para esse exemplo, inclua os pacotes `errors` e `log` nos imports.

**Atenção:** um resultado indeterminado não significa que o servidor deixou de executar o comando. O comando pode ter sido executado, mesmo que o cliente não tenha recebido a confirmação.

O `request_id` não permite consultar o resultado anterior nem impede a execução duplicada de comandos.

A implementação atual não oferece:

- Deduplicação server-side.
- Recuperação posterior do resultado por identificador.
- Retry automático.
- Reconexão automática.

Não repita automaticamente uma operação cujo resultado seja desconhecido sem avaliar os possíveis efeitos de uma execução duplicada.

### 9.3. Cancelamento e falhas de comunicação

O comportamento do driver depende do momento da interrupção:

| Situação | Comportamento |
|---|---|
| Falha ao estabelecer conexão | Retorna erro na inicialização. |
| Erro estruturado do servidor | Retorna `ServerError`; client não é invalidado automaticamente. |
| Cancelamento enquanto aguarda acesso à conexão | Operação cancelada; conexão preservada. |
| Falha de comunicação durante I/O | Client passa para `failed`; resultado pode ser desconhecido. |
| Cancelamento durante I/O | Conexão encerrada; client passa para `failed`. |
| Resposta inválida ou incompatível | Client é invalidado; resultado pode ser desconhecido. |
| Uso após `Close()` | Operação rejeitada. |
| Uso após falha de comunicação | Operação rejeitada. |

Depois de uma falha de comunicação ou cancelamento durante I/O, não reutilize o client.

## 10. Erros comuns e diagnóstico

| Situação | Causa provável | Ação recomendada |
|---|---|---|
| Servidor não está ativo | Processo não iniciado ou encerrado. | Inicie o servidor e verifique o endereço. |
| Conexão recusada | Endereço ou porta incorretos, ou serviço indisponível. | Confira `MEMORYDB_ADDR` e o endereço utilizado pelo client. |
| Porta ocupada | Outro processo está utilizando a porta. | Escolha outra porta ou encerre o processo conflitante. |
| Chave vazia | SET, GET ou DELETE recebeu uma chave vazia. | Envie uma chave não vazia. |
| TTL positivo | A funcionalidade ainda não está implementada. | Utilize `ttl_ms = 0`. |
| Frame com tamanho zero | Payload inválido. | Envie um payload Protobuf válido. |
| Frame maior que 1 MiB | Payload excede o limite do transporte. | Reduza o tamanho da mensagem. |
| Frame incompleto | A mensagem não foi transmitida integralmente. | Confira o tamanho declarado e o envio dos bytes. |
| Payload Protobuf inválido | Serialização incompatível ou mensagem malformada. | Confira o schema e a serialização. |
| Chave inexistente em GET | Chave não está armazenada. | Verifique `found`; isso não representa erro. |
| DELETE de chave inexistente | Chave não estava armazenada. | Verifique `deleted`; isso não representa erro. |
| Contexto cancelado antes da operação | Cancelamento ou deadline antes do início do I/O. | Verifique o prazo e o estado do contexto. |
| Contexto cancelado durante I/O | Comunicação interrompida. | Crie outro client e avalie se é seguro repetir a operação. |
| OperationOutcomeUnknownError | Não foi possível confirmar o resultado. | Não repita indiscriminadamente o comando. |
| Client fechado ou com falha | Ciclo de vida encerrado ou conexão invalidada. | Crie um novo client. |

Erros de framing e decodificação não são necessariamente convertidos em `ErrorResult`. Em geral, eles encerram o processamento da conexão.

Erros de conexão podem apresentar mensagens específicas do sistema operacional.

## 11. Limitações conhecidas

A implementação atual possui as seguintes limitações:

- **Sem persistência:** os valores são perdidos quando o processo termina ou reinicia.
- **Sem TTL:** valores positivos de `ttl_ms` são rejeitados.
- **Sem autenticação:** clientes que alcançam o servidor podem tentar executar comandos.
- **Sem autorização:** não há controle de permissões por cliente ou operação.
- **Sem TLS:** a comunicação não é criptografada.
- **Sem isolamento por tenant:** clientes da mesma instância compartilham o armazenamento.
- **Limite de frame:** payload máximo de 1 MiB.
- **Sem timeout configurável por operação no servidor:** não há política configurável de timeout de leitura e escrita.
- **Sem cota de conexões:** não há limite configurável de conexões concorrentes.
- **Sem proteção específica contra clientes lentos:** não há política específica de timeout de inatividade ou envio lento de dados.
- **Shutdown sem fechamento forçado:** conexões ativas não são fechadas diretamente quando o contexto de shutdown expira.
- **Sem multiplexação:** o driver Go executa uma operação por vez em cada conexão.
- **Sem pool de conexões integrado:** o driver não gerencia um conjunto de conexões.
- **Sem reconexão automática:** conexões interrompidas não são restabelecidas automaticamente.
- **Sem retry automático:** comandos não são repetidos após falhas de comunicação.
- **Sem idempotência server-side:** `request_id` não impede execução duplicada.
- **Resultado indeterminado:** uma interrupção durante a comunicação pode impedir que o cliente saiba se o comando foi executado.
- **Cliente de demonstração com endereço fixo:** utiliza `127.0.0.1:6380`.
- **Sem interface HTTP ou REST:** o protocolo disponível é TCP com framing e Protobuf.
- **Sem driver Java implementado e validado nesta etapa:** a integração Java não faz parte do escopo funcional atual.

Não utilize o MemoryDB como armazenamento durável nem exponha o serviço a redes não confiáveis enquanto os mecanismos de segurança necessários não estiverem implementados.

## 12. Perguntas frequentes

### Os dados ficam disponíveis para mais de um cliente?

Sim. Clientes conectados à mesma instância utilizam o mesmo armazenamento em memória.

Cada processo do servidor possui seu próprio storage. Instâncias independentes não compartilham automaticamente os dados.

### Os dados continuam disponíveis após reiniciar?

Não. O armazenamento atual é exclusivamente em memória.

### Posso utilizar TTL?

Ainda não. O campo `ttl_ms` está presente no contrato Protobuf, mas valores positivos são rejeitados. Utilize zero.

### O servidor oferece API REST?

Não. A implementação atual utiliza TCP com frames de tamanho prefixado e payload Protobuf.

### Posso enviar JSON diretamente pelo TCP?

Não. O servidor espera frames contendo mensagens serializadas conforme o contrato Protobuf.

### Posso utilizar outro cliente além do Go?

Sim, desde que o cliente implemente corretamente o framing TCP e o contrato Protobuf.

Isso não significa que já exista um driver pronto para todas as linguagens.

### Posso cancelar uma operação?

Sim. O driver Go disponibiliza métodos com `context.Context`.

Se o cancelamento ocorrer enquanto o client aguarda para iniciar uma operação, a conexão permanece disponível.

Se ocorrer durante a comunicação de rede, a conexão é encerrada porque o resultado da operação pode ser desconhecido.

### O contexto de DialContext controla as operações posteriores?

Não. Ele controla somente o estabelecimento da conexão. Para controlar operações posteriores, utilize `PingContext`, `SetContext`, `GetContext` ou `DeleteContext`.

### O driver reconecta automaticamente?

Não. Após falha de comunicação ou cancelamento durante I/O, é necessário criar um novo client.

### Posso executar várias operações simultaneamente no mesmo client?

As chamadas podem partir de goroutines diferentes, mas o driver serializa as operações na mesma conexão.

O servidor atende múltiplas conexões simultaneamente.

### O que significa OperationOutcomeUnknownError?

Significa que o client não conseguiu confirmar se o servidor executou a operação.

O comando pode ter sido executado, mesmo que a resposta não tenha chegado. Não repita automaticamente a operação sem avaliar os efeitos de uma execução duplicada.

### O que acontece quando o servidor é encerrado?

O servidor fecha o listener e aguarda as conexões ativas, respeitando o timeout de 10 segundos.

Se o contexto expirar, o executável trata o erro e encerra pelo caminho de falha. O shutdown não força diretamente o fechamento das conexões ativas.

Os dados em memória são perdidos quando o processo termina.

## 13. Referências técnicas

| Assunto | Arquivo |
|---|---|
| Arquitetura geral | `docs/tecnica/ARQUITETURA.md` |
| Componentes | `docs/tecnica/COMPONENTES.md` |
| Requisitos | `docs/requisitos/REQUISITOS.md` |
| Contrato Protobuf | `protocol/proto/memorydb/v1/memorydb.proto` |
| Engine | `server/internal/engine/engine.go` |
| Storage | `server/internal/storage/storage.go` |
| Modelo interno | `server/internal/protocol/message.go` |
| Codec | `server/internal/codec/codec.go` |
| Codec Protobuf | `server/internal/codec/protobuf/codec.go` |
| Handler TCP | `server/internal/transport/tcp/handler.go` |
| Servidor TCP | `server/internal/transport/tcp/server.go` |
| Framing | `transport/tcp/go/framing.go` |
| Conexão TCP | `transport/tcp/go/connection.go` |
| Driver Go | `drivers/go/client.go` |
| Testes do driver Go | `drivers/go/client_test.go` |
| Cliente de demonstração | `api/client/main.go` |
| Executável do servidor | `server/cmd/memorydb/main.go` |
| Workspace Go | `go.work` |
| Guia específico do driver Go | `drivers/go/README.md` |
| Roadmap | `memorydb-roadmap.md` |

## 14. Referências externas

- Repositório: https://github.com/pescador95/memorydb
- Documentação oficial do Go: https://go.dev/doc/
- Download do Go: https://go.dev/dl/
- Gerenciamento de módulos Go: https://go.dev/doc/modules/managing-dependencies
- Pacote `context`: https://pkg.go.dev/context
- Pacote `errors`: https://pkg.go.dev/errors
- Pacote `net/http`: https://pkg.go.dev/net/http

---

**MemoryDB**

Servidor chave-valor em memória com comunicação TCP e protocolo Protocol Buffers.