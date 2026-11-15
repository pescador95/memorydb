#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PROTO_DIR="$ROOT_DIR/protocol/proto"
OUTPUT_DIR="$ROOT_DIR/protocol/generated/go"
GO_MODULE="github.com/pescador95/memorydb/protocol/generated/go"

mkdir -p "$OUTPUT_DIR"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$OUTPUT_DIR" \
  --go_opt="module=$GO_MODULE" \
  "$PROTO_DIR/memorydb/v1/memorydb.proto"

echo "Bindings Go gerados com sucesso."