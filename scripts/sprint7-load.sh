#!/usr/bin/env bash
# Sprint 7: end-to-end load — checkout, pay, wait for receipt + FTS document.
set -euo pipefail

BASE_URL="${BASE_URL:-https://localhost:8080}"
ANALYTICS_URL="${ANALYTICS_URL:-http://localhost:8084}"
JWT_SECRET="${JWT_SECRET:-dev-jwt-secret-minimum-32-characters-long}"
DURATION="${DURATION:-30s}"
RATE="${RATE:-10}"
PREP_COUNT="${PREP_COUNT:-100}"
RESULTS_DIR="${RESULTS_DIR:-docs/load_results}"
CURL_OPTS=(--fail --show-error --silent --insecure)

mkdir -p "$RESULTS_DIR"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"

if ! command -v vegeta >/dev/null 2>&1; then
  echo "vegeta is required: go install github.com/tsenart/vegeta@latest" >&2
  exit 1
fi

new_uuid() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr '[:upper:]' '[:lower:]'
  else
    python3 -c 'import uuid; print(uuid.uuid4())'
  fi
}

mint_jwt() {
  JWT_SECRET="$JWT_SECRET" go run ./scripts/mint-jwt.go "$1"
}

users_file="$(mktemp)"
tokens_file="$(mktemp)"
order_ids_file="$(mktemp)"
checkout_targets="$(mktemp)"
pay_targets="$(mktemp)"
checkout_body="$(mktemp)"
trap 'rm -f "$users_file" "$tokens_file" "$order_ids_file" "$checkout_targets" "$pay_targets" "$checkout_body"' EXIT

echo '{"delivery_address":"Sprint7 Load Street"}' >"$checkout_body"

echo "Preparing $PREP_COUNT carts and checkouts..."
for _ in $(seq 1 "$PREP_COUNT"); do
  user_id="$(new_uuid)"
  token="$(mint_jwt "$user_id")"
  product_id="$(new_uuid)"
  curl "${CURL_OPTS[@]}" -X POST "$BASE_URL/cart/items" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"product_id\":\"$product_id\",\"quantity\":1,\"unit_price\":100}" >/dev/null

  order_resp=$(curl "${CURL_OPTS[@]}" -X POST "$BASE_URL/orders" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d @"$checkout_body")
  order_id=$(python3 -c 'import sys,json; print(json.load(sys.stdin)["order_id"])' <<<"$order_resp")

  echo "$user_id" >>"$users_file"
  echo "$token" >>"$tokens_file"
  echo "$order_id" >>"$order_ids_file"
done

paste "$tokens_file" "$order_ids_file" | while read -r token order_id; do
  printf 'POST %s/orders/%s/pay\nAuthorization: Bearer %s\n\n' "$BASE_URL" "$order_id" "$token" >>"$pay_targets"
done

paste "$tokens_file" | while read -r token; do
  printf 'POST %s/orders\nContent-Type: application/json\nAuthorization: Bearer %s\n@%s\n\n' \
    "$BASE_URL" "$token" "$checkout_body" >>"$checkout_targets"
done

echo "Running vegeta on POST /orders/{id}/pay: rate=$RATE duration=$DURATION"
vegeta attack -rate="$RATE" -duration="$DURATION" -targets="$pay_targets" \
  | tee "$RESULTS_DIR/sprint7_pay_${TIMESTAMP}.bin" \
  | vegeta report -type=text

vegeta report -type=json <"$RESULTS_DIR/sprint7_pay_${TIMESTAMP}.bin" >"$RESULTS_DIR/sprint7_pay_${TIMESTAMP}.json"

sample_order_id="$(head -1 "$order_ids_file")"
sample_token="$(head -1 "$tokens_file")"
echo "Sample receipt URL check for order ${sample_order_id}"
curl -sf -H "Authorization: Bearer ${sample_token}" "${ANALYTICS_URL}/receipts/${sample_order_id}" | head -c 300 || true
echo
curl -sf -H "Authorization: Bearer ${sample_token}" "${ANALYTICS_URL}/receipts/search?q=Sprint7" | head -c 300 || true
echo

echo "Results saved to $RESULTS_DIR/sprint7_pay_${TIMESTAMP}.*"
