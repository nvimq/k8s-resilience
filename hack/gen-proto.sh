#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
PROTO_DIR="$PROJECT_DIR/api/proto"

export PATH="$PATH:$(go env GOPATH)/bin"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$PROTO_DIR" \
  --go_opt=paths=source_relative \
  --go-grpc_out="$PROTO_DIR" \
  --go-grpc_opt=paths=source_relative \
  "$PROTO_DIR"/worker/v1/*.proto

echo "proto: generated Go code in $PROTO_DIR"
