
#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

source "$SCRIPT_DIR/common.sh"

COVERAGE_DIR="$ROOT_DIR/coverage"

if [[ ! -d "$COVERAGE_DIR" ]]; then
    printf 'ERRO: diretório de cobertura não encontrado: %s\n' \
        "$COVERAGE_DIR" >&2
    printf 'Execute primeiro: ./scripts/test.sh coverage\n' >&2
    exit 1
fi

REPORT_FILE="$(mktemp "${TMPDIR:-/tmp}/memorydb-uncovered.XXXXXX")"
SORTED_REPORT="$(mktemp "${TMPDIR:-/tmp}/memorydb-uncovered-sorted.XXXXXX")"

cleanup() {
    rm -f "$REPORT_FILE" "$SORTED_REPORT"
}

trap cleanup EXIT

analyze_profile() {
    local profile="$1"
    local module="$2"

    : > "$REPORT_FILE"

    LC_ALL=C awk '
        /^mode: / {
            next
        }

        {
            location = $1
            file = location
            sub(/:.*/, "", file)

            statements = $2 + 0
            count = $3 + 0

            total[file] += statements

            if (count > 0) {
                covered[file] += statements
                next
            }

            range = location
            sub(/^[^:]*:/, "", range)

            split(range, positions, ",")

            split(positions[1], start, ".")
            split(positions[2], finish, ".")

            block = start[1] "-" finish[1]

            blocks[file] = blocks[file] \
                block "|" statements "\n"
        }

        END {
            for (file in total) {
                if (total[file] == 0 || covered[file] >= total[file]) {
                    continue
                }

                percentage = covered[file] / total[file] * 100

                if (covered[file] == 0) {
                    printf "%s|0|ZERO|0.0|0|%d||\n", \
                        file, total[file]
                    continue
                }

                printf "%s|0|PARTIAL|%.1f|%d|%d||\n", \
                    file, percentage, covered[file], total[file]

                number_of_blocks = split(blocks[file], file_blocks, "\n")

                for (i = 1; i <= number_of_blocks; i++) {
                    if (file_blocks[i] == "") {
                        continue
                    }

                    split(file_blocks[i], block_data, "|")

                    printf "%s|1|BLOCK||||%s|%s\n", \
                        file, block_data[1], block_data[2]
                }
            }
        }
    ' "$profile" > "$REPORT_FILE"

    sort -t '|' -k1,1 -k2,2n -k7,7n "$REPORT_FILE" > "$SORTED_REPORT"

    printf '\n===== ANÁLISE DE COBERTURA: %s =====\n' "$module"

    if [[ ! -s "$SORTED_REPORT" ]]; then
        printf 'Todos os arquivos possuem 100%% de cobertura.\n'
        return 0
    fi

    while IFS='|' read -r file order type percentage covered total lines statements; do
        case "$type" in
            ZERO)
                printf '\n%s\n' "$file"
                printf '  Sem cobertura (0%%; %s instruções).\n' "$total"
                ;;

            PARTIAL)
                printf '\n%s\n' "$file"
                printf '  Cobertura parcial (%s%%; %s/%s instruções).\n' \
                    "$percentage" "$covered" "$total"
                printf '  Blocos não cobertos:\n'
                ;;

            BLOCK)
                printf '    Linhas %s (%s instruções)\n' \
                    "$lines" "$statements"
                ;;
        esac
    done < "$SORTED_REPORT"
}

for module in "${MODULES[@]}"; do
    module_name="${module//\//_}"
    profile="$COVERAGE_DIR/$module_name.out"

    if [[ ! -f "$profile" ]]; then
        printf '\nERRO: perfil não encontrado para %s: %s\n' \
            "$module" "$profile" >&2
        printf 'Execute primeiro: ./scripts/test.sh coverage\n' >&2
        exit 1
    fi

    analyze_profile "$profile" "$module"
done

printf '\n===== ANÁLISE DE COBERTURA NÃO EXECUTADA CONCLUÍDA =====\n'
