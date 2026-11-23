
#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$SCRIPT_DIR/common.sh"

require_go

COVERAGE_DIR="$ROOT_DIR/coverage"

mkdir -p "$COVERAGE_DIR"

SUMMARY_FILE="$(mktemp "${TMPDIR:-/tmp}/memorydb-coverage-summary.XXXXXX")"

cleanup() {
    rm -f "$SUMMARY_FILE"
}

trap cleanup EXIT

print_file_summary() {
    local profile="$1"
    local module="$2"
    local status

    : > "$SUMMARY_FILE"

    LC_ALL=C awk '
        /^mode: / {
            next
        }

        {
            file = $1
            sub(/:.*/, "", file)

            statements = $2 + 0
            count = $3 + 0

            total[file] += statements

            if (count > 0) {
                covered[file] += statements
            }
        }

        END {
            for (file in total) {
                if (total[file] == 0) {
                    continue
                }

                percentage = covered[file] / total[file] * 100

                if (covered[file] == 0) {
                    status = "ZERO"
                } else if (covered[file] < total[file]) {
                    status = "PARTIAL"
                } else {
                    status = "FULL"
                }

                printf "%s|%s|%.1f|%d|%d\n", \
                    status, file, percentage, \
                    covered[file], total[file]
            }
        }
    ' "$profile" | sort -t '|' -k1,1 -k2,2 > "$SUMMARY_FILE"

    printf '\n===== COBERTURA POR ARQUIVO: %s =====\n' "$module"

    for status in FULL PARTIAL ZERO; do
        case "$status" in
            FULL)
                printf '\n100%% COBERTOS:\n'
                ;;
            PARTIAL)
                printf '\nPARCIALMENTE COBERTOS:\n'
                ;;
            ZERO)
                printf '\nSEM COBERTURA:\n'
                ;;
        esac

        awk -F '|' -v status="$status" '
            $1 == status {
                printf "  %-75s %5s%% (%s/%s)\n", \
                    $2, $3, $4, $5
                found = 1
            }

            END {
                if (!found) {
                    print "  Nenhum arquivo nesta categoria."
                }
            }
        ' "$SUMMARY_FILE"
    done
}

run_coverage() {
    local module
    local module_dir
    local profile
    local module_name

    for module in "${MODULES[@]}"; do
        module_dir="$ROOT_DIR/$module"
        module_name="${module//\//_}"
        profile="$COVERAGE_DIR/$module_name.out"

        if [[ ! -f "$module_dir/go.mod" ]]; then
            printf 'ERRO: go.mod não encontrado em %s\n' \
                "$module_dir" >&2
            return 1
        fi

        printf '\n===== COBERTURA: %s =====\n' "$module"

        (
            cd "$module_dir"
            go test -coverprofile="$profile" ./...
        )

        printf '\nCobertura total do módulo %s:\n' "$module"

        go tool cover -func="$profile" | tail -n 1

        print_file_summary "$profile" "$module"
    done
}

run_coverage

printf '\n===== RELATÓRIOS HTML =====\n'

INDEX_FILE="$COVERAGE_DIR/index.html"

cat > "$INDEX_FILE" <<'EOF'
<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>MemoryDB - Cobertura de Testes</title>
    <style>
        body {
            font-family: system-ui, sans-serif;
            max-width: 900px;
            margin: 40px auto;
            padding: 0 20px;
            line-height: 1.5;
        }

        h1 {
            margin-bottom: 8px;
        }

        p {
            color: #666;
        }

        ul {
            padding-left: 20px;
        }

        li {
            margin: 12px 0;
        }

        a {
            color: #0969da;
            text-decoration: none;
        }

        a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <h1>MemoryDB - Cobertura de Testes</h1>
    <p>Relatórios individuais de cobertura por módulo.</p>
    <ul>
EOF

for module in "${MODULES[@]}"; do
    module_name="${module//\//_}"
    profile="$COVERAGE_DIR/$module_name.out"
    report="$COVERAGE_DIR/$module_name.html"

    if [[ ! -f "$profile" ]]; then
        printf 'ERRO: perfil de cobertura não encontrado: %s\n' \
            "$profile" >&2
        return 1
    fi

    go tool cover -html="$profile" -o="$report"

    printf 'Relatório gerado: %s\n' "$report"

    printf '        <li><a href="%s.html" target="_blank" rel="noopener noreferrer">%s</a></li>\n' \
        "$module_name" "$module" >> "$INDEX_FILE"
done

cat >> "$INDEX_FILE" <<'EOF'
    </ul>
</body>
</html>
EOF

printf 'Índice gerado: %s\n' "$INDEX_FILE"

printf '\n===== ANÁLISE DE COBERTURA CONCLUÍDA =====\n'