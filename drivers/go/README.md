# MemoryDB — Driver Go

Guia de instalação e utilização do driver Go do MemoryDB em aplicações independentes.

## 1. Visão geral

O driver Go permite que aplicações desenvolvidas em Go se conectem a uma instância do MemoryDB por TCP e executem operações de leitura, escrita e exclusão de dados.

O driver disponibiliza:

- Conexão com o servidor MemoryDB.
- PING para verificar a comunicação.
- SET para armazenar valores.
- GET para consultar valores.
- DELETE para remover valores.
- Operações com suporte a `context.Context`.
- Serialização de operações concorrentes por conexão.
- Tratamento de erros de comunicação e erros retornados pelo servidor.
- Identificação de operações cujo resultado não pôde ser confirmado.

**Importante:** o driver é uma biblioteca cliente. A aplicação consumidora precisa ter acesso a uma instância do servidor MemoryDB em execução.

O driver não implementa o armazenamento dos dados. As operações são encaminhadas ao servidor, que mantém os valores em memória.

## 2. Pré-requisitos

Antes de utilizar o driver, é necessário ter:

- Go instalado.
- Um projeto Go inicializado com `go.mod`.
- Acesso a uma instância do servidor MemoryDB.
- Conectividade TCP com o endereço configurado no servidor.

## 3. Instalação do Go

O Go pode ser instalado em Windows, Linux ou macOS.

Download oficial:

https://go.dev/dl/

### 3.1. Windows

1. Acesse https://go.dev/dl/.
2. Baixe o instalador `.msi` para Windows.
3. Execute o instalador e siga as instruções.
4. Abra um novo terminal para carregar as configurações de ambiente.

Verifique a instalação:

```bash
go version
```

### 3.2. Linux

Acesse https://go.dev/dl/ e baixe o arquivo de instalação correspondente à arquitetura do sistema.

Extraia o pacote no diretório recomendado pela documentação oficial e configure o PATH para incluir `/usr/local/go/bin`.

Verifique a instalação:

```bash
go version
```

### 3.3. macOS

Acesse https://go.dev/dl/, baixe o instalador `.pkg` correspondente à arquitetura do equipamento e execute-o.

Verifique a instalação:

```bash
go version
```

### 3.4. Versão necessária

Utilize uma versão do Go igual ou superior à versão declarada na diretiva `go` do arquivo `drivers/go/go.mod` do MemoryDB.

A versão instalada pode ser verificada com:

```bash
go version
```

## 4. Instalação do driver em uma aplicação Go

O driver é disponibilizado como um módulo Go independente.

Seu caminho de importação é:

```go
github.com/pescador95/memorydb/drivers/go
```

### 4.1. Inicializar uma aplicação

Crie um diretório para a aplicação:

```bash
mkdir minha-api
cd minha-api
```

Inicialize o módulo:

```bash
go mod init minha-api
```

### 4.2. Instalar o driver

Após a publicação de uma versão do driver e de suas dependências, execute:

```bash
go get github.com/pescador95/memorydb/drivers/go@v0.1.0
```

Substitua `v0.1.0` pela versão publicada que deseja utilizar.

Em seguida, organize as dependências:

```bash
go mod tidy
```

**Atenção:** o MemoryDB utiliza módulos Go independentes. Para que a instalação funcione fora do repositório original, as dependências do driver, incluindo os módulos de transporte TCP e Protobuf, também precisam estar publicadas e acessíveis nas versões declaradas pelo driver.

O arquivo `go.work` utilizado no desenvolvimento do monorepo não é necessário na aplicação consumidora. Ele é utilizado para facilitar o desenvolvimento conjunto dos módulos locais.

### 4.3. Importar a biblioteca

```go
import (
    memorydb "github.com/pescador95/memorydb/drivers/go"
)
```

O alias `memorydb` facilita a identificação dos tipos e funções disponibilizados pelo driver.

## 5. Conectar ao servidor

O driver disponibiliza `Dial` e `DialContext` para estabelecer uma conexão com o servidor.

Exemplo utilizando contexto e timeout:

```go
package main

import (
    "context"
    "log"
    "time"

    memorydb "github.com/pescador95/memorydb/drivers/go"
)

func main() {
    ctx, cancel := context.WithTimeout(
        context.Background(),
        5*time.Second,
    )
    defer cancel()

    client, err := memorydb.DialContext(ctx, "127.0.0.1:6380")
    if err != nil {
        log.Printf("Erro ao conectar ao MemoryDB: %v", err)
        return
    }
    defer client.Close()

    log.Println("Conectado ao MemoryDB")
}
```

O endereço `127.0.0.1:6380` é um exemplo. Utilize o endereço e a porta da instância do MemoryDB à qual a aplicação terá acesso.

O `DialContext` utiliza o contexto durante o estabelecimento da conexão. Ele não mantém esse contexto associado às operações posteriores. Para controlar cada operação, utilize os métodos que recebem `context.Context`.

O `Close` encerra a conexão TCP associada ao client.

## 6. Operações disponíveis

As operações podem ser executadas por métodos convencionais ou por métodos que recebem `context.Context`.

| Operação | Método convencional | Método com contexto |
|---|---|---|
| Verificar comunicação | `Ping()` | `PingContext(ctx)` |
| Armazenar | `Set(key, value)` | `SetContext(ctx, key, value)` |
| Consultar | `Get(key)` | `GetContext(ctx, key)` |
| Excluir | `Delete(key)` | `DeleteContext(ctx, key)` |

Para aplicações HTTP e serviços, recomenda-se utilizar as variantes com contexto.

### 6.1. PING

Verifica a comunicação com o servidor:

```go
if err := client.PingContext(ctx); err != nil {
    log.Printf("Falha na comunicação: %v", err)
    return
}
```

O PING verifica se o servidor responde à solicitação. Ele não verifica a disponibilidade de funcionalidades adicionais, persistência ou capacidade de processamento.

### 6.2. SET — armazenar um valor

Armazena um valor associado a uma chave:

```go
err := client.SetContext(
    ctx,
    "usuario:123",
    []byte("João"),
)
if err != nil {
    log.Printf("Erro ao armazenar: %v", err)
    return
}
```

Os valores são enviados como `[]byte`.

Quando uma chave já existe, um novo SET sobrescreve seu valor.

### 6.3. GET — consultar um valor

Consulta uma chave e informa se ela foi encontrada:

```go
value, found, err := client.GetContext(ctx, "usuario:123")
if err != nil {
    log.Printf("Erro ao consultar: %v", err)
    return
}

if !found {
    log.Println("Chave não encontrada")
    return
}

log.Printf("Valor: %s", value)
```

O método retorna:

- `[]byte`: valor armazenado.
- `bool`: indica se a chave foi encontrada.
- `error`: erro durante a operação.

O indicador `found` permite distinguir uma chave inexistente de uma chave existente cujo valor é vazio.

### 6.4. DELETE — excluir um valor

Remove uma chave do armazenamento:

```go
deleted, err := client.DeleteContext(ctx, "usuario:123")
if err != nil {
    log.Printf("Erro ao excluir: %v", err)
    return
}

if !deleted {
    log.Println("Chave não encontrada")
    return
}

log.Println("Chave removida")
```

O método retorna:

- `bool`: indica se a chave foi removida.
- `error`: erro durante a operação.

## 7. Exemplo CRUD completo

O exemplo abaixo demonstra as operações de criação, leitura, atualização e exclusão utilizando o driver.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    memorydb "github.com/pescador95/memorydb/drivers/go"
)

func run() error {
    ctx, cancel := context.WithTimeout(
        context.Background(),
        5*time.Second,
    )
    defer cancel()

    client, err := memorydb.DialContext(ctx, "127.0.0.1:6380")
    if err != nil {
        return fmt.Errorf("conectar ao MemoryDB: %w", err)
    }
    defer client.Close()

    // Verificar comunicação.
    if err := client.PingContext(ctx); err != nil {
        return fmt.Errorf("executar PING: %w", err)
    }

    key := "usuario:123"

    // CREATE: armazenar um valor.
    if err := client.SetContext(ctx, key, []byte("João")); err != nil {
        return fmt.Errorf("armazenar valor: %w", err)
    }

    fmt.Println("Valor armazenado")

    // READ: consultar o valor.
    value, found, err := client.GetContext(ctx, key)
    if err != nil {
        return fmt.Errorf("consultar valor: %w", err)
    }

    if found {
        fmt.Printf("Valor encontrado: %s\n", value)
    }

    // UPDATE: sobrescrever o valor da chave.
    if err := client.SetContext(ctx, key, []byte("Maria")); err != nil {
        return fmt.Errorf("atualizar valor: %w", err)
    }

    fmt.Println("Valor atualizado")

    // READ: consultar o valor atualizado.
    value, found, err = client.GetContext(ctx, key)
    if err != nil {
        return fmt.Errorf("consultar valor atualizado: %w", err)
    }

    if found {
        fmt.Printf("Novo valor: %s\n", value)
    }

    // DELETE: remover a chave.
    deleted, err := client.DeleteContext(ctx, key)
    if err != nil {
        return fmt.Errorf("excluir chave: %w", err)
    }

    fmt.Printf("Chave removida: %t\n", deleted)

    // READ: confirmar a remoção.
    _, found, err = client.GetContext(ctx, key)
    if err != nil {
        return fmt.Errorf("confirmar exclusão: %w", err)
    }

    fmt.Printf("Chave encontrada após exclusão: %t\n", found)

    return nil
}

func main() {
    if err := run(); err != nil {
        log.Printf("Erro na execução: %v", err)
    }
}
```

**Observação:** o MemoryDB disponibiliza operações chave-valor. Portanto, UPDATE é realizado por meio de um novo SET sobre a mesma chave.

O exemplo utiliza uma única janela de timeout para a sequência completa. Em aplicações reais, o prazo deve ser definido de acordo com o tempo esperado para cada operação ou fluxo de negócio.

## 8. Integração com uma API HTTP

Em aplicações HTTP, recomenda-se criar uma única instância do client durante a inicialização da aplicação e compartilhá-la entre os handlers.

O driver permite o compartilhamento do client entre goroutines. Entretanto, as operações são serializadas na conexão. Assim, chamadas concorrentes aguardam sua vez de execução.

Exemplo:

```go
package main

import (
    "context"
    "errors"
    "log"
    "net/http"
    "time"

    memorydb "github.com/pescador95/memorydb/drivers/go"
)

type API struct {
    client *memorydb.Client
}

func NewAPI(client *memorydb.Client) *API {
    return &API{
        client: client,
    }
}

func (api *API) handleMemoryDBError(
    w http.ResponseWriter,
    err error,
) {
    var outcomeErr *memorydb.OperationOutcomeUnknownError

    if errors.As(err, &outcomeErr) {
        log.Printf(
            "Resultado indeterminado no MemoryDB: request_id=%d: %v",
            outcomeErr.RequestID,
            err,
        )

        http.Error(
            w,
            "Não foi possível confirmar o resultado da operação",
            http.StatusServiceUnavailable,
        )
        return
    }

    log.Printf("Erro na operação MemoryDB: %v", err)

    http.Error(
        w,
        "Erro ao processar operação de armazenamento",
        http.StatusInternalServerError,
    )
}

func (api *API) Create(
    w http.ResponseWriter,
    r *http.Request,
) {
    key := r.PathValue("key")

    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    err := api.client.SetContext(ctx, key, []byte("valor"))
    if err != nil {
        api.handleMemoryDBError(w, err)
        return
    }

    w.WriteHeader(http.StatusCreated)
}

func (api *API) Read(
    w http.ResponseWriter,
    r *http.Request,
) {
    key := r.PathValue("key")

    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    value, found, err := api.client.GetContext(ctx, key)
    if err != nil {
        api.handleMemoryDBError(w, err)
        return
    }

    if !found {
        http.NotFound(w, r)
        return
    }

    if _, err := w.Write(value); err != nil {
        log.Printf("Erro ao escrever resposta HTTP: %v", err)
    }
}

func (api *API) Update(
    w http.ResponseWriter,
    r *http.Request,
) {
    key := r.PathValue("key")

    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    err := api.client.SetContext(ctx, key, []byte("novo valor"))
    if err != nil {
        api.handleMemoryDBError(w, err)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

func (api *API) Delete(
    w http.ResponseWriter,
    r *http.Request,
) {
    key := r.PathValue("key")

    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    deleted, err := api.client.DeleteContext(ctx, key)
    if err != nil {
        api.handleMemoryDBError(w, err)
        return
    }

    if !deleted {
        http.NotFound(w, r)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

func main() {
    client, err := memorydb.Dial("127.0.0.1:6380")
    if err != nil {
        log.Printf("Erro ao conectar ao MemoryDB: %v", err)
        return
    }
    defer client.Close()

    api := NewAPI(client)

    mux := http.NewServeMux()
    mux.HandleFunc("POST /memory/{key}", api.Create)
    mux.HandleFunc("GET /memory/{key}", api.Read)
    mux.HandleFunc("PUT /memory/{key}", api.Update)
    mux.HandleFunc("DELETE /memory/{key}", api.Delete)

    server := &http.Server{
        Addr:              ":8080",
        Handler:           mux,
        ReadHeaderTimeout: 5 * time.Second,
    }

    log.Println("API disponível em :8080")

    if err := server.ListenAndServe(); err != nil &&
        !errors.Is(err, http.ErrServerClosed) {
        log.Printf("Erro no servidor HTTP: %v", err)
    }
}
```

O exemplo utiliza `net/http` da biblioteca padrão do Go e os métodos HTTP POST, GET, PUT e DELETE.

A demonstração mantém o foco na integração com o driver. Em uma aplicação real:

- Os valores devem ser recebidos e validados conforme o contrato da API.
- A aplicação deve definir políticas apropriadas de timeout.
- O encerramento do servidor HTTP deve ser integrado ao mecanismo de shutdown da aplicação.
- Os erros devem ser classificados antes de serem convertidos em respostas HTTP.
- Uma operação com resultado indeterminado não deve ser apresentada como uma falha comprovada de execução.

O exemplo utiliza `Dial` para simplificar a inicialização. Em aplicações que precisam limitar o tempo de conexão, utilize `DialContext`.

## 9. Contexto, timeout e cancelamento

As operações com contexto permitem controlar o tempo de execução e interromper operações quando necessário.

Exemplo:

```go
ctx, cancel := context.WithTimeout(
    context.Background(),
    2*time.Second,
)
defer cancel()

value, found, err := client.GetContext(ctx, "usuario:123")
```

Em handlers HTTP, prefira propagar `r.Context()` para o driver, garantindo que o cancelamento da requisição seja propagado à operação.

### 9.1. Comportamento do driver

O comportamento depende do momento em que o cancelamento acontece:

- **Enquanto aguarda a conexão:** a operação é cancelada sem invalidar o client, pois ainda não começou a utilizar a conexão para executar a solicitação.
- **Durante uma operação de I/O:** se o cancelamento interromper a comunicação, a conexão é encerrada e o client passa para o estado `failed`.
- **Após a interrupção da comunicação:** não é possível garantir se o servidor executou o comando antes de a conexão ser interrompida.

Esse último cenário é chamado de resultado indeterminado.

### 9.2. OperationOutcomeUnknownError

O driver disponibiliza `OperationOutcomeUnknownError` para representar operações cujo resultado não pôde ser confirmado.

Esse erro contém:

- `RequestID`: identificador da solicitação associada à operação.
- `Cause`: causa da interrupção ou falha.

O tipo implementa `Unwrap()`, permitindo utilizar `errors.Is` e `errors.As` para examinar o erro original.

Exemplo de identificação:

```go
var outcomeErr *memorydb.OperationOutcomeUnknownError

if errors.As(err, &outcomeErr) {
    log.Printf(
        "Resultado indeterminado: request_id=%d",
        outcomeErr.RequestID,
    )

    if errors.Is(err, context.Canceled) {
        log.Println("A operação foi interrompida por cancelamento")
    }
}
```

**Importante:** `RequestID` é um identificador de correlação da solicitação. Ele não é uma chave de idempotência e não permite consultar ou recuperar o resultado de uma operação posteriormente.

O driver não implementa retry automático, deduplicação de comandos ou recuperação server-side do resultado.

Por isso, não implemente retries automáticos indiscriminados para operações cujo resultado ficou incerto. Repetir um comando sem garantias de idempotência pode produzir efeitos diferentes dos esperados pela aplicação.

## 10. Concorrência e compartilhamento do client

O client pode ser compartilhado entre goroutines.

Entretanto, as operações são serializadas por conexão. Isso significa que chamadas concorrentes utilizando a mesma instância não são executadas simultaneamente no servidor.

Características:

- Uma operação por vez em cada conexão.
- Leituras e escritas concorrentes na aplicação são coordenadas pelo driver.
- O client deve possuir um ciclo de vida gerenciado pela aplicação.
- Não é necessário criar uma conexão para cada requisição HTTP.
- Uma operação que aguarda a conexão pode ser cancelada sem interromper uma operação que já esteja em andamento.

O compartilhamento simplifica o gerenciamento de recursos e evita a criação desnecessária de conexões.

O uso de múltiplas conexões pode ser considerado futuramente quando houver necessidade de maior paralelismo. Isso exige gerenciamento adequado de conexões, concorrência, falhas e ciclo de vida.

## 11. Tratamento de erros

O driver diferencia erros de comunicação de erros estruturados retornados pelo servidor.

Os principais cenários são:

| Situação | Comportamento |
|---|---|
| Falha ao estabelecer conexão | Retorna erro na inicialização |
| Erro estruturado retornado pelo servidor | Retorna `ServerError` sem invalidar automaticamente o client |
| Falha de comunicação durante uma operação | Client passa para `failed` |
| Cancelamento durante espera pelo acesso à conexão | Operação cancelada, conexão preservada |
| Cancelamento durante I/O | Conexão encerrada e client invalidado |
| Falha que impede confirmar o resultado da operação | Retorna `OperationOutcomeUnknownError`, quando aplicável |
| Uso após `Close()` | Operação rejeitada |
| Uso após falha de comunicação | Operação rejeitada |

### 11.1. ServerError

O driver disponibiliza `ServerError` para representar erros estruturados retornados pelo servidor.

Esse tipo permite diferenciar uma rejeição da operação pelo servidor de uma falha na comunicação TCP.

No contrato atual, um erro estruturado representa uma operação rejeitada, e não uma operação executada cuja resposta foi perdida. A aplicação pode tratar esse erro sem invalidar automaticamente a conexão.

### 11.2. Erros de transporte e resultado indeterminado

Uma falha de transporte não significa necessariamente que o comando deixou de ser executado.

Por exemplo:

1. O client envia um SET.
2. O servidor recebe e executa o comando.
3. A conexão é interrompida antes de a resposta chegar ao client.
4. O client não consegue confirmar o resultado.

Nesse cenário, o servidor pode ter armazenado o valor, mesmo que o client tenha recebido um erro.

A aplicação deve tratar esse cenário separadamente de uma rejeição explícita do servidor.

**Recomendação:** não exponha detalhes internos do MemoryDB diretamente aos consumidores da API. Registre os detalhes técnicos nos logs e retorne mensagens apropriadas ao contrato da aplicação.

## 12. Encerramento da conexão

Ao finalizar a aplicação, feche o client:

```go
defer client.Close()
```

O fechamento encerra a conexão TCP associada ao client.

Como o client representa uma conexão e não um pool de conexões, sua inicialização e seu encerramento devem ser gerenciados pelo ciclo de vida da aplicação.

O encerramento também deve considerar operações em andamento. Fechar o client interrompe a conexão e pode fazer com que uma operação que esteja utilizando a conexão termine com erro.

## 13. Limitações atuais

A implementação atual do driver possui as seguintes características:

- Uma conexão TCP por instância do client.
- Operações serializadas por conexão.
- Sem multiplexação de requisições.
- Sem reconexão automática.
- Sem retry automático.
- Sem pool de conexões integrado.
- Cancelamento durante I/O pode invalidar o client.
- O resultado de uma operação pode ser incerto quando a conexão é interrompida durante a comunicação.
- Não existe mecanismo server-side de deduplicação ou recuperação de resultados por identificador de operação.

As funcionalidades de persistência, TTL, autenticação e TLS dependem da implementação correspondente no servidor e não devem ser presumidas como disponíveis.

O driver não oferece garantias de durabilidade dos dados. A disponibilidade e a retenção dos valores dependem do servidor MemoryDB.

## 14. Referências

- Repositório: https://github.com/pescador95/memorydb
- Documentação oficial do Go: https://go.dev/doc/
- Download do Go: https://go.dev/dl/
- Gerenciamento de módulos Go: https://go.dev/doc/modules/managing-dependencies
- Pacote `context`: https://pkg.go.dev/context
- Pacote `errors`: https://pkg.go.dev/errors
- Pacote `net/http`: https://pkg.go.dev/net/http

---

**MemoryDB — Driver Go**

Biblioteca cliente para integração de aplicações Go com o servidor MemoryDB.