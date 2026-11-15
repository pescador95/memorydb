# Manual do MemoryDB

## O que o sistema faz

MemoryDB é um servidor chave-valor que mantém valores em memória durante a execução. Clientes enviam comandos PING, SET, GET e DELETE por TCP, utilizando mensagens Protocol Buffers.

Os dados são compartilhados pelos clientes conectados à mesma instância do servidor, mas não são preservados quando o processo é encerrado ou reiniciado.

A implementação atual oferece:
- Armazenamento de chaves do tipo `string`.
- Armazenamento de valores do tipo `[]byte`.
- Consulta, inserção, substituição e remoção de valores.
- Compartilhamento dos dados entre conexões da mesma instância.
- Comunicação TCP com mensagens Protobuf.
- Driver cliente para aplicações Go.

Evidências:
- `server/internal/engine/engine.go`
- `server/internal/storage/storage.go`
- `protocol/proto/memorydb/v1/memorydb.proto`

## Pré-requisitos

Para executar o projeto localmente, são necessários:

- Go 1.27.0.
- Repositório MemoryDB clonado.
- Workspace Go configurado pelo arquivo `go.work` da raiz.
- Porta TCP disponível para o servidor, por padrão `6380`.

O projeto possui vários módulos Go independentes. Por isso, os comandos de execução devem ser realizados dentro do módulo correspondente.

## Iniciar e usar a demonstração

### 1. Iniciar o servidor

Abra um terminal na raiz do repositório e entre no módulo `server`:

```powershell
cd server
go run ./cmd/memorydb
```

Por padrão, o servidor escuta na porta TCP `6380`, em todas as interfaces de rede.

Para escolher outro endereço, defina a variável `MEMORYDB_ADDR` antes de iniciar o servidor.

Exemplo no PowerShell:

```powershell
$env:MEMORYDB_ADDR = "127.0.0.1:6380"
go run ./cmd/memorydb
```

Para voltar ao endereço padrão na sessão atual do PowerShell:

```powershell
Remove-Item Env:MEMORYDB_ADDR
```

O endereço padrão e a variável de ambiente são definidos em:

`server/cmd/memorydb/main.go`

### 2. Executar o cliente de demonstração

Com o servidor ativo, abra outro terminal na raiz do repositório e entre no módulo `api`:

```powershell
cd api
go run ./client
```

O cliente de demonstração utiliza o driver Go e conecta ao endereço `127.0.0.1:6380`.

Ele demonstra operações de PING, SET, GET e DELETE.

**Importante:** o cliente de demonstração utiliza endereço fixo. Alterar `MEMORYDB_ADDR` no servidor não altera automaticamente o endereço utilizado pelo cliente.

Arquivos envolvidos:
- `api/client/main.go`
- `drivers/go/client.go`

### 3. Encerrar o servidor

O servidor trata os sinais de encerramento SIGINT e SIGTERM e solicita um shutdown gracioso.

No terminal em que o servidor está executando, pressione `Ctrl+C`.

O servidor fecha o listener e aguarda as conexões ativas terminarem, respeitando o timeout de shutdown de 10 segundos configurado no executável.

O shutdown não força o fechamento das conexões ativas quando esse prazo expira.

## Utilizar o driver Go em outra aplicação

O módulo `drivers/go` oferece uma API cliente para aplicações Go.

As operações públicas disponíveis são:

- `Dial(address)`: estabelece conexão com o servidor.
- `New(conn)`: cria um cliente a partir de uma conexão `net.Conn`.
- `Ping()`: verifica a comunicação.
- `Set(key, value)`: armazena ou substitui um valor.
- `Get(key)`: consulta um valor.
- `Delete(key)`: remove uma chave.
- `Close()`: encerra a conexão.

Exemplo de utilização:

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

O exemplo demonstra a API do driver. Para utilizá-lo em outro projeto, o módulo `github.com/pescador95/memorydb/drivers/go` deverá estar publicado ou disponível por outro mecanismo de resolução de dependências Go.

No desenvolvimento dentro deste repositório, o `go.work` resolve os módulos locais.

**Observação:** o driver atual serializa as operações de requisição e resposta em cada conexão. Não há multiplexação de requisições simultâneas na mesma conexão.

## Funcionalidades disponíveis

O MemoryDB não oferece interface web, endpoints HTTP ou comandos de texto diretamente pelo TCP.

A comunicação utiliza:
1. Conexão TCP.
2. Frame com prefixo de tamanho.
3. Payload serializado em Protocol Buffers.
4. Solicitação e resposta correlacionadas por `request_id`.

O contrato externo está definido em:

`protocol/proto/memorydb/v1/memorydb.proto`

| Comando | Quando usar | Entrada do contrato | Resultado esperado |
|---|---|---|---|
| PING | Verificar se o servidor responde. | `Request { request_id, ping: {} }` | `success=true`, resultado `pong` |
| SET | Armazenar ou substituir um valor. | `Request { request_id, set: { key, value, ttl_ms: 0 } }` | `success=true`, resultado `set` |
| GET | Consultar um valor. | `Request { request_id, get: { key } }` | `success=true`; `get.found` informa se a chave existe |
| DELETE | Remover uma chave. | `Request { request_id, delete: { key } }` | `success=true`; `delete.deleted` informa se houve remoção |

Os exemplos representam os campos do schema Protobuf, não um formato JSON aceito pelo servidor.

Chaves são strings e valores são bytes. O `request_id` enviado pelo cliente deve ser preservado na resposta correspondente.

### Exemplo lógico de utilização

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

Essa sequência representa o fluxo de operações disponibilizado pelo engine e pelo driver Go.

## Semântica dos comandos

### PING

Verifica se o servidor está respondendo.

Uma resposta válida contém o mesmo `request_id` da solicitação e o resultado `PongResult`.

### SET

Armazena um valor associado a uma chave.

Se a chave já existir, o valor anterior será substituído.

O valor é recebido como bytes. O armazenamento realiza uma cópia dos bytes para evitar que alterações no slice original modifiquem diretamente o conteúdo armazenado.

O campo `ttl_ms` deve ser zero na implementação atual.

### GET

Consulta o valor associado a uma chave.

O resultado possui dois campos importantes:

- `found`: indica se a chave existe.
- `value`: contém os bytes armazenados quando a chave existe.

Uma chave existente com valor vazio retorna `found=true`. Uma chave inexistente retorna `found=false`.

Portanto, não utilize apenas o tamanho do valor para determinar se uma chave existe.

### DELETE

Remove uma chave do armazenamento.

O resultado `deleted` indica se uma chave foi efetivamente removida.

Excluir uma chave inexistente não é considerado erro de aplicação: a operação retorna sucesso com `deleted=false`.

## Protocolo de transporte

O transporte TCP utiliza framing para delimitar as mensagens.

Cada frame possui:

| Campo | Tamanho | Descrição |
|---|---:|---|
| Prefixo | 4 bytes | Tamanho do payload, como `uint32` Big Endian |
| Payload | 1 byte a 1 MiB | Mensagem Protobuf serializada |

O prefixo informa somente o tamanho do payload, sem incluir os próprios quatro bytes do prefixo.

O leitor utiliza leitura completa para lidar com fragmentação TCP. Isso é necessário porque TCP fornece um fluxo de bytes, não uma sequência de mensagens com fronteiras preservadas.

O módulo reutilizável de transporte está em:

- `transport/tcp/go/framing.go`
- `transport/tcp/go/connection.go`

O framing é independente de Protobuf. A serialização é responsabilidade do codec utilizado pelo servidor e pelo driver.

## Erros comuns

| Situação | O que acontece | Como resolver |
|---|---|---|
| Servidor não está ativo | O cliente não consegue estabelecer conexão. | Inicie `server/cmd/memorydb` e confirme endereço e porta. |
| Endereço ou porta incorretos | A conexão falha ou não alcança a instância esperada. | Confira o endereço configurado no cliente e em `MEMORYDB_ADDR`. |
| SET, GET ou DELETE com chave vazia | Retorno `INVALID_ARGUMENT`, com mensagem `key cannot be empty`. | Envie uma chave não vazia. |
| SET com `ttl_ms` maior que zero | Retorno `UNSUPPORTED_FEATURE`, com mensagem `TTL is not supported yet`. | Utilize `ttl_ms: 0`. |
| Frame com tamanho zero | A operação de framing falha. | Envie um payload válido. |
| Frame maior que 1 MiB | A operação de framing falha. | Reduza o tamanho da mensagem. |
| Frame incompleto | A leitura não consegue completar o frame e retorna erro. | Confira o tamanho declarado e envie todos os bytes. |
| Payload Protobuf inválido | A decodificação falha e o handler encerra o processamento da conexão. | Confira o schema e a serialização do cliente. |
| Comando ou resultado ausente | A validação ou decodificação rejeita a mensagem. | Envie um comando suportado e um envelope válido. |
| Chave inexistente em GET | Retorno bem-sucedido com `found=false`. | Verifique o campo `found`; isso não representa erro. |
| DELETE de chave inexistente | Retorno bem-sucedido com `deleted=false`. | Grave a chave antes, caso precise removê-la. |

Erros de framing e de decodificação não são necessariamente convertidos em `ErrorResult`. Em geral, eles encerram o processamento da conexão.

Erros de conexão, como conexão recusada ou conexão encerrada, podem apresentar mensagens específicas do sistema operacional.

## Limitações conhecidas

- **Sem persistência:** os valores são perdidos quando o processo é encerrado ou reiniciado.
- **TTL indisponível:** `ttl_ms` existe no contrato, mas valores positivos retornam `UNSUPPORTED_FEATURE`.
- **Sem autenticação:** qualquer cliente que consiga estabelecer conexão pode tentar executar comandos.
- **Sem TLS:** a comunicação não é criptografada.
- **Limite de frame:** o payload máximo é de 1 MiB.
- **Sem timeout configurável de leitura e escrita:** não há política configurável de timeout por operação.
- **Sem limite configurável de conexões:** não há cota de conexões concorrentes implementada.
- **Shutdown sem fechamento forçado:** conexões ativas podem atrasar o encerramento até o contexto de shutdown expirar.
- **Sem multiplexação:** o driver Go processa uma operação por vez em cada conexão.
- **Sem reconexão automática:** o driver não restabelece automaticamente conexões interrompidas.
- **Cliente de demonstração com endereço fixo:** utiliza `127.0.0.1:6380`.
- **Sem interface HTTP ou REST:** o protocolo disponível é TCP com framing e Protobuf.

Não exponha o serviço a redes não confiáveis enquanto autenticação e TLS não estiverem implementados.

## Perguntas frequentes

### O que significa GET retornar `found=false`?

Significa que a chave não existe no armazenamento.

Isso é diferente de uma chave existente com valor de zero bytes, que retorna `found=true`.

### Os dados ficam disponíveis para mais de um cliente?

Sim. Enquanto os clientes estiverem conectados à mesma instância do servidor, todos utilizam o mesmo armazenamento em memória.

Cada processo do servidor possui seu próprio storage. Duas instâncias independentes não compartilham automaticamente os dados.

### Os dados continuam disponíveis após reiniciar?

Não. O armazenamento atual é exclusivamente em memória e não existe persistência implementada.

### Posso utilizar TTL?

Ainda não. O campo `ttl_ms` está presente no contrato Protobuf, mas qualquer valor positivo é rejeitado.

Utilize zero para armazenar valores sem expiração.

### O servidor oferece API REST?

Não. A implementação atual utiliza TCP com frames de tamanho prefixado e payload Protobuf.

### Posso enviar JSON diretamente pelo TCP?

Não. O servidor espera frames contendo mensagens serializadas conforme o contrato Protobuf.

### Posso utilizar outro cliente além do Go?

Sim, desde que o cliente implemente o framing TCP e o contrato Protobuf corretamente.

O transporte e o schema são independentes da linguagem. Um driver em outra linguagem pode implementar o mesmo contrato.

### O que acontece quando o servidor é encerrado?

O servidor fecha o listener e aguarda as conexões ativas terminarem, respeitando o timeout configurado no executável.

Os dados em memória são perdidos quando o processo termina.

## Referências técnicas

| Assunto | Arquivo |
|---|---|
| Arquitetura geral | `docs/tecnica/ARQUITETURA.md` |
| Catálogo de componentes | `docs/tecnica/COMPONENTES.md` |
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
| Cliente de demonstração | `api/client/main.go` |
| Executável do servidor | `server/cmd/memorydb/main.go` |
| Workspace Go | `go.work` |
| Roadmap | `memorydb-roadmap.md` |