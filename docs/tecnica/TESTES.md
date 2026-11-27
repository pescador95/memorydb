# Guia de Testes do MemoryDB

## 1. Objetivo

Este documento descreve os procedimentos de validação do MemoryDB, centralizados em `scripts/test/` e orquestrados pelo script `scripts/test.sh`.

O projeto utiliza um workspace Go com múltiplos módulos independentes. Por isso, as verificações de análise estática, testes, compilação e cobertura são executadas individualmente em cada módulo.

O objetivo é manter um processo padronizado de validação, facilitar a execução local e garantir que as mesmas verificações possam ser utilizadas em ambientes de integração contínua.

## 2. Pré-requisitos

Para executar os procedimentos de teste, é necessário ter:

- Go instalado e disponível no `PATH`;
- Bash;
- utilitários de linha de comando utilizados pelos scripts: `find`, `xargs`, `dirname`, `awk`, `sort`, `tail`, `mktemp`, `rm`, `mkdir` e `cat`;
- `gofmt` e `go tool cover`, disponibilizados pela instalação do Go.

Em ambientes Windows, recomenda-se utilizar um ambiente Bash compatível, como Git Bash.

A partir da raiz do repositório, execute:

```bash
./scripts/test.sh
```

O script identifica automaticamente a raiz do repositório com base na localização dos próprios arquivos, não dependendo do diretório de trabalho a partir do qual foi iniciado.

As etapas que percorrem os módulos verificam a existência do respectivo `go.mod` antes de executar os comandos Go.

A etapa `format` percorre os arquivos Go do repositório, enquanto as etapas que executam comandos Go nos módulos utilizam a lista centralizada em `scripts/test/common.sh`.

## 3. Organização dos módulos

As verificações são executadas nos seguintes sete módulos:

| Módulo | Diretório |
|---|---|
| API | `api/` |
| Driver Go | `drivers/go/` |
| Protocolo Protobuf gerado | `protocol/generated/go/` |
| Servidor | `server/` |
| Componentes compartilhados | `shared/` |
| Transporte TCP Go | `transport/tcp/go/` |
| Storage | `storage/go/` |

O módulo Storage possui seu próprio `go.mod` e é validado independentemente do módulo Server. Sua extração para um módulo Go independente não altera o fato de que, na execução do servidor, Storage é utilizado como biblioteca no mesmo processo.

O schema Protocol Buffers utilizado pelo projeto está localizado em:

```text
protocol/proto/memorydb/v1/memorydb.proto
```

Esse diretório contém a definição do protocolo, mas não constitui um módulo Go independente. A implementação gerada para Go é validada pelo módulo `protocol/generated/go/`.

A lista dos módulos é mantida em `scripts/test/common.sh`, por meio da variável `MODULES`.

## 4. Execução dos testes

O script principal aceita uma etapa como argumento:

```bash
./scripts/test.sh [etapa]
```

Quando nenhuma etapa é informada, o comportamento padrão é executar `all`.

### 4.1. Etapas disponíveis

| Etapa | Descrição |
|---|---|
| `format` | Verifica a formatação dos arquivos Go. |
| `vet` | Executa a análise estática com `go vet`. |
| `unit` | Executa os testes unitários. |
| `race` | Executa os testes com o detector de condições de corrida. |
| `build` | Valida a compilação dos módulos. |
| `coverage` | Executa os testes e gera os relatórios de cobertura. |
| `uncovered` | Identifica arquivos e blocos de código sem cobertura completa. |
| `all` | Executa todas as etapas na sequência definida pelo projeto. |
| `help` | Exibe a ajuda de utilização. |

Também são aceitos `-h` e `--help`.

Exemplos:

```bash
# Executa todas as etapas
./scripts/test.sh

# Executa todas as etapas explicitamente
./scripts/test.sh all

# Executa somente a verificação de formatação
./scripts/test.sh format

# Executa somente os testes unitários
./scripts/test.sh unit

# Executa os testes com detector de condições de corrida
./scripts/test.sh race

# Gera os relatórios de cobertura
./scripts/test.sh coverage

# Analisa os blocos sem cobertura
./scripts/test.sh uncovered

# Exibe a ajuda
./scripts/test.sh help
```

O script aceita no máximo um argumento. Uma etapa desconhecida ou a passagem de argumentos adicionais resulta na exibição da ajuda e no encerramento com erro.

## 5. Etapas de validação

### 5.1. Formatação

Comando:

```bash
./scripts/test.sh format
```

A etapa verifica todos os arquivos com extensão `.go` existentes no repositório, excluindo o diretório `.git`.

A verificação utiliza:

```bash
gofmt -l
```

Os arquivos são identificados por `find` e encaminhados ao `gofmt` por meio de `xargs`.

Caso sejam encontrados arquivos fora do padrão de formatação, seus caminhos são apresentados na saída e a etapa falha.

Essa verificação não modifica os arquivos. Para aplicar a formatação, utilize `gofmt -w` nos arquivos que precisam de correção.

### 5.2. Análise estática

Comando:

```bash
./scripts/test.sh vet
```

Executa:

```bash
go vet ./...
```

O comando é executado individualmente em cada módulo, verificando possíveis problemas estáticos no código Go.

### 5.3. Testes unitários

Comando:

```bash
./scripts/test.sh unit
```

Executa:

```bash
go test -count=1 ./...
```

Os testes são executados individualmente em cada módulo.

A opção `-count=1` desabilita o reaproveitamento do resultado em cache dos testes, garantindo uma nova execução.

### 5.4. Testes com detector de condições de corrida

Comando:

```bash
./scripts/test.sh race
```

Executa:

```bash
go test -race -count=1 ./...
```

A etapa utiliza o detector de condições de corrida do Go para identificar acessos concorrentes à memória que não estejam devidamente sincronizados.

Assim como na etapa unitária, `-count=1` força uma nova execução dos testes.

### 5.5. Compilação

Comando:

```bash
./scripts/test.sh build
```

Executa:

```bash
go build ./...
```

A compilação é realizada individualmente em cada módulo, validando a construção dos pacotes e suas dependências.

### 5.6. Cobertura de testes

Comando:

```bash
./scripts/test.sh coverage
```

Essa etapa executa os testes de cada módulo utilizando:

```bash
go test -coverprofile=<arquivo> ./...
```

Os perfis são armazenados no diretório `coverage/`, na raiz do repositório.

Cada módulo possui um arquivo de cobertura próprio:

| Módulo | Perfil |
|---|---|
| `api` | `coverage/api.out` |
| `drivers/go` | `coverage/drivers_go.out` |
| `protocol/generated/go` | `coverage/protocol_generated_go.out` |
| `server` | `coverage/server.out` |
| `shared` | `coverage/shared.out` |
| `transport/tcp/go` | `coverage/transport_tcp_go.out` |
| `storage/go` | `coverage/storage_go.out` |

O nome dos arquivos substitui as barras dos caminhos dos módulos por sublinhados.

Após a execução dos testes, os perfis são utilizados para calcular a cobertura por módulo e gerar relatórios HTML.

São gerados os seguintes relatórios:

| Módulo | Relatório HTML |
|---|---|
| `api` | `coverage/api.html` |
| `drivers/go` | `coverage/drivers_go.html` |
| `protocol/generated/go` | `coverage/protocol_generated_go.html` |
| `server` | `coverage/server.html` |
| `shared` | `coverage/shared.html` |
| `transport/tcp/go` | `coverage/transport_tcp_go.html` |
| `storage/go` | `coverage/storage_go.html` |

Também é gerado:

```text
coverage/index.html
```

Esse arquivo funciona como índice para acessar os relatórios individuais.

Os relatórios HTML são produzidos com `go tool cover`.

O resumo apresentado no terminal também classifica os arquivos conforme a cobertura de suas instruções:

- **FULL:** todas as instruções contabilizadas possuem cobertura;
- **PARTIAL:** existe cobertura, mas nem todas as instruções foram executadas;
- **ZERO:** nenhuma instrução contabilizada possui cobertura.

A classificação é calculada a partir dos contadores dos perfis de cobertura. Um bloco é considerado coberto quando seu contador de execução é maior que zero.

A etapa também apresenta a cobertura total de cada módulo e o resumo por arquivo.

### 5.7. Análise de código não coberto

Comando:

```bash
./scripts/test.sh uncovered
```

Essa etapa analisa os perfis `.out` produzidos pela etapa `coverage`.

Para cada módulo, identifica:

- arquivos sem cobertura;
- arquivos com cobertura parcial;
- percentual de cobertura;
- quantidade de instruções cobertas em relação ao total;
- blocos e intervalos de linhas não cobertos.

A análise utiliza `awk` para interpretar os perfis e `sort` para organizar os resultados.

Os resultados são apresentados no terminal, sem gerar um relatório adicional persistente.

A análise considera como não cobertos os blocos cujo contador de execução é zero. Para arquivos parcialmente cobertos, são apresentados os intervalos de linhas correspondentes aos blocos não executados.

**Dependência:** a etapa exige que os sete perfis de cobertura estejam presentes em `coverage/`. Caso o diretório ou algum perfil não exista, a execução é interrompida e o script orienta a executar:

```bash
./scripts/test.sh coverage
```

A etapa `uncovered` não executa novamente os testes nem atualiza os perfis. Ela utiliza os arquivos gerados pela execução anterior de `coverage`.

### 5.8. Testes de dimensionamento e detecção de recursos

O pacote `server/resources` pertence ao módulo `server`; ele não é um oitavo módulo Go. Seus testes são executados junto com os demais pacotes do módulo:

```bash
cd server
go test ./resources/...
go test -race ./resources/...
```

A suíte contém testes unitários e de integração com arquivos temporários para a descoberta de montagens e memberships de cgroups, resolução de hierarquias, limites herdados, valores ilimitados, caminhos inválidos, erros de leitura, parsing de variáveis de ambiente, cálculo de perfis e validação de overrides.

Os arquivos específicos de plataforma utilizam build tags:
- `*_linux.go` e testes `//go:build linux` são compilados e executados em Linux;
- `detector_windows.go` e testes `//go:build windows` são compilados e executados em Windows;
- `detector_unsupported.go` atende plataformas diferentes de Linux e Windows.

Uma execução no WSL Ubuntu valida o caminho Linux. Ela não executa os testes Windows; para validar esse caminho é necessária uma execução em Windows ou uma compilação/teste cruzado apropriado.

## 6. Pipeline completo

Comando:

```bash
./scripts/test.sh all
```

O pipeline executa as etapas na seguinte ordem:

| Ordem | Etapa | Objetivo |
|---:|---|---|
| 1 | `format` | Verificar a formatação do código Go. |
| 2 | `vet` | Executar a análise estática. |
| 3 | `unit` | Executar os testes unitários. |
| 4 | `race` | Verificar condições de corrida. |
| 5 | `build` | Validar a compilação. |
| 6 | `coverage` | Gerar os perfis e relatórios de cobertura. |
| 7 | `uncovered` | Identificar código sem cobertura completa. |

Cada etapa é executada por meio de seu respectivo script em `scripts/test/`.

O script principal e os scripts individuais utilizam `set -euo pipefail`, interrompendo a execução diante de falhas.

Consequentemente, se uma etapa falhar, as etapas seguintes não serão executadas.

Ao concluir todas as etapas, o script apresenta a mensagem:

```text
PIPELINE CONCLUÍDO COM SUCESSO
```

## 7. Validação integral registrada em 27/11/2026

Em 27/11/2026, o usuário executou `./scripts/test.sh` no WSL Ubuntu. Todas as etapas do pipeline foram concluídas com sucesso:

- `format`;
- `vet`;
- `unit`;
- `race`;
- `build`;
- `coverage`;
- `uncovered`.

A execução percorreu os sete módulos do workspace. O pacote `server/resources` foi incluído nas etapas do módulo `server`.

### Cobertura observada

| Módulo | Cobertura |
|---|---:|
| `api` | 97,6% |
| `drivers/go` | 95,3% |
| `protocol/generated/go` | 0,0% |
| `server` | 95,1% |
| `shared` | 100,0% |
| `transport/tcp/go` | 98,3% |
| `storage/go` | 98,2% |

O código Protobuf gerado apresentou 0,0% por não possuir testes próprios. No módulo `server`, `server/resources` apresentou 96,3% de cobertura. Nenhum arquivo do pacote `server/resources` ficou na categoria sem cobertura; alguns arquivos permaneceram parcialmente cobertos.

A etapa `uncovered` listou blocos não executados em diferentes componentes. Cobertura parcial não é, por si só, uma falha: os caminhos restantes devem ser avaliados pelo comportamento que representam, e não apenas pelo percentual.

**Escopo da evidência:** essa execução comprova o resultado do pipeline Linux informado nessa data. Ela não substitui uma execução em Windows para os caminhos protegidos por build tags Windows, nem garante que alterações posteriores continuem aprovadas.

## 8. Artefatos gerados

A execução da etapa `coverage` gera os seguintes artefatos:

```text
coverage/
├── api.out
├── api.html
├── drivers_go.out
├── drivers_go.html
├── protocol_generated_go.out
├── protocol_generated_go.html
├── server.out
├── server.html
├── shared.out
├── shared.html
├── transport_tcp_go.out
├── transport_tcp_go.html
├── storage_go.out
├── storage_go.html
└── index.html
```

Os arquivos `.out` são perfis de cobertura produzidos pelo `go test`.

Os arquivos `.html` são relatórios individuais gerados pelo `go tool cover`.

O arquivo `index.html` reúne os links para os relatórios individuais.

O diretório `coverage/` é ignorado pelo Git por meio da regra:

```gitignore
/coverage/
```