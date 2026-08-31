#!/usr/bin/env bash
# Sprint 6: manual failover scenarios for PostgreSQL and MongoDB.
# Run after `task demo:up`. See docs/SPRINT6_REPORT.md for expected outcomes.

set -euo pipefail

COMPOSE="docker compose --env-file .env.example"
BASE_URL="${BASE_URL:-https://localhost:8080}"
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
    echo "${label}: add cart item HTTP ${http_code} (expected 201 on success path)"
    return 0
  fi

  http_code=$(curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' -X POST "${BASE_URL}/orders" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${token}" \
    -d '{"delivery_address":"Failover Test"}')

  echo "${label}: checkout HTTP ${http_code}"
}

wait_for_primary() {
  local attempts="${1:-30}"
  local i

  for ((i = 1; i <= attempts; i++)); do
    if $COMPOSE exec -T mongodb mongosh --quiet --eval "rs.isMaster().ismaster" 2>/dev/null | grep -q true; then
      echo "MongoDB PRIMARY is ready (attempt ${i})"
      return 0
    fi
    if $COMPOSE exec -T mongodb-2 mongosh --quiet --eval "rs.isMaster().ismaster" 2>/dev/null | grep -q true; then
      echo "MongoDB PRIMARY is ready on mongodb-2 (attempt ${i})"
      return 0
    fi
    if $COMPOSE exec -T mongodb-3 mongosh --quiet --eval "rs.isMaster().ismaster" 2>/dev/null | grep -q true; then
      echo "MongoDB PRIMARY is ready on mongodb-3 (attempt ${i})"
      return 0
    fi
    sleep 1
  done

  echo "MongoDB PRIMARY not ready after ${attempts}s"
  return 1
}

current_primary_host() {
  local primary
  primary=$($COMPOSE exec -T mongodb mongosh --quiet --eval \
    "rs.status().members.find(m => m.stateStr==='PRIMARY').name" 2>/dev/null | tr -d '\r')
  if [[ -z "$primary" ]]; then
    primary=$($COMPOSE exec -T mongodb-2 mongosh --quiet --eval \
      "rs.status().members.find(m => m.stateStr==='PRIMARY').name" 2>/dev/null | tr -d '\r')
  fi
  if [[ -z "$primary" ]]; then
    primary=$($COMPOSE exec -T mongodb-3 mongosh --quiet --eval \
      "rs.status().members.find(m => m.stateStr==='PRIMARY').name" 2>/dev/null | tr -d '\r')
  fi
  echo "${primary%%:*}"
}

section "Baseline health"
curl -sk "${BASE_URL}/health" | head -c 200 || true
echo
curl -sk "${BASE_URL}/ready" || true
echo
try_checkout "baseline checkout"

section "Redis cart TTL sample"
$COMPOSE exec -T redis redis-cli --scan --pattern 'cart:*' | head -5 | while read -r key; do
  echo "$key TTL=$($COMPOSE exec -T redis redis-cli TTL "$key")"
done

section "PostgreSQL replication status"
$COMPOSE exec -T postgres psql -U orders -d orders -c "SELECT pg_is_in_recovery() AS primary_is_recovery;" || true
$COMPOSE exec -T postgres-replica psql -U orders -d orders -c "SELECT pg_is_in_recovery() AS replica_is_recovery;" || true

section "MongoDB replica set status"
$COMPOSE exec -T mongodb mongosh --quiet --eval "rs.status().members.forEach(m => print(m.name, m.stateStr))" || true

section "Stop PostgreSQL primary (expect write failures)"
echo "Stopping postgres..."
$COMPOSE stop postgres
sleep 3
curl -sk -o /dev/null -w "order /ready HTTP %{http_code}\n" "${BASE_URL}/ready" || true
try_checkout "checkout while PG primary down"
echo "Replica still readable:"
$COMPOSE exec -T postgres-replica psql -U orders -d orders -c "SELECT count(*) FROM orders;" || true
echo "Starting postgres..."
$COMPOSE start postgres
sleep 5
curl -sk -o /dev/null -w "order /ready HTTP %{http_code}\n" "${BASE_URL}/ready" || true
try_checkout "checkout after PG primary up"

section "Stop MongoDB secondary"
PRIMARY_HOST="$(current_primary_host)"
echo "Current PRIMARY host: ${PRIMARY_HOST:-unknown}"
if [[ "$PRIMARY_HOST" == "mongodb-2" ]]; then
  TARGET=mongodb-3
elif [[ "$PRIMARY_HOST" == "mongodb-3" ]]; then
  TARGET=mongodb-2
else
  TARGET=mongodb-2
fi
echo "Stopping secondary ${TARGET}..."
$COMPOSE stop "$TARGET"
sleep 3
$COMPOSE exec -T mongodb mongosh --quiet --eval "rs.status().members.forEach(m => print(m.name, m.stateStr))" || true
try_checkout "checkout while Mongo secondary down"
echo "Starting secondary ${TARGET}..."
$COMPOSE start "$TARGET"
wait_for_primary 30 || true
try_checkout "checkout after Mongo secondary up"

section "Stop MongoDB primary (expect brief errors, then new PRIMARY)"
PRIMARY_HOST="$(current_primary_host)"
if [[ -n "$PRIMARY_HOST" ]]; then
  echo "Stopping primary container ${PRIMARY_HOST}..."
  $COMPOSE stop "$PRIMARY_HOST"
  sleep 5
  $COMPOSE exec -T mongodb mongosh --quiet --eval "rs.status().members.forEach(m => print(m.name, m.stateStr))" 2>/dev/null || \
    $COMPOSE exec -T mongodb-2 mongosh --quiet --eval "rs.status().members.forEach(m => print(m.name, m.stateStr))" 2>/dev/null || \
    $COMPOSE exec -T mongodb-3 mongosh --quiet --eval "rs.status().members.forEach(m => print(m.name, m.stateStr))" 2>/dev/null || true
  try_checkout "checkout while Mongo primary down"
  echo "Starting former primary ${PRIMARY_HOST}..."
  $COMPOSE start "$PRIMARY_HOST"
  wait_for_primary 30 || true
  try_checkout "checkout after Mongo primary restored"
fi

section "Done — capture logs/metrics for docs/SPRINT6_REPORT.md"
