# Dimensionamento e detecção de recursos

## 1. Objetivo e escopo

O pacote público `server/resources` calcula e valida a configuração de recursos utilizada na inicialização do MemoryDB.

Ele está dentro do módulo Go `server`, mas pode ser importado por aplicações Go externas por meio do caminho:

```go
github.com/pescador95/memorydb/server/resources
```

O pacote não é um serviço HTTP, não inicia processos e não altera limites do sistema operacional. Sua responsabilidade é:

1. detectar a capacidade de CPU e memória disponível para o processo;
2. selecionar um perfil de dimensionamento;
3. calcular os limites e orçamentos dos componentes;
4. aplicar substituições explícitas;
5. validar a configuração final.

A configuração é calculada no startup e entregue aos componentes que precisam dela. Não há redimensionamento dinâmico durante a execução.

## 2. Perfis de recursos

O perfil é selecionado por `MEMORYDB_RESOURCE_PROFILE`. Quando a variável está ausente ou vazia, o perfil utilizado é `mid`.

| Perfil | Estratégia |
|---|---|
| `low` | Preset determinístico de menor capacidade |
| `mid` | Preset determinístico intermediário e padrão |
| `high` | Preset determinístico de maior capacidade |
| `auto` | Detecta CPU e memória disponíveis e calcula os limites |
| `custom` | Recebe capacidade de CPU e orçamento de memória explicitamente informados |

### 2.1. Presets `low`, `mid` e `high`

Os três presets possuem valores fixos. Eles não são automaticamente reduzidos para caber no host ou container. Depois de selecionado o preset, o carregador compara CPU e orçamento de memória com os recursos detectados. Se o preset exceder a capacidade detectada, a inicialização falha com erro de recursos insuficientes.

| Parâmetro | `low` | `mid` | `high` |
|---|---:|---:|---:|
| Orçamento de memória do processo | 256 MiB | 1 GiB | 4 GiB |
| Capacidade de CPU | 1.000m | 2.000m | 4.000m |
| Storage — máximo de entradas | 100.000 | 500.000 | 2.000.000 |
| Storage — tamanho máximo da chave | 256 B | 512 B | 1 KiB |
| Storage — tamanho máximo do valor | 16 KiB | 64 KiB | 256 KiB |
| Idempotência — máximo de registros | 10.000 | 100.000 | 500.000 |
| Rede — conexões simultâneas | 256 | 1.024 | 4.096 |
| Rede — requisições em processamento | 64 | 256 | 1.024 |
| Rede — payload máximo do frame | 64 KiB | 256 KiB | 1 MiB |
| Retenção de idempotência | 24 h | 24 h | 24 h |

Os tamanhos de frame representam somente o payload. O cabeçalho TCP de framing, de 4 bytes, não faz parte desse valor.

### 2.2. Perfis `auto` e `custom`

`auto` utiliza a capacidade detectada de CPU e memória como entradas do algoritmo de dimensionamento.

`custom` utiliza os valores fornecidos pelo operador, mas exige que ambos sejam positivos e não ultrapassem a capacidade detectada. Os dois perfis utilizam o mesmo algoritmo de distribuição de memória e dimensionamento de entradas.

No cálculo de `auto` e `custom`, os valores de referência para limites de rede, tamanhos máximos de chave e valor e retenção vêm do preset `mid`. Os limites de quantidade de entradas de Storage e Idempotência são escalados proporcionalmente aos respectivos orçamentos de memória calculados.

Para `custom`, são obrigatórias as variáveis:

- `MEMORYDB_RESOURCE_CPU`;
- `MEMORYDB_RESOURCE_MEMORY`.

Essas duas variáveis não devem ser informadas nos perfis `low`, `mid`, `high` ou `auto`: nesses casos, os valores são definidos pelo preset ou pela detecção, e informar valores explícitos provoca erro de configuração.

Exemplo:

```bash
MEMORYDB_RESOURCE_PROFILE=custom \
MEMORYDB_RESOURCE_CPU=1500m \
MEMORYDB_RESOURCE_MEMORY=2GiB \
go run ./cmd/memorydb
```

A capacidade de CPU aceita millicores (`1500m`) ou cores (`1.5`, equivalente a `1500m`). O resultado precisa corresponder a millicores inteiros.

## 3. Orçamento de memória

O orçamento total do processo é dividido em quatro parcelas:

| Componente | Percentual | Responsabilidade |
|---|---:|---|
| Storage | 55% | Orçamento lógico de chaves e valores |
| Idempotência | 15% | Registros, metadados e resultados retidos |
| Processamento | 15% | Reservas lógicas para frames e processamento de requisições |
| Runtime | 15% | Parcela restante destinada ao runtime e demais estruturas do processo |

As quatro parcelas somam exatamente `ProcessMemoryBudget`. O cálculo utiliza aritmética inteira e valida overflow e consistência da soma.

### 3.1. O que os orçamentos representam

Os orçamentos são limites de contabilização lógica específicos de cada componente. Eles não correspondem a uma medição exata do heap Go nem ao RSS do processo.

- **Storage:** contabiliza `len(key) + len(value)` por entrada. A substituição de valor considera o consumo anterior; `DELETE` libera o consumo lógico correspondente. Estruturas de `map`, objetos, locks, cópias e overhead do runtime não entram nessa conta.
- **Idempotência:** contabiliza uma reserva-base estimada para OperationID, fingerprint e metadados, além dos conteúdos variáveis da resposta armazenada. Não é uma medição exata do heap.
- **Processamento:** o Handler compartilhado entre as conexões reserva uma vaga global por instância e uma quantidade lógica de memória antes de alocar o payload do frame. A reserva considera o tamanho do frame e dois buffers dimensionados pelo payload máximo configurado; é mantida até a resposta ser escrita ou ocorrer erro.
- **Runtime:** recebe o restante do orçamento depois das três parcelas anteriores. Nesta versão, é um valor de planejamento e validação; não é aplicado como limite rígido do runtime.

O `ProcessMemoryBudget` não deve ser interpretado como limite máximo de RSS. A implementação não configura `GOMEMLIMIT`, não chama `debug.SetMemoryLimit` e não instala um limite de memória do sistema operacional. O orçamento também não garante que o processo permanecerá abaixo desse valor em todas as condições.

### 3.2. Capacidade de CPU

A capacidade de CPU é representada em millicores, seguindo a convenção usada por plataformas de orquestração, por exemplo `500m`, `1000m` e `2000m`.

O valor detectado ou configurado é utilizado no cálculo e na validação da configuração. Nesta versão, ele não altera `GOMAXPROCS`, não cria afinidade de CPU e não configura um limite de CPU no sistema operacional.

## 4. Detecção por sistema operacional

A API pública é `resources.DetectResources()`. A implementação é selecionada por build tags.

### 4.1. Linux

A implementação Linux utiliza:

- `runtime.NumCPU()` como capacidade de CPU do host, convertida para millicores;
- `MemAvailable` de `/proc/meminfo` como memória disponível no host;
- `/proc/self/mountinfo` e `/proc/self/cgroup` para identificar montagens e membership de cgroups;
- cgroup v1 e cgroup v2 para consultar limites de CPU e memória.

Para CPU, são considerados os limites de quota e período (`cpu.cfs_quota_us` / `cpu.cfs_period_us` no v1 e `cpu.max` no v2).

Para memória, são considerados limite e uso (`memory.limit_in_bytes` / `memory.usage_in_bytes` no v1 e `memory.max` / `memory.current` no v2). Um limite finito é convertido em memória disponível subtraindo o uso atual.

A detecção percorre o cgroup atual e seus ancestrais até a raiz da montagem, considerando o menor limite aplicável. O resultado também é limitado pela capacidade disponível do host.

Quando os metadados de cgroups não estão disponíveis, a detecção utiliza os recursos do host. Erros de leitura ou conteúdo inválido em metadados que foram encontrados não são silenciosamente tratados como ausência de limite: a detecção retorna erro.

Valores ilimitados são reconhecidos conforme a representação de cada versão, como `max` no cgroup v2 e os valores sem limite do cgroup v1.

Depois da detecção, são descontadas margens de segurança de 10% para CPU e 20% para memória.

A origem de cada capacidade é informada separadamente como `host` ou `container`. A origem `container` é selecionada quando um limite aplicável reduz a capacidade em relação ao host.

### 4.2. Windows

A implementação Windows utiliza:

- `runtime.NumCPU()` para a quantidade de CPUs lógicas;
- `GlobalMemoryStatusEx` para obter a memória física disponível.

São descontadas as mesmas margens de segurança: 10% para CPU e 20% para memória.

A origem informada é `host`. A implementação atual não consulta limites de Windows Job Objects; portanto, não detecta nem aplica limites de CPU ou memória definidos por esse mecanismo.

### 4.3. Outros sistemas operacionais

Em sistemas que não possuem implementação de detecção, `DetectResources` retorna `ErrUnsupportedPlatform`. O carregamento da configuração de recursos não prossegue.

## 5. Variáveis de ambiente

As variáveis são interpretadas por `LoadResourceConfigFromEnv()`.

Variáveis ausentes ou vazias preservam o valor selecionado pelo perfil. Valores presentes, mas inválidos, provocam erro. Os tamanhos de memória aceitam unidades decimais e binárias; números sem sufixo são interpretados como bytes.

| Variável | Significado | Observação |
|---|---|---|
| `MEMORYDB_RESOURCE_PROFILE` | Perfil de recursos | Padrão `mid` |
| `MEMORYDB_RESOURCE_CPU` | Capacidade de CPU | Obrigatória em `custom`; aceita cores ou millicores |
| `MEMORYDB_RESOURCE_MEMORY` | Orçamento de memória do processo | Obrigatória em `custom` |
| `MEMORYDB_STORAGE_MAX_ENTRIES` | Máximo de entradas do Storage | Override |
| `MEMORYDB_STORAGE_MEMORY_BUDGET` | Orçamento lógico do Storage | Override |
| `MEMORYDB_STORAGE_MAX_KEY_SIZE` | Tamanho máximo da chave | Override |
| `MEMORYDB_STORAGE_MAX_VALUE_SIZE` | Tamanho máximo do valor | Override |
| `MEMORYDB_IDEMPOTENCY_MAX_ENTRIES` | Máximo de registros de idempotência | Nome canônico |
| `MEMORYDB_IDEMPOTENCY_CAPACITY` | Alias legado para o máximo de registros | Se ambos forem informados, os valores devem ser iguais |
| `MEMORYDB_IDEMPOTENCY_MEMORY_BUDGET` | Orçamento lógico da idempotência | Override |
| `MEMORYDB_IDEMPOTENCY_RETENTION` | Retenção dos registros concluídos | Padrão 24h |
| `MEMORYDB_MAX_CONNECTIONS` | Máximo de conexões TCP | Compartilhada com `shared/config` |
| `MEMORYDB_MAX_IN_FLIGHT_REQUESTS` | Máximo de requisições em processamento | Override |
| `MEMORYDB_MAX_FRAME_PAYLOAD_SIZE` | Payload máximo do frame | Override; não pode superar o limite máximo do transporte |
| `MEMORYDB_PROCESSING_MEMORY_BUDGET` | Orçamento lógico de processamento | Override |
| `MEMORYDB_RUNTIME_MEMORY_BUDGET` | Parcela destinada ao runtime | Override; a soma dos quatro orçamentos precisa continuar igual ao orçamento do processo |

Unidades aceitas para memória: `B`, `KB`, `MB`, `GB`, `TB`, `KiB`, `MiB`, `GiB` e `TiB`. Unidades decimais usam potências de 1000; unidades binárias usam potências de 1024.

Exemplos:

```bash
MEMORYDB_RESOURCE_PROFILE=mid
MEMORYDB_STORAGE_MEMORY_BUDGET=600MiB
MEMORYDB_STORAGE_MAX_KEY_SIZE=1KiB
MEMORYDB_STORAGE_MAX_VALUE_SIZE=128KiB
MEMORYDB_IDEMPOTENCY_MAX_ENTRIES=150000
MEMORYDB_IDEMPOTENCY_RETENTION=48h
MEMORYDB_MAX_CONNECTIONS=1500
MEMORYDB_MAX_IN_FLIGHT_REQUESTS=300
MEMORYDB_MAX_FRAME_PAYLOAD_SIZE=512KiB
```

`MEMORYDB_MAX_CONNECTIONS` também é reconhecida como configuração de recursos. O limite escolhido pelo perfil pode ser substituído pela variável.

Se `MEMORYDB_IDEMPOTENCY_MAX_ENTRIES` e `MEMORYDB_IDEMPOTENCY_CAPACITY` forem informadas simultaneamente com valores diferentes, o carregamento falha com erro de conflito.

Os overrides são aplicados ao perfil antes da validação final. Portanto, combinações inconsistentes — por exemplo, soma dos orçamentos diferente de `ProcessMemoryBudget`, valor máximo maior que o frame máximo ou limites não positivos — impedem a inicialização.

## 6. Configurações operacionais fora do pacote de recursos

As variáveis abaixo continuam pertencendo à configuração operacional do servidor, em `server/internal/config`, e não ao dimensionamento de recursos:

| Variável | Padrão | Finalidade |
|---|---:|---|
| `MEMORYDB_ADDR` | `:6380` | Endereço TCP de escuta |
| `MEMORYDB_SHUTDOWN_TIMEOUT` | `10s` | Prazo do shutdown |
| `MEMORYDB_READ_TIMEOUT` | `30s` | Deadline de cada operação de leitura TCP |
| `MEMORYDB_WRITE_TIMEOUT` | `10s` | Deadline de cada operação de escrita TCP |

As variáveis do cliente de demonstração também são independentes da configuração de recursos do servidor:

| Variável | Padrão | Finalidade |
|---|---:|---|
| `MEMORYDB_CLIENT_ADDR` | `127.0.0.1:6380` | Endereço utilizado pelo cliente de demonstração |
| `MEMORYDB_DIAL_TIMEOUT` | `5s` | Timeout para estabelecer a conexão inicial do cliente |

## 7. API Go pública

O pacote permite carregar a configuração tanto por variáveis de ambiente quanto por opções programáticas.

```go
package main

import (
    "fmt"

    "github.com/pescador95/memorydb/server/resources"
)

func main() {
    config, err := resources.LoadResourceConfig(resources.ResourceOptions{
        Profile: resources.ResourceProfileAuto,
    })
    if err != nil {
        panic(err)
    }

    fmt.Printf("CPU: %dm\n", config.CPUCapacity)
    fmt.Printf("Process memory budget: %d bytes\n", config.ProcessMemoryBudget)
}
```

As principais operações públicas são:

- `DetectResources()` — detecta a capacidade disponível;
- `LoadResourceConfig(options)` — carrega e valida uma configuração programática;
- `LoadResourceConfigFromEnv()` — carrega e valida uma configuração a partir do ambiente;
- `ResourceConfig.Validate()` — valida uma configuração já construída.

`ResourceOptions` aceita um perfil, valores de CPU e memória para `custom` e `ResourceOverrides` opcionais. Os overrides programáticos cobrem CPU e orçamento do processo; máximo de entradas, orçamento, tamanho de chave e tamanho de valor do Storage; máximo de entradas, orçamento e retenção da Idempotência; máximo de conexões, requisições em processamento e payload de frame; e orçamentos de Processamento e Runtime.

Os mesmos critérios de detecção, cálculo e validação são aplicados aos dois caminhos de configuração.

## 8. Integração no servidor

Durante a inicialização, `server/internal/config.Load()` chama `resources.LoadResourceConfigFromEnv()`. O resultado validado é armazenado em `Config.Resources`.

O executável `server/cmd/memorydb` distribui somente os limites necessários a cada componente:

- Storage recebe máximo de entradas, orçamento lógico, tamanho máximo de chave e valor;
- Executor de idempotência recebe capacidade, orçamento lógico e retenção;
- Handler recebe máximo de requisições em processamento, tamanho máximo de payload e orçamento lógico de processamento;
- Server TCP recebe máximo de conexões.

O pacote de recursos não instancia esses componentes e não os faz depender do detector de hardware ou de cgroups. A composição permanece no executável.

Antes de iniciar o listener, o executável também valida se os maiores comandos e respostas permitidos pelos limites de chave e valor cabem no payload máximo do frame. Essa validação utiliza `proto.Size` sobre mensagens Protobuf representativas, incluindo o overhead do envelope e dos metadados; não compara somente os bytes crus da chave ou do valor.

## 9. Limitações e garantias

- A configuração é calculada no startup; não existe redimensionamento dinâmico.
- Orçamentos lógicos não são medições exatas do heap ou RSS.
- `Runtime.MemoryBudget` não é aplicado como limite do runtime.
- `ProcessMemoryBudget` não é um hard limit de memória do processo.
- `CPUCapacity` não configura limite de CPU nem altera `GOMAXPROCS`.
- No Windows, limites de Job Objects ainda não são detectados.
- Em sistemas diferentes de Linux e Windows, a detecção não está implementada.
- A detecção de cgroups no Linux utiliza quotas de CPU e limites de memória; não interpreta todas as possíveis políticas e controladores de cgroups.
- O perfil `low`, `mid` ou `high` pode ser rejeitado quando não couber nos recursos detectados.
- Overrides precisam respeitar a validação de consistência da configuração final.

A implementação deve ser interpretada como dimensionamento e aplicação de limites lógicos por componente, não como isolamento de recursos garantido pelo sistema operacional.
