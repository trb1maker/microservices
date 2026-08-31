#!/bin/sh
set -eu

PGDATA="${PGDATA:-/var/lib/postgresql/data}"
PRIMARY_HOST="${PRIMARY_HOST:-postgres}"
PRIMARY_PORT="${PRIMARY_PORT:-5432}"
REPLICATION_USER="${REPLICATION_USER:-replicator}"
: "${REPLICATION_PASSWORD:?REPLICATION_PASSWORD is required}"

echo "Waiting for primary at ${PRIMARY_HOST}:${PRIMARY_PORT}..."
until pg_isready -h "$PRIMARY_HOST" -p "$PRIMARY_PORT" -U "${POSTGRES_USER:-orders}"; do
  sleep 1
done

if [ ! -s "${PGDATA}/PG_VERSION" ]; then
  echo "Initializing replica from primary..."
  rm -rf "${PGDATA:?}"/*
  export PGPASSWORD="$REPLICATION_PASSWORD"
  pg_basebackup \
    -h "$PRIMARY_HOST" \
    -p "$PRIMARY_PORT" \
    -U "$REPLICATION_USER" \
    -D "$PGDATA" \
    -Fp \
    -Xs \
    -P \
    -R
fi

exec docker-entrypoint.sh postgres
