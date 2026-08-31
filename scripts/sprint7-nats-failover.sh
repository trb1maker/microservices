#!/usr/bin/env bash
# Sprint 7: NATS restart — verify JetStream file store survives and durable consumers resume.
# Run after `task demo:up`.

set -euo pipefail

COMPOSE="docker compose --env-file .env.example"
BASE_URL="${BASE_URL:-https://localhost:8080}"
NATS_MONITOR="${NATS_MONITOR:-http://localhost:8222}"
DEMO_EMAIL="${DEMO_EMAIL:-demo@example.com}"
DEMO_PASSWORD="${DEMO_PASSWORD:-demo123}"
E2E_PRODUCT_ID="${E2E_PRODUCT_ID:-22222222-2222-4222-8222-222222222222}"
CURL_OPTS=(--silent --show-error --insecure)

section() {
  echo
  echo "=== $1 ==="
}

login_token() {
  curl "${CURL_OPTS[@]}" -X POST "${BASE_URL}/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"${DEMO_EMAIL}\",\"password\":\"${DEMO_PASSWORD}\"}" \
    | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])'
}

try_checkout() {
  local label="$1"
  local token
  token="$(login_token)"

  local http_code
  http_code=$(curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' -X POST "${BASE_URL}/cart/items" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -d "{\"product_id\":\"${E2E_PRODUCT_ID}\",\"quantity\":1,\"unit_price\":2500}")

  if [[ "$http_code" != "201" ]]; then
    echo "${label}: add cart item HTTP ${http_code}"
    return 0
  fi

  http_code=$(curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' -X POST "${BASE_URL}/orders" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -d '{"delivery_address":"NATS Failover Test"}')

  echo "${label}: checkout HTTP ${http_code}"
}

js_snapshot() {
  curl -sf "${NATS_MONITOR}/jsz?streams=1&consumers=1" | python3 -c '
import sys, json
try:
    data = json.load(sys.stdin)
    details = (data.get("account_details") or [{}])[0]
    for stream in details.get("stream_detail") or []:
        state = stream.get("state", {})
        print(f"stream {stream.get(\"name\")}: messages={state.get(\"messages\")}")
    for consumer in details.get("consumer_detail") or []:
        print(f"consumer {consumer.get(\"name\")}: pending={consumer.get(\"num_pending\")}")
except Exception as exc:
    print(f"jsz parse error: {exc}")
' 2>/dev/null || echo "jsz unavailable"
}

section "Baseline JetStream"
js_snapshot
try_checkout "baseline checkout"

section "Restart NATS broker"
$COMPOSE restart nats
sleep 5

section "JetStream after restart (disk store should retain messages)"
js_snapshot
curl -sk -o /dev/null -w "order /ready HTTP %{http_code}\n" "${BASE_URL}/ready" || true
try_checkout "checkout after NATS restart"

section "Service reconnect"
$COMPOSE ps nats order-service analytics-service store-service notification-service payment-service 2>/dev/null || true

section "Done — capture logs for docs/SPRINT7_REPORT.md"
