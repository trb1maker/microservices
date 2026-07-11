#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DURATION="${DURATION:-30s}"
RATE="${RATE:-30}"
PREP_COUNT="${PREP_COUNT:-600}"
RESULTS_DIR="${RESULTS_DIR:-docs/load_results}"

mkdir -p "$RESULTS_DIR"
TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"

if ! command -v vegeta >/dev/null 2>&1; then
  echo "vegeta is required: go install github.com/tsenart/vegeta@latest" >&2
  exit 1
fi

new_uuid() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen
  else
    python3 -c 'import uuid; print(uuid.uuid4())'
  fi
}

users_file="$(mktemp)"
targets_file="$(mktemp)"
body_file="$(mktemp)"
trap 'rm -f "$users_file" "$targets_file" "$body_file"' EXIT

echo '{"delivery_address":"Load Test Street 1"}' >"$body_file"

echo "Preparing $PREP_COUNT carts..."
for _ in $(seq 1 "$PREP_COUNT"); do
  user_id="$(new_uuid)"
  product_id="$(new_uuid)"
  curl -fsS -X POST "$BASE_URL/cart/items" \
    -H "Content-Type: application/json" \
    -H "X-User-ID: $user_id" \
    -d "{\"product_id\":\"$product_id\",\"quantity\":1,\"unit_price\":100}" >/dev/null
  echo "$user_id" >>"$users_file"
done

while IFS= read -r user_id; do
  printf 'POST %s/orders\nContent-Type: application/json\nX-User-ID: %s\n@%s\n\n' \
    "$BASE_URL" "$user_id" "$body_file" >>"$targets_file"
done <"$users_file"

echo "Running vegeta attack on POST /orders: rate=$RATE duration=$DURATION"
vegeta attack -rate="$RATE" -duration="$DURATION" -targets="$targets_file" \
  | tee "$RESULTS_DIR/orders_${TIMESTAMP}.bin" \
  | vegeta report -type=text

vegeta report -type=json <"$RESULTS_DIR/orders_${TIMESTAMP}.bin" >"$RESULTS_DIR/orders_${TIMESTAMP}.json"
vegeta plot <"$RESULTS_DIR/orders_${TIMESTAMP}.bin" >"$RESULTS_DIR/orders_${TIMESTAMP}.html"

echo "Results saved to $RESULTS_DIR/orders_${TIMESTAMP}.*"
