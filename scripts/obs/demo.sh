#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://localhost:8080}"
DEMO_EMAIL="${DEMO_EMAIL:-demo@example.com}"
DEMO_PASSWORD="${DEMO_PASSWORD:-demo123}"
CHECKOUT_COUNT="${CHECKOUT_COUNT:-20}"
ERROR_DURATION_SEC="${ERROR_DURATION_SEC:-75}"
ERROR_RPS="${ERROR_RPS:-5}"
CURL_OPTS=(--fail --show-error --silent --insecure)

cart_item_file="$(mktemp)"
checkout_file="$(mktemp)"
trap 'rm -f "$cart_item_file" "$checkout_file"' EXIT

echo "Generating successful traffic..."
TOKEN=$(curl "${CURL_OPTS[@]}" -X POST "${BASE_URL}/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${DEMO_EMAIL}\",\"password\":\"${DEMO_PASSWORD}\"}" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')

PRODUCT_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
echo "{\"product_id\":\"${PRODUCT_ID}\",\"quantity\":1,\"unit_price\":100}" >"$cart_item_file"
curl "${CURL_OPTS[@]}" -X POST "${BASE_URL}/cart/items" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${TOKEN}" \
  -d @"$cart_item_file" >/dev/null

echo '{"delivery_address":"Moscow"}' >"$checkout_file"
for _ in $(seq 1 "$CHECKOUT_COUNT"); do
  curl "${CURL_OPTS[@]}" -X POST "${BASE_URL}/orders" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${TOKEN}" \
    -d @"$checkout_file" >/dev/null
done

echo "Generating sustained 5xx traffic for alert demo (${ERROR_DURATION_SEC}s)..."
for _ in $(seq 1 "$ERROR_DURATION_SEC"); do
  for _ in $(seq 1 "$ERROR_RPS"); do
    curl "${CURL_OPTS[@]}" "${BASE_URL}/debug/error" >/dev/null &
  done
  wait
  sleep 1
done

echo "Done. Check Grafana http://localhost:3000 and Alertmanager http://localhost:9093"
