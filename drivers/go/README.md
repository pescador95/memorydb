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

**Importante:** o driver é uma biblioteca cliente. A aplicação consumidora precisa ter acesso a uma instância do servidor MemoryDB em execução.

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
go get github.com/pescador95/memorydb/drivers/go@vX.Y.Z
```

Substitua `vX.Y.Z` pela versão publicada que deseja utilizar.

Em seguida, organize as dependências:

```bash
go mod tidy
```

**Atenção:** o MemoryDB utiliza módulos Go independentes. Para que a instalação funcione fora do repositório original, as dependências do driver, incluindo os módulos de transporte TCP e Protobuf, também precisam estar publicadas e acessíveis nas versões declaradas pelo driver.

O arquivo `go.work` utilizado no desenvolvimento do monorepo não é necessário na aplicação consumidora.

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
        log.Fatal(err)
    }
    defer client.Close()

    log.Println("Conectado ao MemoryDB")
}
```

O endereço `127.0.0.1:6380` é um exemplo. Utilize o endereço e a porta da instância do MemoryDB à qual a aplicação terá acesso.

O `DialContext` utiliza o contexto durante o estabelecimento da conexão. Ele não mantém esse contexto associado às operações posteriores.

## 6. Operações disponíveis

As operações podem ser executadas por métodos convencionais ou por métodos que recebem `context.Context`.

| Operação | Método convencional | Método com contexto |
|---|---|---|
| Verificar conexão | `Ping()` | `PingContext(ctx)` |
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

func main() {
    ctx, cancel := context.WithTimeout(
        context.Background(),
        5*time.Second,
    )
    defer cancel()

    client, err := memorydb.DialContext(ctx, "127.0.0.1:6380")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Verificar conexão.
    if err := client.PingContext(ctx); err != nil {
        log.Fatal(err)
    }

    key := "usuario:123"

    // CREATE: armazenar um valor.
    if err := client.SetContext(ctx, key, []byte("João")); err != nil {
        log.Fatal(err)
    }

    fmt.Println("Valor armazenado")

    // READ: consultar o valor.
    value, found, err := client.GetContext(ctx, key)
    if err != nil {
        log.Fatal(err)
    }

    if found {
        fmt.Printf("Valor encontrado: %s\n", value)
    }

    // UPDATE: sobrescrever o valor da chave.
    if err := client.SetContext(ctx, key, []byte("Maria")); err != nil {
        log.Fatal(err)
    }

    fmt.Println("Valor atualizado")

    // READ: consultar o valor atualizado.
    value, found, err = client.GetContext(ctx, key)
    if err != nil {
        log.Fatal(err)
    }

    if found {
        fmt.Printf("Novo valor: %s\n", value)
    }

    // DELETE: remover a chave.
    deleted, err := client.DeleteContext(ctx, key)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Chave removida: %t\n", deleted)

    // READ: confirmar a remoção.
    _, found, err = client.GetContext(ctx, key)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Chave encontrada após exclusão: %t\n", found)
}
```

**Observação:** o MemoryDB disponibiliza operações chave-valor. Portanto, UPDATE é realizado por meio de um novo SET sobre a mesma chave.

## 8. Integração com uma API HTTP

Em aplicações HTTP, recomenda-se criar uma única instância do client durante a inicialização da aplicação e compartilhá-la entre os handlers.

O driver serializa operações concorrentes na mesma conexão. Portanto, compartilhar o client é permitido, mas as operações dessa conexão são executadas uma por vez.

Exemplo:

```go
package main

import (
    "context"
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

func (api *API) Create(w http.ResponseWriter, r *http.Request) {
    key := r.PathValue("key")

    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    err := api.client.SetContext(ctx, key, []byte("valor"))
    if err != nil {
        http.Error(w, "Erro ao armazenar valor", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusCreated)
}

func (api *API) Read(w http.ResponseWriter, r *http.Request) {
    key := r.PathValue("key")

    value, found, err := api.client.GetContext(r.Context(), key)
    if err != nil {
        http.Error(w, "Erro ao consultar valor", http.StatusInternalServerError)
        return
    }

    if !found {
        http.NotFound(w, r)
        return
    }

    w.Write(value)
}

func (api *API) Update(w http.ResponseWriter, r *http.Request) {
    key := r.PathValue("key")

    err := api.client.SetContext(r.Context(), key, []byte("novo valor"))
    if err != nil {
        http.Error(w, "Erro ao atualizar valor", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

func (api *API) Delete(w http.ResponseWriter, r *http.Request) {
    key := r.PathValue("key")

    deleted, err := api.client.DeleteContext(r.Context(), key)
    if err != nil {
        http.Error(w, "Erro ao excluir valor", http.StatusInternalServerError)
        return
    }

    if !deleted {
        http.NotFound(w, r)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    client, err := memorydb.DialContext(ctx, "127.0.0.1:6380")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    api := NewAPI(client)

    mux := http.NewServeMux()
    mux.HandleFunc("POST /memory/{key}", api.Create)
    mux.HandleFunc("GET /memory/{key}", api.Read)
    mux.HandleFunc("PUT /memory/{key}", api.Update)
    mux.HandleFunc("DELETE /memory/{key}", api.Delete)

    log.Println("API disponível em :8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

O exemplo utiliza `net/http` da biblioteca padrão do Go e os métodos HTTP POST, GET, PUT e DELETE.

Em uma aplicação real, os valores devem ser recebidos e validados conforme o contrato da API, em vez de serem fixados nos handlers.

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

**Comportamento do driver:**

- Cancelamento enquanto aguarda a liberação da conexão: a operação é cancelada sem invalidar o client.
- Cancelamento durante uma operação de I/O: a conexão é encerrada e o client passa para o estado `failed`.
- Após uma falha durante I/O, não é possível garantir se o servidor executou o comando antes de a conexão ser interrompida.

Por esse motivo, não implemente retries automáticos indiscriminados para operações cujo resultado ficou incerto.

## 10. Concorrência e compartilhamento do client

O client pode ser compartilhado entre goroutines.

Entretanto, as operações são serializadas por conexão. Isso significa que chamadas concorrentes utilizando a mesma instância não são executadas simultaneamente no servidor.

Características:

- Uma operação por vez em cada conexão.
- Leituras e escritas concorrentes na aplicação são coordenadas pelo driver.
- O client deve possuir um ciclo de vida gerenciado pela aplicação.
- Não é necessário criar uma conexão para cada requisição HTTP.

O uso de múltiplas conexões pode ser considerado futuramente quando houver necessidade de maior paralelismo.

## 11. Tratamento de erros

O driver diferencia erros de comunicação de erros retornados pelo servidor.

Os principais cenários são:

| Situação | Comportamento |
|---|---|
| Falha ao estabelecer conexão | Retorna erro na inicialização |
| Erro retornado pelo servidor | Retorna erro sem invalidar automaticamente o client |
| Falha de comunicação | Client passa para `failed` |
| Cancelamento durante espera pelo acesso à conexão | Operação cancelada, conexão preservada |
| Cancelamento durante I/O | Conexão encerrada e client invalidado |
| Uso após `Close()` | Operação rejeitada |
| Uso após falha de comunicação | Operação rejeitada |

O driver também disponibiliza `ServerError` para representar erros estruturados retornados pelo servidor.

A aplicação deve tratar os erros conforme o contexto da operação, evitando expor detalhes internos do MemoryDB diretamente aos consumidores da API.

## 12. Encerramento da conexão

Ao finalizar a aplicação, feche o client:

```go
defer client.Close()
```

O fechamento encerra a conexão TCP associada ao client.

Como o client representa uma conexão e não um pool de conexões, sua inicialização e seu encerramento devem ser gerenciados pelo ciclo de vida da aplicação.

## 13. Limitações atuais

A implementação atual do driver possui as seguintes características:

- Uma conexão TCP por instância do client.
- Operações serializadas por conexão.
- Sem multiplexação de requisições.
- Sem reconexão automática.
- Sem retry automático.
- Sem pool de conexões integrado.
- Cancelamento durante I/O invalida o client.
- O resultado de uma operação pode ser incerto quando a conexão é interrompida durante a comunicação.

As funcionalidades de persistência, TTL, autenticação e TLS dependem da implementação correspondente no servidor e não devem ser presumidas como disponíveis.

## 14. Referências

- Repositório: https://github.com/pescador95/memorydb
- Documentação oficial do Go: https://go.dev/doc/
- Download do Go: https://go.dev/dl/
- Gerenciamento de módulos Go: https://go.dev/doc/modules/managing-dependencies

---

**MemoryDB — Driver Go**

Biblioteca cliente para integração de aplicações Go com o servidor MemoryDB.