#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/dist"
OUTPUT_FILE="$OUTPUT_DIR/memorydb.exe"

if ! command -v go >/dev/null 2>&1; then
    echo "Erro: Go não encontrado no PATH."
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

echo "Compilando MemoryDB para Windows AMD64..."

CGO_ENABLED=0 \
GOOS=windows \
GOARCH=amd64 \
go -C "$ROOT_DIR/server" build \
    -trimpath \
    -ldflags="-s -w" \
    -o "$OUTPUT_FILE" \
    ./cmd/memorydb

if [[ ! -f "$OUTPUT_FILE" ]]; then
    echo "Erro: executável não foi gerado."
    exit 1
fi

echo "Compilação concluída!"
echo "Arquivo: $OUTPUT_FILE"