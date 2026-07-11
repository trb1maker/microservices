#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CERTS_DIR="${ROOT_DIR}/deploy/certs"
DAYS=825
CA_SUBJ="/CN=Microservices Dev CA/O=Microservices/C=RU"

mkdir -p "${CERTS_DIR}/ca" "${CERTS_DIR}/nats" "${CERTS_DIR}/order-service" "${CERTS_DIR}/payment-service"

if [[ ! -f "${CERTS_DIR}/ca/ca.key" ]]; then
  openssl genrsa -out "${CERTS_DIR}/ca/ca.key" 4096
  openssl req -x509 -new -nodes -key "${CERTS_DIR}/ca/ca.key" -sha256 -days "${DAYS}" \
    -out "${CERTS_DIR}/ca/ca.crt" -subj "${CA_SUBJ}"
fi

generate_cert() {
  local name="$1"
  local cn="$2"
  local dir="$3"
  local san="$4"

  if [[ -f "${dir}/server.key" && -f "${dir}/server.crt" ]]; then
    return
  fi

  openssl genrsa -out "${dir}/server.key" 2048
  openssl req -new -key "${dir}/server.key" -out "${dir}/server.csr" \
    -subj "/CN=${cn}/O=Microservices/C=RU"

  cat > "${dir}/server.ext" <<EOF
subjectAltName = ${san}
extendedKeyUsage = serverAuth
EOF

  openssl x509 -req -in "${dir}/server.csr" \
    -CA "${CERTS_DIR}/ca/ca.crt" -CAkey "${CERTS_DIR}/ca/ca.key" -CAcreateserial \
    -out "${dir}/server.crt" -days "${DAYS}" -sha256 -extfile "${dir}/server.ext"
  rm -f "${dir}/server.csr" "${dir}/server.ext"
}

generate_client_cert() {
  local cn="$1"
  local dir="$2"

  if [[ -f "${dir}/client.key" && -f "${dir}/client.crt" ]]; then
    return
  fi

  openssl genrsa -out "${dir}/client.key" 2048
  openssl req -new -key "${dir}/client.key" -out "${dir}/client.csr" \
    -subj "/CN=${cn}/O=Microservices/C=RU"

  cat > "${dir}/client.ext" <<EOF
extendedKeyUsage = clientAuth
EOF

  openssl x509 -req -in "${dir}/client.csr" \
    -CA "${CERTS_DIR}/ca/ca.crt" -CAkey "${CERTS_DIR}/ca/ca.key" -CAcreateserial \
    -out "${dir}/client.crt" -days "${DAYS}" -sha256 -extfile "${dir}/client.ext"
  rm -f "${dir}/client.csr" "${dir}/client.ext"
}

generate_cert "nats" "nats" "${CERTS_DIR}/nats" "DNS:nats,DNS:localhost,IP:127.0.0.1"
generate_cert "order-service" "order-service" "${CERTS_DIR}/order-service" "DNS:order-service,DNS:localhost,IP:127.0.0.1"
generate_client_cert "order-service" "${CERTS_DIR}/order-service"
generate_client_cert "payment-service" "${CERTS_DIR}/payment-service"

echo "Certificates generated in ${CERTS_DIR}"
