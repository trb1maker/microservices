#!/usr/bin/env bash
set -euo pipefail

ENDPOINT="${MINIO_ENDPOINT:-http://minio:9000}"
ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
BUCKET="${MINIO_BUCKET:-receipts}"

until mc alias set local "${ENDPOINT}" "${ACCESS_KEY}" "${SECRET_KEY}"; do
  echo "waiting for minio..."
  sleep 2
done

mc mb --ignore-existing "local/${BUCKET}"
echo "bucket ${BUCKET} is ready"
