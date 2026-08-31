# Отчёт спринта 8: отказоустойчивость

**Период:** 01.09 – 07.09  
**Цель:** Circuit breaker и timeout на gRPC Payment, NAK backoff+jitter, outbox для saga-событий order, CQRS-документация, проверка Payment-down без зависания.

---

## 1. Что реализовано

| Задача | Статус | Реализация |
| ------ | ------ | ---------- |
| Circuit breaker Payment gRPC | Готово | [`internal/platform/breaker`](../internal/platform/breaker/), обёртка в [`payment/grpc/client.go`](../internal/order-service/adapters/payment/grpc/client.go) |
| RPC timeout | Готово | `PAYMENT_RPC_TIMEOUT` (default 2s), HTTP 503 при недоступности |
| NAK backoff + jitter | Готово | [`internal/platform/retry`](../internal/platform/retry/), [`natsx/consume.go`](../internal/platform/natsx/consume.go) + `NumDelivered` |
| Outbox confirm/finalize/cancel | Готово | [`checkout_writer/postgres/writer.go`](../internal/order-service/adapters/checkout_writer/postgres/writer.go), `OrderService` |
| CQRS | Документировано | ADR 11 — PG replica hot standby, без read URL в рантайме |
| ADR / отчёт | Готово | ADR 9–11 в [`DESIGN.md`](DESIGN.md), этот файл |
| Тесты | Готово | unit breaker/retry/nats, integration Payment-down, e2e happy path |

---

## 2. gRPC Payment: timeout и circuit breaker

```
POST /orders/{id}/pay
  → context.WithTimeout(PAYMENT_RPC_TIMEOUT)
  → breaker.Execute(Charge)
  → payment-service:50051
```

- **503 Service Unavailable:** deadline, `Unavailable`, breaker open.
- **402 Payment Required:** успешный gRPC с `PaymentStatus_FAILED` (недостаточно средств).
- **CheckHealth** дашборда — без breaker (отдельный короткий probe).

Настройки breaker (defaults): 5 consecutive transport failures → open 10s → half-open 1 probe.

---

## 3. NATS NAK backoff

При ошибке handler:

1. `msg.Metadata().NumDelivered` → номер попытки.
2. `retry.BackoffWithJitter(attempt, 2s, 30s, ±20%)`.
3. `NakWithDelay(delay)`; `MaxDeliver=10`.

Inbox по-прежнему dedup по `Nats-Msg-Id` до handler.

---

## 4. Order saga outbox

| Событие | Когда | TX |
| ------- | ----- | -- |
| `orders.created` | Checkout | order + outbox |
| `orders.confirm` | PayOrder (по item) | order PAID + outbox |
| `orders.finalized` | HandleOrderConfirmed | order CONFIRMED + outbox |
| `orders.cancelled` | Cancel / reservation failed | order CANCELLED + outbox |

Relay общий (`internal/platform/outbox`); cart reserve/release — прямая публикация (без изменений).

---

## 5. CQRS

- **Primary:** все сервисы (`DATABASE_URL` → `postgres:5432`).
- **Replica:** `postgres-replica:5433` в compose для failover/демо ([`scripts/sprint6-failover.sh`](../scripts/sprint6-failover.sh)).
- Read path с replica не подключён — зафиксировано в ADR 11.

---

## 6. Payment-down

Integration: `POST /pay` с payment client на `127.0.0.1:1` → **503** за &lt; 2s, заказ остаётся **RESERVED**.

---

## 7. Тесты

```bash
task fmt
task lint
task test:unit
task test:integration
task test:e2e
```

---

## 8. Выводы

- HTTP не зависает при падении Payment: timeout + breaker дают предсказуемый 503.
- Saga-события order теперь атомарны с состоянием заказа через outbox.
- JetStream redelivery не долбит handler фиксированным 2s NAK.

**Звёздочки (не делали):** Temporal saga coordinator, стресс-тест при одновременном отключении нескольких сервисов.
