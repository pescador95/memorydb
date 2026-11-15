#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PROTO_DIR="$ROOT_DIR/protocol/proto"
OUTPUT_DIR="$ROOT_DIR/protocol/generated/java"

mkdir -p "$OUTPUT_DIR"

protoc \
  --proto_path="$PROTO_DIR" \
  --java_out="$OUTPUT_DIR" \
  "$PROTO_DIR/memorydb/v1/memorydb.proto"

echo "Bindings Java gerados com sucesso."
