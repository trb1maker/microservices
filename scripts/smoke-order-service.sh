#!/usr/bin/env bash
set -euo pipefail

IMAGE="${IMAGE:-order-service:ci}"
PORT="${PORT:-8080}"
JWT_SECRET="${JWT_SECRET:-ci-jwt-secret-minimum-32-characters-long}"
CONTAINER_NAME="${CONTAINER_NAME:-order-service-smoke-$$}"

cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run -d --name "$CONTAINER_NAME" \
  -e USE_MEMORY=true \
  -e JWT_SECRET="$JWT_SECRET" \
  -e TLS_CERT_FILE=/certs/server.crt \
  -e TLS_KEY_FILE=/certs/server.key \
  -e TLS_CLIENT_CA_FILE=/certs/ca.crt \
  -p "${PORT}:${PORT}" "$IMAGE"

for _ in $(seq 1 10); do
  if curl -kfsS "https://localhost:${PORT}/health"; then
    exit 0
  fi
  sleep 1
done

docker logs "$CONTAINER_NAME"
exit 1
