# Guia de Testes do MemoryDB

## 1. Objetivo

Este documento descreve como executar os testes automatizados e as verificações técnicas do MemoryDB, incluindo formatação, análise estática, compilação, detecção de condições de corrida e cobertura de código.

Os procedimentos são centralizados nos scripts de `scripts/test/` e orquestrados por `scripts/test.sh`.

O repositório utiliza múltiplos módulos Go coordenados por um workspace. Portanto, as verificações são executadas individualmente em cada módulo, em vez de depender de um único `go test ./...` na raiz.

## 2. Pré-requisitos

Para executar o pipeline, é necessário:

- Go instalado e disponível no `PATH`.
- Bash.
- Utilitários de linha de comando utilizados pelos scripts: `find`, `xargs`, `awk`, `sort`, `tail`, `mktemp` e `rm`.

A execução deve ser iniciada na raiz do repositório:

```bash
./scripts/test.sh
```

Os scripts calculam a raiz do projeto a partir de sua própria localização. Assim, não dependem de o comando ser iniciado em um diretório específico, desde que o caminho do script seja informado corretamente.

O script verifica se o executável `go` está disponível e se cada módulo possui seu respectivo `go.mod`.

## 3. Módulos verificados

O pipeline percorre os seguintes módulos:

| Módulo | Diretório |
|---|---|
| API | `api` |
| Driver Go | `drivers/go` |
| Protobuf gerado para Go | `protocol/generated/go` |
| Servidor | `server` |
| Transporte TCP para Go | `transport/tcp/go` |

Cada módulo é verificado separadamente. Isso permite que os comandos Go sejam executados no contexto do respectivo `go.mod`.

## 4. Comandos disponíveis

O ponto de entrada é `scripts/test.sh`. Ele aceita uma etapa como argumento. Quando nenhum argumento é informado, executa `all`.

| Comando | Finalidade |
|---|---|
| `./scripts/test.sh` | Executa o pipeline completo |
| `./scripts/test.sh all` | Executa o pipeline completo |
| `./scripts/test.sh format` | Verifica a formatação dos arquivos Go |
| `./scripts/test.sh vet` | Executa `go vet ./...` em cada módulo |
| `./scripts/test.sh unit` | Executa os testes unitários em cada módulo |
| `./scripts/test.sh race` | Executa os testes com o detector de condições de corrida |
| `./scripts/test.sh build` | Valida a compilação de cada módulo |
| `./scripts/test.sh coverage` | Executa os testes e gera os relatórios de cobertura |
| `./scripts/test.sh uncovered` | Analisa os blocos não cobertos nos perfis existentes |
| `./scripts/test.sh help` | Exibe a ajuda |

Também são aceitos `-h` e `--help`.

Se uma etapa falhar, o script encerra o pipeline com erro. As etapas seguintes não são executadas.

## 5. Etapas de validação

### 5.1. Formatação

Comando:

```bash
./scripts/test.sh format
```

O script procura arquivos com extensão `.go` no repositório, desconsiderando o diretório `.git`, e utiliza:

```bash
gofmt -l
```

A etapa falha quando existem arquivos que precisam de formatação. Nesse caso, os caminhos dos arquivos encontrados são apresentados na saída.

Essa etapa verifica a formatação, mas não modifica os arquivos automaticamente.

### 5.2. Análise estática

Comando:

```bash
./scripts/test.sh vet
```

Executa, em cada módulo:

```bash
go vet ./...
```

O objetivo é identificar construções suspeitas ou possíveis problemas estáticos detectáveis pelo `go vet`.

### 5.3. Testes unitários

Comando:

```bash
./scripts/test.sh unit
```

Executa, em cada módulo:

```bash
go test -count=1 ./...
```

A opção `-count=1` evita reutilizar resultados de testes armazenados em cache, solicitando uma nova execução.

Os testes são executados para os pacotes encontrados pelo padrão `./...` dentro de cada módulo.

### 5.4. Detecção de condições de corrida

Comando:

```bash
./scripts/test.sh race
```

Executa, em cada módulo:

```bash
go test -race -count=1 ./...
```

O detector de condições de corrida procura acessos concorrentes à memória que não estejam devidamente sincronizados.

A aprovação dessa etapa representa o resultado dos testes executados com o detector habilitado. Não constitui prova de ausência de condições de corrida em todos os caminhos possíveis de execução.

### 5.5. Compilação

Comando:

```bash
./scripts/test.sh build
```

Executa, em cada módulo:

```bash
go build ./...
```

Essa etapa verifica se os pacotes compilam. Ela não substitui a execução dos testes automatizados.

## 6. Cobertura de código

### 6.1. Geração dos relatórios

Comando:

```bash
./scripts/test.sh coverage
```

A etapa executa `go test -coverprofile` em cada módulo e gera um perfil de cobertura individual.

Os arquivos são armazenados no diretório `coverage/`, na raiz do repositório:

| Módulo | Perfil gerado |
|---|---|
| `api` | `coverage/api.out` |
| `drivers/go` | `coverage/drivers_go.out` |
| `protocol/generated/go` | `coverage/protocol_generated_go.out` |
| `server` | `coverage/server.out` |
| `transport/tcp/go` | `coverage/transport_tcp_go.out` |

Para cada módulo, o script apresenta:

- Cobertura total calculada pelo Go.
- Arquivos com 100% de cobertura.
- Arquivos parcialmente cobertos.
- Arquivos sem cobertura.

A classificação por arquivo considera a quantidade de instruções cobertas em relação ao total de instruções registradas no perfil.

O script também gera o relatório HTML do módulo `server`:

```text
coverage/server.html
```

Esse relatório pode ser aberto em um navegador para inspecionar visualmente a cobertura do código do servidor.

### 6.2. Análise dos blocos não cobertos

Comando:

```bash
./scripts/test.sh uncovered
```

Essa etapa consome os perfis `.out` existentes em `coverage/`. Ela não executa os testes nem atualiza os relatórios.

Para cada módulo, apresenta:

- Arquivos sem cobertura, com 0%.
- Arquivos parcialmente cobertos, com percentual e quantidade de instruções cobertas.
- Linhas correspondentes aos blocos não cobertos, acompanhadas da quantidade de instruções desses blocos.

Arquivos com 100% de cobertura não são listados.

Se o diretório ou algum perfil esperado não existir, o script informa o problema e orienta a executar:

```bash
./scripts/test.sh coverage
```

**Importante:** como `uncovered` utiliza os perfis existentes, seus resultados podem estar desatualizados caso o código ou os testes tenham sido modificados depois da última execução de `coverage`. Para analisar o estado atual do projeto, execute primeiro a geração dos relatórios.

### 6.3. Interpretação dos resultados

A cobertura é uma métrica auxiliar para identificar partes do código que não foram exercitadas pelos testes executados. Ela não mede, isoladamente, a qualidade dos testes nem comprova a correção do comportamento implementado.

A interpretação deve considerar a responsabilidade de cada arquivo e o tipo de código:

- Código de regras de negócio, armazenamento, protocolo e tratamento de requisições deve ter cenários de teste compatíveis com seus comportamentos e caminhos de erro.
- Arquivos de inicialização e pontos de entrada, como funções `main`, podem exigir testes de integração ou estratégias específicas para exercitar seu comportamento.
- Código gerado, como arquivos Protobuf, não deve ser tratado automaticamente como uma lacuna de testes da mesma forma que código escrito manualmente. A validação deve considerar os testes do código que utiliza essas estruturas e a necessidade de verificar a integração com o formato gerado.
- Blocos não cobertos devem ser avaliados individualmente. Nem todo bloco exige necessariamente um teste isolado, mas lacunas relevantes precisam ser justificadas.

O objetivo não é alcançar 100% de cobertura artificialmente. O objetivo é obter evidências adequadas de que os comportamentos importantes, inclusive situações de erro e limites, estão sendo verificados.

## 7. Execução completa

Comando:

```bash
./scripts/test.sh
```

Ou, explicitamente:

```bash
./scripts/test.sh all
```

A execução completa segue esta ordem:

1. Formatação dos arquivos Go.
2. Análise estática com `go vet`.
3. Testes unitários.
4. Testes com detector de condições de corrida.
5. Compilação dos módulos.
6. Execução dos testes e geração dos relatórios de cobertura.
7. Análise dos blocos não cobertos.

A etapa `uncovered` depende dos perfis produzidos pela etapa `coverage`. Por isso, ambas fazem parte do pipeline completo nessa ordem.

Se todas as etapas forem concluídas, o script apresenta a mensagem de conclusão do pipeline.

## 8. Artefatos gerados e versionamento

Os relatórios são gerados localmente no diretório:

```text
coverage/
```

Os perfis `.out` e o relatório `server.html` são artefatos de execução, não documentação nem código-fonte. Eles não devem ser versionados no Git.

O `.gitignore` do projeto já contém regras que ignoram esses artefatos, incluindo o diretório `/coverage/`.

Os scripts utilizados para executar as verificações, por outro lado, fazem parte da infraestrutura do projeto e devem ser versionados:

```text
scripts/test.sh
scripts/test/
```

## 9. Manutenção dos testes

Ao adicionar ou modificar funcionalidades:

1. Criar ou atualizar testes para os comportamentos afetados.
2. Cobrir os cenários relevantes de sucesso, erro e limites.
3. Executar as etapas de validação adequadas.
4. Executar `./scripts/test.sh` antes de considerar a validação completa.
5. Avaliar os resultados de cobertura e investigar lacunas relevantes.
6. Atualizar esta documentação quando os comandos, módulos ou procedimentos de validação forem alterados.

Os resultados de uma execução representam o estado do código e dos testes naquele momento. Registros históricos de validação são evidências de execuções anteriores e não substituem a execução do pipeline após novas alterações.