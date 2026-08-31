# Analytics Service — REST API и поиск

Analytics Service — NATS-воркер **и** HTTP API на `HTTP_ADDR` (default `:8084`):

1. **JetStream consumer** — `orders.finalized` → чек в MinIO, сводка и FTS-документ в PostgreSQL.
2. **REST** (JWT, тот же `JWT_SECRET`, что у order-service):
   - `GET /receipts/{order_id}` — `{ "url": "...", "expires_in": 900 }` (секунды, presigned URL через `MINIO_PUBLIC_ENDPOINT`)
   - `GET /receipts/search?q=&limit=` — полнотекстовый поиск по своим чекам (`limit` default 20, max 100)
3. **Metrics/health** — `METRICS_ADDR` (default `:9094`): `/health`, `/ready` (postgres + minio + nats), `/metrics`.

Presigned URL подписывается с `MINIO_PUBLIC_ENDPOINT` (для браузера с хоста), объект пишется через `MINIO_ENDPOINT` (внутри compose).

Поиск — таблица `receipt_documents` (GIN на `tsv`), альтернатива Elasticsearch для demo.

См. также [SPRINT7_REPORT.md](SPRINT7_REPORT.md) и [DESIGN.md](DESIGN.md) ADR 6/9.
