# Отчёт спринта 5: Взаимодействие сервисов (REST, gRPC, NATS, Saga)

**Период:** 20.07 – 03.08  
**Сервисы:** `order-service`, `payment-service`, `store-service`, `notification-service`, `analytics-service`, UI gateway  
**Цель:** реализовать все микросервисы платформы, протоколы REST / gRPC / NATS, Saga-хореографию заказа и demo-клиент.

---

## 1. Архитектура Saga

Хореография через **NATS JetStream**: Order Service публикует команды, Store и Payment отвечают событиями, Order обновляет статус заказа.

### Жизненный цикл заказа

| Статус      | Когда наступает                                      |
| ----------- | ---------------------------------------------------- |
| `RESERVED`  | Store подтвердил резерв (`store.items_reserved`)     |
| `PAID`      | Payment успешно списал средства (gRPC `Charge`)    |
| `CONFIRMED` | Store подтвердил отгрузку (`store.order_confirmed`) |
| `CANCELLED` | Отмена пользователем или компенсация после сбоя    |

### Основные NATS subjects

| Subject                    | Направление              | Назначение                    |
| -------------------------- | ------------------------ | ----------------------------- |
| `cart.reserve_items`       | Order → Store            | команда резервирования        |
| `store.items_reserved`     | Store → Order            | успешный резерв               |
| `store.reservation_failed` | Store → Order            | отказ склада                  |
| `orders.confirm`           | Order → Store            | подтверждение после оплаты    |
| `store.order_confirmed`    | Store → Order            | товар списан со склада        |
| `cart.release_reservation` | Order → Store            | освобождение резерва          |
| `payment.succeeded`        | Payment → подписчики     | успешная оплата               |
| `payment.refund_succeeded` | Payment → подписчики     | успешный возврат              |
| `orders.finalized`         | Order → Analytics/Notif  | заказ завершён                |
| `orders.cancelled`           | Order → Analytics/Notif  | заказ отменён                 |

### Схема happy path

```
Checkout (REST) → reserve (NATS) → RESERVED
Pay (REST) → Charge (gRPC/mTLS) → PAID → confirm (NATS) → CONFIRMED
                                    ↓
                          orders.finalized → Notification + Analytics
```

---

## 2. Payment Service

gRPC-сервер с методами **Charge** и **Refund** ([api/payment/payment.proto](api/payment/payment.proto)).

| Компонент    | Реализация                                                                 |
| ------------ | -------------------------------------------------------------------------- |
| Хранение     | PostgreSQL (счета, транзакции, optimistic locking)                         |
| События      | NATS: `payment.succeeded`, `payment.failed`, `payment.refund_*`          |
| Безопасность | mTLS server cert (`payment-service/server.crt`)                            |
| Метрики      | `GET :9091/ready`, `GET :9091/metrics`                                     |

Demo-счета создаёт миграция `20260719120000_demo_user_accounts.sql` для пользователей `demo@example.com` и `admin@example.com`.

---

## 3. Store Service

NATS-воркер без REST API — только команды и события.

| Команда (subscribe)          | Действие                          |
| ---------------------------- | --------------------------------- |
| `cart.reserve_items`         | резерв товара в MongoDB           |
| `orders.confirm`             | списание после оплаты             |
| `cart.release_reservation`   | освобождение при отмене           |

MongoDB хранит агрегаты товаров (`stock`, `reserved`). Seed-товары с UUID используются в Demo UI.

---

## 4. Notification Service

NATS consumer подписан на:

- `orders.finalized`, `orders.cancelled`
- `payment.succeeded`, `payment.refund_succeeded`

Уведомления пишутся в **stdout** через structured logging (`slog`) — [notifier/slog_notifier.go](../services/notification-service/internal/adapters/notifier/slog_notifier.go).

---

## 5. Analytics Service

NATS consumer на `orders.finalized`:

1. JSON-чек заказа сохраняется в **MinIO** (бакет `receipts`, ключ = `order_id`)
2. Idempotent upsert в PostgreSQL таблицу `daily_summary` (orders count, revenue, avg)

Метрики: `GET :9094/ready`.

---

## 6. Order Service

### REST API (JWT)

| Метод    | Путь                   | Описание              |
| -------- | ---------------------- | --------------------- |
| POST     | `/orders`              | checkout → RESERVED   |
| GET      | `/orders`              | список заказов        |
| GET      | `/orders/{id}`         | детали заказа         |
| POST     | `/orders/{id}/pay`     | оплата → PAID         |
| DELETE   | `/orders/{id}`         | отмена → CANCELLED    |
| GET      | `/swagger/`            | Swagger UI (swaggo)   |

Генерация: `task gen:swagger`.

### gRPC

| Направление | Метод               | Назначение                          |
| ----------- | ------------------- | ----------------------------------- |
| Client      | `Payment.Charge`    | синхронная оплата (mTLS)            |
| Client      | `Payment.Refund`    | возврат при отмене / компенсации    |
| Server      | `WatchOrderStatus`  | server-stream статусов для UI       |

### NATS consumers

Order Service слушает `store.items_reserved`, `store.reservation_failed`, `store.order_confirmed` и маршрутизует переходы Saga.

---

## 7. Health dashboard

`GET /health` с заголовком `Accept: text/html` возвращает HTML-страницу ([status_page.go](../services/order-service/internal/adapters/http/status_page.go)):

- liveness Order Service + readiness (PostgreSQL, Redis, NATS)
- remote checks Payment (`:9091/ready`) и Store (`:9092/ready`)

Без `Accept: text/html` — JSON `{"status":"ok"}` (совместимость с CI smoke).

**Проверка:**

```bash
curl -k -H 'Accept: text/html' https://localhost:8080/health
```

---

## 8. Demo UI (HTMX gateway)

Go UI gateway в [scripts/ui/](../scripts/ui/):

- Login → session cookie (JWT не попадает в браузерный JS)
- Корзина, checkout, pay, cancel через REST к order-service
- **SSE** `/orders/{id}/stream` — bridge из gRPC `WatchOrderStatus`

| URL                         | Назначение        |
| --------------------------- | ----------------- |
| http://localhost:8081       | Demo UI           |
| https://localhost:8080      | order-service API |

**Поведение статусов в UI:**

- Строка **Status:** обновляется после Pay (`PAID`) через HTMX panel swap
- **CONFIRMED** появляется в блоке **Live status updates** через SSE (через несколько секунд после confirm на складе)

**Demo-пользователи:**

| Email             | Password | User ID                              |
| ----------------- | -------- | ------------------------------------ |
| demo@example.com  | demo123  | 11111111-1111-4111-8111-111111111111 |
| admin@example.com | admin123 | 22222222-2222-4222-8222-222222222222 |

---

## 9. Тестирование

| Уровень       | Где                                      | Сценарии                                      |
| ------------- | ---------------------------------------- | --------------------------------------------- |
| Unit          | каждый сервис `internal/app/*_test.go`   | доменная логика                               |
| Integration   | `services/*/internal/integration/`       | Testcontainers (PG, Mongo, NATS, Redis, MinIO)|
| E2E           | [tests/e2e/](../tests/e2e/)              | полная Saga in-process через `testwire`       |
| CI            | `.github/workflows/ci.yml`               | matrix lint/test/build/docker + E2E job       |

### E2E-сценарии

| Тест                              | Сценарий                                      |
| --------------------------------- | --------------------------------------------- |
| `TestE2E_HappyPath`               | checkout → pay → CONFIRMED + receipt + notify |
| `TestE2E_StoreFailureAfterPayment`| confirm fail → refund → CANCELLED             |
| `TestE2E_UserCancellation`        | cancel до pay → release stock                 |

### Order integration

`TestIntegration_SagaHappyPath`, `TestIntegration_CancelOrder`, `TestIntegration_CancelPaidOrder`, `TestIntegration_ListOrders`.

---

## 10. Demo-стек

```bash
task certs:generate
cp .env.example .env
task demo:up
```

Поднимает все 5 сервисов, UI, PostgreSQL, MongoDB, Redis, NATS (mTLS), MinIO, observability.

Ключевые исправления compose:

- NATS 2.14 TLS flags (`--tlscacert`, `--tlsverify`)
- server cert для `payment-service` в `scripts/certs/generate.sh`
- order→payment gRPC с **client** certs (`NATS_TLS_*` pattern)
- Dockerfiles копируют все `go.mod` workspace перед `go mod download`

---

## 11. Новые пакеты и файлы

| Путь                         | Назначение                              |
| ---------------------------- | --------------------------------------- |
| `api/payment/`, `api/order/` | proto-схемы                             |
| `pkg/proto/`                 | сгенерированный Go (`task proto:gen`)   |
| `pkg/health/`                | shared HTTP health handlers             |
| `services/payment-service/`  | gRPC + PostgreSQL + NATS                |
| `services/store-service/`    | MongoDB + NATS worker                   |
| `services/notification-service/` | NATS consumer + slog notifier       |
| `services/analytics-service/`    | MinIO receipts + PG summaries       |
| `scripts/ui/`                | HTMX demo gateway                       |
| `tests/e2e/`                 | сквозные E2E-тесты                      |
| `docs/*-SERVICE.md`          | доменные описания сервисов              |

---

## 12. Соответствие PLAN.md

Все обязательные пункты [PLAN.md § Спринт 5](PLAN.md) (строки 126–147) выполнены:

| Задача PLAN                         | Статус |
| ----------------------------------- | ------ |
| Payment: gRPC Charge/Refund, PG, NATS | ✅   |
| Store: NATS worker, MongoDB         | ✅     |
| Notification: NATS worker, logging  | ✅     |
| Analytics: MinIO + PostgreSQL       | ✅     |
| Order: REST lifecycle, Swagger    | ✅     |
| Order: gRPC Payment client          | ✅     |
| Order: gRPC WatchOrderStatus stream | ✅     |
| Order: NATS consumers, Saga         | ✅     |
| Integration tests (success/fail/cancel) | ✅ |
| Web client REST + realtime statuses | ✅     |
| Health page с состоянием сервисов   | ✅     |

**Нюанс:** «отказ склада» в E2E — сбой **confirm после оплаты** (`TestE2E_StoreFailureAfterPayment`). Отказ на этапе reserve покрыт integration-тестами Store и unit-тестом consumer в Order.

**«Звёздочка»** (proxy-сервис) — не реализована.

---

## 13. Сознательно отложено

- Proxy-сервис с метриками трафика и помехами («звёздочка» спринта 5)
- Keycloak / OIDC
- Отдельные `README.md` у payment/store/notification/analytics (есть `docs/*-SERVICE.md`)
- E2E checkout при insufficient stock на уровне HTTP (покрыто store integration)

---

## 14. Проверка

```bash
task lint
task test:unit
task test:integration
task test:e2e

task certs:generate
cp .env.example .env
task demo:up

# Swagger UI
open https://localhost:8080/swagger/   # curl -k

# REST happy path
TOKEN=$(curl -sk -X POST https://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@example.com","password":"demo123"}' | jq -r .access_token)

curl -sk -X POST https://localhost:8080/cart/items \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"product_id":"22222222-2222-4222-8222-222222222222","quantity":1,"unit_price":1000}'

curl -sk -X POST https://localhost:8080/orders \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"delivery_address":"Moscow"}'

# Demo UI
open http://localhost:8081
```
