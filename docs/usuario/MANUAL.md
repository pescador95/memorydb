# Manual do MemoryDB

## 1. Visão geral

MemoryDB é um servidor chave-valor em memória, implementado em Go, que permite armazenar, consultar, substituir e remover valores por meio de comandos enviados por TCP.

A comunicação utiliza mensagens Protocol Buffers (Protobuf), encapsuladas em frames com prefixo de tamanho.

A implementação atual oferece:

- Comandos PING, SET, GET e DELETE.
- Chaves do tipo `string` e valores do tipo `[]byte`.
- Armazenamento compartilhado entre conexões da mesma instância.
- Atendimento concorrente de múltiplas conexões.
- Driver cliente para aplicações Go.
- Operações do driver com suporte a `context.Context`.
- Idempotência server-side para SET e DELETE, por meio de `OperationID`.
- Deduplicação de operações e reprodução de respostas enquanto os registros estão retidos.
- Tratamento de erros de aplicação, falhas de comunicação e resultados indeterminados.

O armazenamento e os registros de idempotência são exclusivamente em memória. Os dados e o histórico de operações são perdidos quando o processo do servidor termina ou é reiniciado.

## 2. Pré-requisitos

Para executar o projeto localmente:

- Go 1.27.0.
- Repositório MemoryDB clonado.
- Workspace Go disponível no arquivo `go.work` da raiz.
- Porta TCP disponível para o servidor, por padrão `6380`.
- Utilitários de linha de comando utilizados pelos scripts, como `find`, `xargs`, `awk`, `sort`, `tail`, `mktemp` e `rm`.

O projeto é organizado em módulos Go independentes. A raiz do repositório contém um workspace Go, mas não é, por si só, um módulo Go. Por isso, os comandos de execução e validação devem ser realizados no módulo correspondente.

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

Os dados em memória e os registros de idempotência são perdidos quando o processo termina.

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

O cliente gera `OperationID` para as operações SET e DELETE.

O endereço utilizado é fixo: `127.0.0.1:6380`, definido em `api/client/main.go`.

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
| Armazenar | `Set(operationID, key, value)` | `SetContext(ctx, operationID, key, value)` |
| Consultar | `Get(key)` | `GetContext(ctx, key)` |
| Excluir | `Delete(operationID, key)` | `DeleteContext(ctx, operationID, key)` |
| Encerrar | `Close()` | — |

Os métodos sem contexto utilizam `context.Background()` internamente.

`New(conn)` recebe uma conexão `net.Conn` existente e transfere sua propriedade ao client. Quando `Close()` for chamado, a conexão será encerrada.

As operações `SET` e `DELETE` exigem um `OperationID`, utilizado pelo servidor para identificar uma operação lógica e evitar sua execução duplicada enquanto o registro correspondente estiver retido.

O tipo `memorydb.OperationID` possui 16 bytes. A função `memorydb.NewOperationID()` gera um identificador aleatório utilizando `crypto/rand`.

A aplicação consumidora deve:

- Gerar um identificador novo para cada operação lógica.
- Reutilizar o mesmo identificador em novas tentativas da mesma operação.
- Manter os mesmos parâmetros da operação original ao reutilizar o identificador.
- Não gerar um novo identificador para contornar um resultado indeterminado.

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

    setID, err := memorydb.NewOperationID()
    if err != nil {
        log.Fatal(err)
    }

    if err := client.Set(
        setID,
        "user:1",
        []byte("memorydb"),
    ); err != nil {
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

    deleteID, err := memorydb.NewOperationID()
    if err != nil {
        log.Fatal(err)
    }

    deleted, err := client.Delete(deleteID, "user:1")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Removido: %t\n", deleted)
}
```

Cada operação mutável recebe seu próprio identificador. O `OperationID` do SET não deve ser reutilizado para o DELETE, pois são operações lógicas distintas.

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

    operationID, err := memorydb.NewOperationID()
    if err != nil {
        log.Fatal(err)
    }

    if err := client.SetContext(
        ctx,
        operationID,
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

Armazena um valor associado a uma chave. Se a chave já existir, o valor anterior será substituído.

Chaves são strings e valores são bytes. O armazenamento realiza uma cópia dos bytes recebidos.

O comando SET exige um `OperationID` de exatamente 16 bytes.

O servidor utiliza o identificador e uma impressão digital dos parâmetros da operação para reconhecer novas tentativas da mesma operação lógica.

O campo `ttl_ms` existe no contrato Protobuf, mas a implementação atual não oferece expiração por TTL:

- `ttl_ms = 0`: armazenamento sem expiração.
- `ttl_ms > 0`: retorna `UNSUPPORTED_FEATURE`.

O valor de TTL faz parte dos parâmetros considerados na identificação da operação. Uma tentativa com o mesmo `OperationID`, mas com parâmetros diferentes, é rejeitada como conflito.

### 6.3. GET

Consulta o valor associado a uma chave.

O resultado contém:

- `found`: indica se a chave existe.
- `value`: bytes armazenados quando a chave existe.

Uma chave existente com valor vazio retorna `found=true`. Uma chave inexistente retorna `found=false`.

Não utilize somente o tamanho do valor para determinar se uma chave existe.

GET não utiliza o mecanismo de idempotência por `OperationID`.

### 6.4. DELETE

Remove uma chave do armazenamento.

O comando DELETE exige um `OperationID` de exatamente 16 bytes.

O resultado `deleted` informa se uma chave foi efetivamente removida.

Excluir uma chave inexistente não é erro de aplicação: a operação retorna sucesso com `deleted=false`.

O servidor utiliza o `OperationID` e os parâmetros da operação para reconhecer novas tentativas e evitar a execução duplicada da mutação.

### 6.5. Sequência lógica

```text
PING

SET operation_id=<16 bytes>
    key="user:1"
    value=bytes("memorydb")
    ttl_ms=0

GET key="user:1"
    found=true
    value=bytes("memorydb")

DELETE operation_id=<16 bytes>
    key="user:1"
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

O payload máximo permitido é de 1 MiB. Frames com tamanho zero são inválidos.

Como TCP é um fluxo de bytes, uma mensagem pode ser recebida em partes. O transporte implementa leitura completa do frame e trata escritas parciais.

O framing é independente de Protobuf. A serialização é responsabilidade do codec utilizado pelo servidor e pelo driver.

### 7.2. Envelope Protobuf

Uma solicitação possui:

- `request_id`: identificador da tentativa de comunicação.
- `command`: um dos comandos suportados.

Uma resposta possui:

- `request_id`: identificador correspondente à solicitação.
- `success`: indica sucesso ou rejeição.
- `result`: resultado do comando ou erro estruturado.

O `request_id` é utilizado para correlação entre solicitação e resposta. Ele não funciona como chave de idempotência.

Nas operações SET e DELETE, o campo `operation_id` identifica a operação lógica.

| Identificador | Finalidade | Comportamento em uma nova tentativa |
|---|---|---|
| `request_id` | Correlacionar solicitação e resposta | Novo identificador para cada tentativa |
| `operation_id` | Identificar uma operação lógica mutável | Reutilizar o mesmo identificador |

O `operation_id` possui exatamente 16 bytes e é obrigatório para SET e DELETE.

O servidor calcula uma impressão digital dos parâmetros da operação. Com isso:

- Mesmo `operation_id` e mesmos parâmetros: a resposta anterior é reproduzida, quando o registro está retido.
- Mesmo `operation_id` e parâmetros diferentes: a solicitação é rejeitada com `OPERATION_ID_CONFLICT`.
- Identificador ausente ou com tamanho inválido: a solicitação é rejeitada com `INVALID_OPERATION_ID`.

A deduplicação não é realizada pelo `request_id`.

### 7.3. Clientes em outras linguagens

O protocolo e o schema Protobuf são independentes da linguagem.

Um cliente em outra linguagem pode implementar o mesmo contrato, desde que respeite:

- O framing TCP.
- O prefixo de tamanho em Big Endian.
- O limite máximo do payload.
- A serialização Protobuf.
- Os tipos de solicitação e resposta.
- A correlação por `request_id`.
- O campo `operation_id` de 16 bytes obrigatório em SET e DELETE.
- A semântica de idempotência e os códigos de erro correspondentes.

A implementação de um cliente compatível com o protocolo deve preservar o mesmo `operation_id` em novas tentativas da mesma operação lógica.

A existência do contrato não significa que já exista um driver implementado e validado para todas as linguagens.

## 8. Idempotência e OperationID

### 8.1. Finalidade

A idempotência server-side permite que o servidor reconheça tentativas repetidas de uma mesma operação mutável.

O mecanismo está disponível para SET e DELETE. PING e GET não utilizam esse mecanismo.

O objetivo é reduzir o risco de execução duplicada quando uma aplicação não recebe a resposta de uma operação e precisa tentar novamente. A idempotência não representa persistência nem garantia de execução exatamente uma vez diante de falhas ou reinicializações.

### 8.2. Identificador da operação

O `OperationID` possui exatamente 16 bytes.

O driver Go disponibiliza:

```go
operationID, err := memorydb.NewOperationID()
if err != nil {
    return err
}
```

Cada operação lógica deve receber um identificador próprio. Uma nova tentativa da mesma operação deve utilizar o mesmo identificador e os mesmos parâmetros originais.

Não gere um novo identificador para repetir uma operação cujo resultado anterior seja desconhecido.

### 8.3. Impressão digital da operação

O servidor calcula uma impressão digital dos parâmetros relevantes da operação utilizando SHA-256 e um esquema versionado.

Para SET, são considerados:

- Tipo da operação.
- Chave.
- Valor.
- TTL informado.

Para DELETE, são considerados:

- Tipo da operação.
- Chave.

O `OperationID` identifica a operação lógica, mas não substitui a validação de seus parâmetros. Ele não faz parte da impressão digital.

A impressão digital permite detectar a reutilização incompatível de um identificador.

### 8.4. Regras de deduplicação

| Situação | Comportamento |
|---|---|
| Mesmo ID e mesmos parâmetros | Reproduz a resposta armazenada |
| Mesmo ID e parâmetros diferentes | Rejeita com `OPERATION_ID_CONFLICT` |
| ID ausente ou tamanho inválido | Rejeita com `INVALID_OPERATION_ID` |
| Operação original ainda em andamento | A nova tentativa aguarda a conclusão e recebe resultado equivalente |
| Capacidade esgotada | Rejeita novas operações com `IDEMPOTENCY_CAPACITY_REACHED` |

Quando uma operação duplicada é reconhecida, o servidor não executa novamente a mutação.

A resposta reproduzida utiliza o `request_id` da tentativa atual, preservando a correlação entre a solicitação e a resposta na conexão atual.

### 8.5. Retenção e capacidade

A configuração padrão do mecanismo possui:

| Parâmetro | Valor padrão |
|---|---:|
| Tamanho do `OperationID` | 16 bytes |
| Retenção de operações concluídas | 24 horas |
| Capacidade de registros | 100.000 operações |

A retenção de 24 horas começa após a conclusão da operação.

Operações em andamento não expiram enquanto estiverem sendo executadas.

A capacidade considera tanto operações em andamento quanto operações concluídas ainda retidas.

Quando a capacidade está esgotada:

- Novos identificadores são rejeitados.
- A operação não é executada.
- O identificador rejeitado não é reservado.
- Uma nova tentativa poderá ser realizada quando houver capacidade disponível.

O servidor não remove antecipadamente registros válidos apenas para liberar espaço.

Os valores apresentados são os padrões da implementação atual. Não representam parâmetros configuráveis por variável de ambiente ou pela API pública do servidor.

### 8.6. Erros relacionados à idempotência

| Código | Significado |
|---|---|
| `INVALID_OPERATION_ID` | Identificador ausente ou com tamanho diferente de 16 bytes |
| `OPERATION_ID_CONFLICT` | Identificador já utilizado com parâmetros diferentes |
| `IDEMPOTENCY_CAPACITY_REACHED` | Capacidade de registros esgotada |
| `OPERATION_OUTCOME_UNKNOWN` | O servidor não consegue determinar o resultado da execução |

O código `OPERATION_OUTCOME_UNKNOWN` é um erro estruturado do protocolo, retornado pelo servidor. Ele não deve ser confundido com o tipo `OperationOutcomeUnknownError` do driver Go, que representa uma falha observada pelo cliente ao não conseguir confirmar o resultado da comunicação.

### 8.7. Limites da garantia

O mecanismo oferece deduplicação enquanto o registro da operação permanece retido na memória da instância.

Não existe persistência dos registros de idempotência. Consequentemente:

- Uma reinicialização elimina o histórico de operações.
- Após a expiração do registro, o mesmo identificador poderá ser tratado como uma nova operação.
- Uma operação cujo resultado permaneça indeterminado pode manter seu registro ocupando capacidade.
- Não existe garantia de execução exatamente uma vez diante de crash, reinicialização ou perda do estado em memória.

A garantia é limitada à prevenção de execuções duplicadas para o mesmo `OperationID` enquanto o registro correspondente permanece disponível na mesma instância.

## 9. Concorrência e ciclo de vida do client

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
| `open` | Disponível para executar operações |
| `closed` | Encerrado por `Close()` |
| `failed` | Invalidado após falha de comunicação ou protocolo |

Clientes nos estados `closed` e `failed` não devem ser reutilizados.

Após uma falha que invalide o client, crie uma nova conexão com `Dial` ou `DialContext`.

A idempotência server-side não altera o comportamento de serialização das operações na conexão. Ela permite reconhecer novas tentativas de uma operação lógica mesmo quando realizadas por uma nova conexão, desde que o registro correspondente ainda esteja retido na mesma instância do servidor.

## 10. Tratamento de erros

O driver diferencia erros estruturados retornados pelo servidor de falhas de comunicação observadas pelo cliente.

### 10.1. ServerError

`ServerError` representa um erro estruturado recebido do servidor.

O tipo disponibiliza:

- `Code`: código do erro.
- `Message`: descrição do erro.

Exemplos de códigos utilizados pela implementação:

| Código | Significado |
|---|---|
| `INVALID_REQUEST` | Solicitação inválida ou comando ausente/não suportado |
| `INVALID_ARGUMENT` | Argumento inválido, como chave vazia |
| `UNSUPPORTED_FEATURE` | Funcionalidade não suportada, como TTL positivo |
| `INVALID_OPERATION_ID` | Identificador de operação inválido |
| `OPERATION_ID_CONFLICT` | Reutilização de identificador com parâmetros diferentes |
| `IDEMPOTENCY_CAPACITY_REACHED` | Capacidade de registros de idempotência esgotada |
| `OPERATION_OUTCOME_UNKNOWN` | Resultado da execução não pode ser determinado |

Quando o servidor retorna uma resposta estruturada de erro, o driver disponibiliza `ServerError` e não invalida automaticamente a conexão.

### 10.2. OperationOutcomeUnknownError

`OperationOutcomeUnknownError` é um tipo de erro do driver Go. Ele representa uma situação em que o cliente não consegue determinar se o servidor executou a operação.

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

Para SET e DELETE, o MemoryDB oferece idempotência server-side por meio do `operation_id`. Quando o resultado de uma dessas operações for desconhecido, o consumidor pode realizar uma nova tentativa utilizando:

- Uma nova conexão com o servidor, caso o client anterior tenha sido invalidado.
- O mesmo `OperationID` utilizado na tentativa original.
- Os mesmos parâmetros da operação original.

Se o servidor ainda possuir o registro, reconhecerá a operação duplicada e reproduzirá a resposta armazenada, sem executar novamente a mutação.

Essa recuperação depende de o registro original continuar disponível na memória da mesma instância. A retenção é de 24 horas após a conclusão, e os registros são perdidos em caso de reinicialização.

O driver não executa retry automático. A geração e a reutilização do `OperationID` são responsabilidade da aplicação consumidora.

**Importante:** não gere um novo `OperationID` para repetir uma operação cujo resultado seja desconhecido. Um novo identificador representa uma nova operação lógica e não oferece a proteção contra duplicação da tentativa anterior.

### 10.3. Cancelamento e falhas de comunicação

O comportamento do driver depende do momento da interrupção:

| Situação | Comportamento |
|---|---|
| Falha ao estabelecer conexão | Retorna erro na inicialização |
| Erro estruturado do servidor | Retorna `ServerError`; client não é invalidado automaticamente |
| Cancelamento enquanto aguarda acesso à conexão | Operação cancelada; conexão preservada |
| Falha de comunicação durante I/O | Client passa para `failed`; resultado pode ser desconhecido |
| Cancelamento durante I/O | Conexão encerrada; client passa para `failed` |
| Resposta inválida ou incompatível | Client é invalidado; resultado pode ser desconhecido |
| Uso após `Close()` | Operação rejeitada |
| Uso após falha de comunicação | Operação rejeitada |

Depois de uma falha de comunicação ou cancelamento durante I/O, não reutilize o client.

## 11. Erros comuns e diagnóstico

| Situação | Causa provável | Ação recomendada |
|---|---|---|
| Servidor não está ativo | Processo não iniciado ou encerrado | Inicie o servidor e verifique o endereço |
| Conexão recusada | Endereço ou porta incorretos, ou serviço indisponível | Confira `MEMORYDB_ADDR` e o endereço utilizado pelo client |
| Porta ocupada | Outro processo está utilizando a porta | Escolha outra porta ou encerre o processo conflitante |
| Chave vazia | SET, GET ou DELETE recebeu uma chave vazia | Envie uma chave não vazia |
| TTL positivo | Funcionalidade ainda não implementada | Utilize `ttl_ms = 0` |
| OperationID inválido | Identificador ausente ou diferente de 16 bytes | Gere um identificador com `memorydb.NewOperationID()` |
| OperationID em conflito | Mesmo identificador utilizado com parâmetros diferentes | Confira os parâmetros; use outro identificador somente para uma nova operação lógica |
| Capacidade de idempotência esgotada | Limite de registros atingido | Aguarde a expiração de registros concluídos antes de tentar novamente |
| Frame com tamanho zero | Payload inválido | Envie um payload Protobuf válido |
| Frame maior que 1 MiB | Payload excede o limite do transporte | Reduza o tamanho da mensagem |
| Frame incompleto | A mensagem não foi transmitida integralmente | Confira o tamanho declarado e o envio dos bytes |
| Payload Protobuf inválido | Serialização incompatível ou mensagem malformada | Confira o schema e a serialização |
| Chave inexistente em GET | Chave não está armazenada | Verifique `found`; isso não representa erro |
| DELETE de chave inexistente | Chave não estava armazenada | Verifique `deleted`; isso não representa erro |
| Contexto cancelado antes do I/O | Cancelamento ou deadline antes do início da comunicação | Verifique o prazo e o estado do contexto |
| Contexto cancelado durante I/O | Comunicação interrompida | Crie outro client; para SET ou DELETE, reutilize o mesmo `OperationID` e os mesmos parâmetros |
| `OperationOutcomeUnknownError` | Cliente não conseguiu confirmar o resultado | Para SET ou DELETE, considere uma nova tentativa com o mesmo `OperationID` e parâmetros, desde que o registro ainda possa estar retido |
| Client fechado ou com falha | Ciclo de vida encerrado ou conexão invalidada | Crie um novo client |

Erros de framing e decodificação não são necessariamente convertidos em `ErrorResult`. Em geral, eles encerram o processamento da conexão.

Erros de conexão podem apresentar mensagens específicas do sistema operacional.

## 12. Limitações conhecidas

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
- **Sem retry automático:** comandos não são repetidos após falhas de comunicação. A aplicação consumidora controla novas tentativas.
- **Idempotência limitada:** o mecanismo está disponível somente para SET e DELETE.
- **Retenção de idempotência:** registros concluídos são mantidos por 24 horas após a conclusão.
- **Capacidade de idempotência:** a configuração padrão permite até 100.000 registros por instância, incluindo operações em andamento.
- **Sem persistência da idempotência:** os registros são mantidos somente em memória e são perdidos quando o processo termina ou reinicia.
- **Sem garantia após expiração:** depois da remoção de um registro expirado, o mesmo `OperationID` poderá ser tratado como uma nova operação.
- **Sem garantia de execução exatamente uma vez:** a proteção contra duplicação é limitada à existência do registro na memória da instância.
- **Resultado indeterminado:** uma interrupção durante a comunicação pode impedir que o cliente saiba se o comando foi executado.
- **Cliente de demonstração com endereço fixo:** utiliza `127.0.0.1:6380`.
- **Sem interface HTTP ou REST:** o protocolo disponível é TCP com framing e Protobuf.
- **Sem driver Java implementado e validado nesta etapa:** a integração Java não faz parte do escopo funcional atual.

Não utilize o MemoryDB como armazenamento durável nem exponha o serviço a redes não confiáveis enquanto os mecanismos de segurança necessários não estiverem implementados.

## 13. Perguntas frequentes

### Os dados ficam disponíveis para mais de um cliente?

Sim. Clientes conectados à mesma instância utilizam o mesmo armazenamento em memória. Cada processo do servidor possui seu próprio storage. Instâncias independentes não compartilham automaticamente os dados.

### Os dados continuam disponíveis após reiniciar?

Não. O armazenamento atual é exclusivamente em memória. Os registros de idempotência também são perdidos quando o processo termina ou reinicia.

### Posso utilizar TTL?

Ainda não. O campo `ttl_ms` está presente no contrato Protobuf, mas valores positivos são rejeitados. Utilize zero.

### O servidor oferece API REST?

Não. A implementação atual utiliza TCP com frames de tamanho prefixado e payload Protobuf.

### Posso enviar JSON diretamente pelo TCP?

Não. O servidor espera frames contendo mensagens serializadas conforme o contrato Protobuf.

### Posso utilizar outro cliente além do Go?

Sim, desde que o cliente implemente corretamente o framing TCP e o contrato Protobuf. Para SET e DELETE, também é necessário enviar um `operation_id` de 16 bytes e respeitar as regras de idempotência.

Isso não significa que já exista um driver pronto para todas as linguagens.

### Posso cancelar uma operação?

Sim. O driver Go disponibiliza métodos com `context.Context`.

Se o cancelamento ocorrer enquanto o client aguarda para iniciar uma operação, a conexão permanece disponível. Se ocorrer durante a comunicação de rede, a conexão é encerrada porque o resultado da operação pode ser desconhecido.

Para SET e DELETE, uma nova tentativa pode utilizar o mesmo `OperationID` e os mesmos parâmetros da operação original, desde que o registro ainda esteja retido.

### O contexto de DialContext controla as operações posteriores?

Não. Ele controla somente o estabelecimento da conexão. Para controlar operações posteriores, utilize `PingContext`, `SetContext`, `GetContext` ou `DeleteContext`.

### O driver reconecta automaticamente?

Não. Após falha de comunicação ou cancelamento durante I/O, é necessário criar um novo client.

### Posso executar várias operações simultaneamente no mesmo client?

As chamadas podem partir de goroutines diferentes, mas o driver serializa as operações na mesma conexão. O servidor atende múltiplas conexões simultaneamente.

### O que é OperationID?

É um identificador de 16 bytes utilizado para reconhecer uma operação lógica nas operações SET e DELETE. O driver disponibiliza `memorydb.NewOperationID()` para gerar identificadores. Cada operação lógica deve possuir seu próprio identificador.

### Preciso gerar um novo OperationID a cada tentativa?

Não. Para repetir uma operação cujo resultado seja desconhecido, reutilize o mesmo identificador e os mesmos parâmetros da tentativa original. Gere um novo identificador somente para uma nova operação lógica.

### Por quanto tempo o servidor mantém um OperationID?

O servidor mantém os registros de operações concluídas por 24 horas após sua conclusão. Operações em andamento não expiram enquanto estiverem sendo executadas.

### Existe limite de OperationIDs?

Sim. A capacidade padrão é de 100.000 registros por instância, incluindo operações em andamento. Quando a capacidade está esgotada, novas operações são rejeitadas até que haja espaço disponível. O identificador rejeitado não é reservado.

### O que acontece quando a capacidade de idempotência está esgotada?

O servidor rejeita novas operações com `IDEMPOTENCY_CAPACITY_REACHED`, sem executar a mutação e sem reservar o identificador rejeitado. Uma nova tentativa poderá ser realizada quando houver capacidade disponível.

### GET e PING também são idempotentes?

Não utilizam o mecanismo de deduplicação por `OperationID`. Esse mecanismo está disponível somente para SET e DELETE.

### O que acontece se reutilizar um OperationID com parâmetros diferentes?

O servidor rejeita a operação com `OPERATION_ID_CONFLICT`. O identificador não pode ser utilizado para representar operações lógicas diferentes enquanto seu registro estiver retido.

### A idempotência funciona após reiniciar o servidor?

Não. Os registros são mantidos somente em memória. Após reiniciar o processo, o servidor perde o histórico de operações e não consegue reconhecer identificadores utilizados anteriormente.

### O que significa OperationOutcomeUnknownError?

É um erro do driver Go que indica que o cliente não conseguiu confirmar se o servidor executou a operação.

Para SET e DELETE, a aplicação pode realizar uma nova tentativa utilizando o mesmo `OperationID` e os mesmos parâmetros da operação original. O servidor poderá reconhecer a duplicata e reproduzir a resposta anterior, desde que o registro ainda esteja retido. Utilize uma nova conexão se o client anterior tiver sido invalidado.

Não gere um novo `OperationID` para repetir a mesma operação lógica. Para PING e GET, não existe deduplicação por `OperationID`.

### O que significa OPERATION_OUTCOME_UNKNOWN?

É um código de erro estruturado do protocolo, retornado pelo servidor quando o mecanismo de execução não consegue determinar o resultado da operação. Não é o mesmo que o tipo `OperationOutcomeUnknownError` do driver, que representa a impossibilidade de o cliente confirmar o resultado da comunicação.

### O que acontece quando o servidor é encerrado?

O servidor fecha o listener e aguarda as conexões ativas, respeitando o timeout de 10 segundos. Se o contexto expirar, o executável trata o erro e encerra pelo caminho de falha. O shutdown não força diretamente o fechamento das conexões ativas.

Os dados em memória e os registros de idempotência são perdidos quando o processo termina.

Para executar os testes automatizados, também são necessários:

- Bash disponível no ambiente.

## 14. Executar testes e validar o projeto

O MemoryDB disponibiliza scripts para executar os testes automatizados e as verificações técnicas nos módulos Go do projeto.

Na raiz do repositório, execute:

```bash
./scripts/test.sh
```

Esse comando executa o pipeline completo, composto pelas seguintes etapas:

1. **format:** verifica a formatação dos arquivos Go com `gofmt`.
2. **vet:** executa análise estática com `go vet`.
3. **unit:** executa os testes unitários.
4. **race:** executa os testes com detector de condições de corrida.
5. **build:** verifica a compilação dos módulos.
6. **coverage:** executa os testes e gera relatórios de cobertura.
7. **uncovered:** identifica arquivos e blocos de código não cobertos pelos testes.

O pipeline percorre os cinco módulos Go do workspace e interrompe a execução quando alguma etapa falha.

### 14.1. Executar uma etapa específica

Também é possível executar cada etapa individualmente:

```bash
./scripts/test.sh format
./scripts/test.sh vet
./scripts/test.sh unit
./scripts/test.sh race
./scripts/test.sh build
./scripts/test.sh coverage
./scripts/test.sh uncovered
```

Para consultar os comandos disponíveis:

```bash
./scripts/test.sh help
```

### 14.2. Relatórios de cobertura

A etapa `coverage` gera os perfis de cobertura dos módulos no diretório `coverage/` e um relatório HTML do servidor:

```text
coverage/server.html
```

A etapa `uncovered` utiliza os perfis existentes para identificar blocos não cobertos. Ela não executa os testes novamente.

Para obter resultados atualizados, execute `coverage` antes de `uncovered`.

Os relatórios são artefatos locais de execução e não devem ser versionados no Git.

### 14.3. Documentação dos testes

O detalhamento dos comandos, módulos verificados, critérios de validação e interpretação dos resultados está disponível no [Guia de testes](../tecnica/TESTES.md)

## 15. Referências técnicas

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
| Idempotência | `server/internal/idempotency/executor.go` |
| Fingerprint de operações | `server/internal/idempotency/fingerprint.go` |
| Handler TCP | `server/internal/transport/tcp/handler.go` |
| Servidor TCP | `server/internal/transport/tcp/server.go` |
| Framing | `transport/tcp/go/framing.go` |
| Conexão TCP | `transport/tcp/go/connection.go` |
| Driver Go | `drivers/go/client.go` |
| OperationID do driver | `drivers/go/operation_id.go` |
| Testes do driver Go | `drivers/go/client_test.go` |
| Cliente de demonstração | `api/client/main.go` |
| Executável do servidor | `server/cmd/memorydb/main.go` |
| Workspace Go | `go.work` |
| Guia específico do driver Go | `drivers/go/README.md` |
| Roadmap | `memorydb-roadmap.md` |
| Guia de testes | `docs/tecnica/TESTES.md` |
| Scripts de validação | `scripts/test.sh` e `scripts/test/` |

## 16. Referências externas

- Repositório: https://github.com/pescador95/memorydb
- Documentação oficial do Go: https://go.dev/doc/
- Download do Go: https://go.dev/dl/
- Gerenciamento de módulos Go: https://go.dev/doc/modules/managing-dependencies
- Pacote `context`: https://pkg.go.dev/context
- Pacote `errors`: https://pkg.go.dev/errors
- Pacote `crypto/rand`: https://pkg.go.dev/crypto/rand
- Protocol Buffers: https://protobuf.dev/

---

**MemoryDB**

Servidor chave-valor em memória com comunicação TCP e protocolo Protocol Buffers.