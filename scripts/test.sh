
#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$SCRIPT_DIR/test"

usage() {
    cat <<'EOF'
Uso:
  ./scripts/test.sh [etapa]

Etapas disponíveis:
  format    Verifica a formatação dos arquivos Go.
  vet       Executa análise estática com go vet.
  unit      Executa os testes unitários.
  race      Executa os testes com detector de condições de corrida.
  build     Valida a compilação dos módulos.
  all       Executa todas as etapas (padrão).
  coverage  Executa os testes e apresenta a cobertura de código.
  uncovered Identifica blocos de código não cobertos pelos testes.
  help      Exibe esta ajuda.

Exemplos:
  ./scripts/test.sh
  ./scripts/test.sh all
  ./scripts/test.sh unit
  ./scripts/test.sh race
  ./scripts/test.sh coverage
  ./scripts/test.sh uncovered
EOF
}

run_stage() {
    local stage="$1"

    printf '\n========================================\n'
    printf ' INICIANDO ETAPA: %s\n' "$stage"
    printf '========================================\n\n'

    bash "$TEST_DIR/$stage.sh"

    printf '\nETAPA CONCLUÍDA: %s\n' "$stage"
}

run_all() {
    local stages=(
        "format"
        "vet"
        "unit"
        "race"
        "build"
        "coverage"
        "uncovered"
    )

    local stage

    for stage in "${stages[@]}"; do
        run_stage "$stage"
    done

    printf '\n========================================\n'
    printf ' PIPELINE CONCLUÍDO COM SUCESSO\n'
    printf '========================================\n'
}

main() {
    local stage="${1:-all}"

    if [[ $# -gt 1 ]]; then
        usage
        return 1
    fi

    case "$stage" in
        format|coverage|uncovered|vet|unit|race|build)
            run_stage "$stage"
            ;;
        all)
            run_all
            ;;
        help|-h|--help)
            usage
            ;;
        *)
            printf 'ERRO: etapa desconhecida: %s\n\n' "$stage" >&2
            usage
            return 1
            ;;
    esac
}

main "$@"
