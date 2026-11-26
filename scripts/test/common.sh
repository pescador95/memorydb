
#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

MODULES=(
    "api"
    "drivers/go"
    "protocol/generated/go"
    "server"
    "shared"
    "transport/tcp/go"
    "storage/go"
)

require_go() {
    if ! command -v go >/dev/null 2>&1; then
        printf 'ERRO: Go não encontrado no PATH.\n' >&2
        return 1
    fi
}

run_in_modules() {
    local description="$1"
    shift

    local module
    local module_dir

    for module in "${MODULES[@]}"; do
        module_dir="$ROOT_DIR/$module"

        if [[ ! -f "$module_dir/go.mod" ]]; then
            printf 'ERRO: go.mod não encontrado em %s\n' "$module_dir" >&2
            return 1
        fi

        printf '\n===== %s: %s =====\n' "$description" "$module"

        (
            cd "$module_dir"
            go "$@"
        )
    done
}

check_format() {
    printf '\n===== VERIFICAÇÃO DE FORMATAÇÃO =====\n'

    local unformatted

    unformatted="$(
        find "$ROOT_DIR" \
            -path "$ROOT_DIR/.git" -prune -o \
            -type f -name '*.go' -print0 |
            xargs -0 -r gofmt -l
    )"

    if [[ -n "$unformatted" ]]; then
        printf 'ERRO: arquivos Go sem formatação adequada:\n'
        printf '%s\n' "$unformatted"
        return 1
    fi

    printf 'Todos os arquivos Go estão formatados.\n'
}
