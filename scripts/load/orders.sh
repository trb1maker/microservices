#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-https://localhost:8080}"
JWT_SECRET="${JWT_SECRET:-dev-jwt-secret-minimum-32-characters-long}"
DURATION="${DURATION:-30s}"
RATE="${RATE:-30}"
PREP_COUNT="${PREP_COUNT:-600}"
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
  JWT_SECRET="$JWT_SECRET" go run ./scripts/jwt/mint.go "$1"
}

users_file="$(mktemp)"
tokens_file="$(mktemp)"
targets_file="$(mktemp)"
body_file="$(mktemp)"
trap 'rm -f "$users_file" "$tokens_file" "$targets_file" "$body_file"' EXIT

echo '{"delivery_address":"Load Test Street 1"}' >"$body_file"

echo "Preparing $PREP_COUNT carts..."
for _ in $(seq 1 "$PREP_COUNT"); do
  user_id="$(new_uuid)"
  token="$(mint_jwt "$user_id")"
  product_id="$(new_uuid)"
  curl "${CURL_OPTS[@]}" -X POST "$BASE_URL/cart/items" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $token" \
    -d "{\"product_id\":\"$product_id\",\"quantity\":1,\"unit_price\":100}" >/dev/null
  echo "$user_id" >>"$users_file"
  echo "$token" >>"$tokens_file"
done

paste "$users_file" "$tokens_file" | while read -r user_id token; do
  printf 'POST %s/orders\nContent-Type: application/json\nAuthorization: Bearer %s\n@%s\n\n' \
    "$BASE_URL" "$token" "$body_file" >>"$targets_file"
done

echo "Running vegeta attack on POST /orders: rate=$RATE duration=$DURATION"
vegeta attack -rate="$RATE" -duration="$DURATION" -targets="$targets_file" \
  | tee "$RESULTS_DIR/orders_${TIMESTAMP}.bin" \
  | vegeta report -type=text

vegeta report -type=json <"$RESULTS_DIR/orders_${TIMESTAMP}.bin" >"$RESULTS_DIR/orders_${TIMESTAMP}.json"
vegeta plot <"$RESULTS_DIR/orders_${TIMESTAMP}.bin" >"$RESULTS_DIR/orders_${TIMESTAMP}.html"

echo "Results saved to $RESULTS_DIR/orders_${TIMESTAMP}.*"
