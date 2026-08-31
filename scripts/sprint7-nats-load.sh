#!/usr/bin/env bash
# Sprint 7: JetStream load test — publish thousands of messages and inspect stream lag.
# Run after `task demo:up`.

set -euo pipefail

COMPOSE="docker compose --env-file .env.example"
NATS_MONITOR="${NATS_MONITOR:-http://localhost:8222}"
PUBLISH_COUNT="${PUBLISH_COUNT:-5000}"
SUBJECT="${SUBJECT:-orders.created}"

section() {
  echo
  echo "=== $1 ==="
}

section "JetStream stream info (before)"
curl -sf "${NATS_MONITOR}/jsz?streams=1" | python3 -c '
import sys, json
data = json.load(sys.stdin)
for name, info in (data.get("account_details") or [{}])[0].get("stream_detail") or []:
    print(f"{name}: messages={info.get(\"state\", {}).get(\"messages\", \"?\")} bytes={info.get(\"state\", {}).get(\"bytes\", \"?\")}")
' 2>/dev/null || curl -sf "${NATS_MONITOR}/jsz?streams=1" | head -c 500 || true
echo

section "Publishing ${PUBLISH_COUNT} messages to ${SUBJECT}"
start_ms=$(python3 -c 'import time; print(int(time.time()*1000))')

for i in $(seq 1 "$PUBLISH_COUNT"); do
  payload=$(printf '{"order_id":"load-%06d","user_id":"11111111-1111-4111-8111-111111111111","total_amount":100}' "$i")
  echo "$payload" | $COMPOSE exec -T nats nats pub "$SUBJECT" --server nats://nats:4222 2>/dev/null || {
    echo "nats CLI inside compose unavailable; using Go publisher fallback"
    break
  }
done

end_ms=$(python3 -c 'import time; print(int(time.time()*1000))')
elapsed=$((end_ms - start_ms))
if [[ "$elapsed" -gt 0 ]]; then
  rps=$((PUBLISH_COUNT * 1000 / elapsed))
  echo "Published ~${PUBLISH_COUNT} messages in ${elapsed}ms (~${rps} msg/s)"
fi

if command -v go >/dev/null 2>&1; then
  section "Go JetStream publisher fallback"
  NATS_URL="${NATS_URL:-tls://localhost:4222}" \
  NATS_TLS_CERT_FILE="${NATS_TLS_CERT_FILE:-deploy/certs/order-service/client.crt}" \
  NATS_TLS_KEY_FILE="${NATS_TLS_KEY_FILE:-deploy/certs/order-service/client.key}" \
  NATS_TLS_CA_FILE="${NATS_TLS_CA_FILE:-deploy/certs/ca/ca.crt}" \
  PUBLISH_COUNT="$PUBLISH_COUNT" SUBJECT="$SUBJECT" \
    go run ./scripts/sprint7-publish.go
fi

section "JetStream stream info (after)"
curl -sf "${NATS_MONITOR}/jsz?streams=1&consumers=1" | python3 -c '
import sys, json
data = json.load(sys.stdin)
details = (data.get("account_details") or [{}])[0]
for stream in details.get("stream_detail") or []:
    state = stream.get("state", {})
    print(f"stream {stream.get(\"name\")}: messages={state.get(\"messages\")} first_seq={state.get(\"first_seq\")} last_seq={state.get(\"last_seq\")}")
for consumer in details.get("consumer_detail") or []:
    print(f"consumer {consumer.get(\"name\")}: pending={consumer.get(\"num_pending\")} ack_pending={consumer.get(\"num_ack_pending\")}")
' 2>/dev/null || curl -sf "${NATS_MONITOR}/jsz?streams=1&consumers=1" | head -c 800 || true
echo

section "Done — capture output for docs/SPRINT7_REPORT.md"
