#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MODULE="github.com/trb1maker/microservices/internal/platform"

while IFS= read -r -d '' f; do
  relpath="${f#api/}"
  protoc --proto_path=api \
    --go_out=internal/platform --go_opt=module="${MODULE}" \
    --go-grpc_out=internal/platform --go-grpc_opt=module="${MODULE}" \
    "$relpath"
done < <(find api -name '*.proto' -print0 | sort -z)
