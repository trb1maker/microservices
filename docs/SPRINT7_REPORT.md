# Отчёт спринта 7: JetStream, Outbox, MinIO, поиск

**Период:** 17.08 – 31.08  
**Цель:** Дисковый JetStream + durable consumers, Transactional Outbox на checkout, presigned URL на чеки, PostgreSQL FTS вместо Elasticsearch, нагрузка и restart NATS.

---

## 1. Что реализовано

| Задача | Статус | Реализация |
| ------ | ------ | ---------- |
| JetStream file store | Готово | `nats -sd=/data`, volume `nats-js`, пакет [`internal/platform/natsx`](../internal/platform/natsx/) |
| Durable + Ack | Готово | 4 стрима (`ORDERS`, `CART`, `STORE`, `PAYMENT`), explicit Ack/Nak, `MaxDeliver`, `NakWithDelay` |
| Transactional Outbox | Готово | таблица `outbox`, [`checkout_writer/postgres`](../internal/order-service/adapters/checkout_writer/postgres/), relay [`outbox_relay`](../internal/order-service/adapters/outbox_relay/) с `FOR UPDATE SKIP LOCKED` |
| MinIO presigned URL | Готово | `PresignGet`, `GET /receipts/{order_id}` на analytics `:8084`, `MINIO_PUBLIC_ENDPOINT` для браузера |
| PostgreSQL FTS | Готово | `receipt_documents`, GIN, `GET /receipts/search?q=` |
| Нагрузка / failover NATS | Готово | `scripts/sprint7-*.sh`, `task sprint7:*` |
| Тесты | Готово | unit analytics, integration analytics+outbox, e2e happy path + receipt/search |

---

## 2. JetStream

### 2.1 Стримы (file storage, MaxAge 7d)

| Stream | Subjects |
| ------ | -------- |
| `ORDERS` | `orders.>` |
| `CART` | `cart.>` |
| `STORE` | `store.>` |
| `PAYMENT` | `payment.>` |

### 2.2 Durable consumers (фактические имена)

| Consumer | Subject | Сервис |
| -------- | ------- | ------ |
| `analytics-orders-finalized` | `orders.finalized` | analytics (`DeliverNew`) |
| `store-reserve-items` | `cart.reserve_items` | store |
| `store-confirm-order` | `orders.confirm` | store |
| `store-release-reservation` | `cart.release_reservation` | store |
| `order-store-items-reserved` | `store.items_reserved` | order |
| `order-store-reservation-failed` | `store.reservation_failed` | order |
| `order-store-order-confirmed` | `store.order_confirmed` | order |
| `notification-orders-finalized` | `orders.finalized` | notification (`DeliverNew`) |
| `notification-orders-cancelled` | `orders.cancelled` | notification |
| `notification-payment-succeeded` | `payment.succeeded` | notification |
| `notification-refund-succeeded` | `payment.refund_succeeded` | notification |

Публикация checkout — через outbox relay; остальные события — `natsx.Publish` / `PublishMsg` с OTEL headers.

---

## 3. Transactional Outbox

```
Checkout → TX(order + outbox row) → 201
Relay (500ms) → claim FOR UPDATE SKIP LOCKED → JetStream orders.created → published_at
```

- Только **checkout** через outbox; `orders.finalized`, confirm/cancel — прямая JS-публикация (с повторной публикацией finalized при redelivery после save).
- Memory/tests без postgres writer: `ImmediateCheckoutWriter` (sync publish + rollback).
- Мониторинг relay: `PendingCount()` + SQL `SELECT COUNT(*) FROM outbox WHERE published_at IS NULL`.

---

## 4. MinIO и REST Analytics

- Чек: `receipts/{order_id}.json`
- `GET /receipts/{order_id}` → `{ "url", "expires_in" }` — `expires_in` в секундах (JWT, проверка `user_id`, 404 на чужой чек)
- TTL: `RECEIPT_URL_TTL=15m`; presigned URL подписывается через `MINIO_PUBLIC_ENDPOINT` (default `localhost:9000`)
- Metrics/health: `:9094` (`/health`, `/ready` с postgres+nats+minio, `/metrics`); API: `HTTP_ADDR=:8084`

---

## 5. PostgreSQL FTS (альтернатива Elasticsearch)

**Почему не ES:** учебный compose без тяжёлого узла; тот же паттерн — индексатор на `orders.finalized` + query API.

```sql
tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', search_text)) STORED
CREATE INDEX receipt_documents_tsv_idx ON receipt_documents USING GIN (tsv);
```

Поиск: `plainto_tsquery('simple', q)` + фильтр `user_id`, `limit` ≤ 100.

---

## 6. Нагрузка и отказ NATS

Запуск: `task demo:up` → скрипты спринта 7.

| Команда | Назначение |
| ------- | ---------- |
| `task sprint7:nats-load` | ~5000 сообщений в JetStream через `scripts/sprint7-publish.go`, `curl :8222/jsz` |
| `task sprint7:nats-failover` | `docker compose restart nats`, checkout после reconnect |
| `task sprint7:load` | vegeta checkout→pay, sample receipt URL + search |

**Ожидаемое поведение после restart NATS:** сообщения на диске сохраняются; durable consumers продолжают с последнего Ack; сервисы переподключаются к брокеру.

*Замеры RPS/p95 фиксируются при прогоне на demo-стеке и сохраняются в `docs/load_results/`.*

---

## 7. Тесты

```bash
go test ./internal/analytics-service/...
go test ./internal/platform/natsx/...
go test -tags=integration ./tests/integration/analytics-service/...
go test -tags=integration ./tests/integration/order-service/ -run Outbox
go test -tags=e2e ./tests/e2e/ -run HappyPath
```

---

## 8. Выводы

- JetStream на диске даёт персистентность без кластера; для production нужен кластер + репликация стримов.
- Outbox убирает race между commit заказа и `orders.created`.
- Presigned URL не проксирует большие JSON через API.
- PostgreSQL FTS достаточен для демонстрации search pipeline; ES — для полнотекстовой аналитики в масштабе.

**Звёздочки (не делали):** S3 tags/multipart/versioning, Cassandra, NATS cluster.
