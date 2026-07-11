# order-service

Центральный сервис платформы: корзина, заказы, оркестрация Saga (Choreography). BFF для фронтенда (REST), взаимодействие с Store и Payment через NATS и gRPC.

Домен и сценарии: [docs/OREDER-SERVICE.md](../../docs/OREDER-SERVICE.md)

## Структура

```
services/order-service/
  cmd/                         — точка входа, wiring
  deploy/Dockerfile            — multistage (scratch)
  migrations/                  — goose SQL-миграции
  internal/
    config/                    — caarlos0/env
    domain/                    — Cart, Order, OrderItem
    app/                       — use cases, порты
    adapters/
      http/                      — REST handlers
      cart_repository/
        memory/                  — USE_MEMORY=true
        redis/
      order_repository/
        memory/                  — USE_MEMORY=true
        postgres/
      event_publisher/
        nats/
```

Общие пакеты: [`pkg/logging`](../../pkg/logging), [`pkg/health`](../../pkg/health).

## Требования

- Go 1.26.4
- Docker (инфраструктура и integration-тесты)
- [Task](https://taskfile.dev/) — команды из **корня монорепозитория**

## Конфигурация

Шаблон: [`.env.example`](.env.example)

| Переменная | Описание | По умолчанию |
|------------|----------|--------------|
| `HTTP_ADDR` | Адрес HTTP-сервера | `:8080` |
| `DATABASE_URL` | PostgreSQL DSN | — |
| `REDIS_ADDR` | Redis | `localhost:6379` |
| `NATS_URL` | NATS | `nats://localhost:4222` |
| `USE_MEMORY` | In-memory repos (без Docker) | `false` |
| `LOG_LEVEL` | slog level | `info` |
| `LOG_FORMAT` | `json` или `text` | `json` |
| `ORDER_CREATED_SUBJECT` | NATS subject для `PublishOrderCreated` | `orders.created` |
| `RESERVE_ITEMS_SUBJECT` | NATS subject для `PublishReserveItems` | `cart.reserve_items` |
| `CONFIRM_ORDER_SUBJECT` | NATS subject для `PublishConfirmOrder` | `orders.confirm` |
| `RELEASE_RESERVATION_SUBJECT` | NATS subject для `PublishReleaseReservation` | `cart.release_reservation` |
| `ORDER_FINALIZED_SUBJECT` | NATS subject для `PublishOrderFinalized` | `orders.finalized` |
| `ORDER_CANCELLED_SUBJECT` | NATS subject для `PublishOrderCancelled` | `orders.cancelled` |

## Команды

| Команда | Описание |
|---------|----------|
| `task infra:up` | PostgreSQL, Redis, NATS |
| `task run SERVICE=order-service` | запуск локально |
| `task test:unit` | юнит-тесты |
| `task test:integration` | testcontainers |
| `task lint` | golangci-lint |
| `task build` | `bin/order-service` |
| `task docker:build` | образ `order-service:dev` |

## API

Идентификация: заголовок `X-User-ID: <uuid>`.

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/health` | liveness (без проверки deps) |
| GET | `/ready` | readiness (PG, Redis, NATS) |
| POST | `/cart/items` | добавить товар |
| GET | `/cart` | корзина |
| DELETE | `/cart/items/{productID}` | удалить позицию |
| POST | `/orders` | оформить заказ + `ORDER_CREATED` |
| GET | `/orders/{id}` | заказ |
| DELETE | `/orders/{id}` | отменить |

### Пример (с инфраструктурой)

```bash
task infra:up
export $(grep -v '^#' services/order-service/.env.example | xargs)
task run SERVICE=order-service

curl localhost:8080/ready
```

## Docker

```bash
task docker:build
docker run --rm -e USE_MEMORY=true -p 8080:8080 order-service:dev
```

В CI smoke используется `USE_MEMORY=true`. В production-like окружении задайте `DATABASE_URL`, `REDIS_ADDR`, `NATS_URL`.

Образ в GHCR: `ghcr.io/<owner>/<repo>/order-service`.
