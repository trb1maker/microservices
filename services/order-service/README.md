# order-service

Центральный сервис платформы: корзина, заказы, оркестрация Saga (Choreography). BFF для фронтенда (REST), взаимодействие с Store и Payment через NATS и gRPC.

Домен и сценарии: [docs/OREDER-SERVICE.md](../../docs/OREDER-SERVICE.md)

## Структура

```
services/order-service/
  cmd/                         — точка входа, wiring
  deploy/Dockerfile            — multistage (scratch) + TLS certs
  migrations/                  — goose SQL-миграции (orders, users)
  internal/
    config/                    — caarlos0/env
    domain/                    — Cart, Order, User
    app/                       — use cases, AuthService
    adapters/
      http/                      — REST handlers, JWT middleware
      user_repository/postgres/
      cart_repository/
        memory/                  — USE_MEMORY=true
        redis/
      order_repository/
        memory/                  — USE_MEMORY=true
        postgres/
      event_publisher/
        nats/                    — TLS + client cert
```

Общие пакеты: [`pkg/auth`](../../pkg/auth), [`pkg/middleware`](../../pkg/middleware), [`pkg/tlsutil`](../../pkg/tlsutil).

## Требования

- Go 1.26.4
- Docker (инфраструктура и integration-тесты)
- OpenSSL (генерация dev-сертификатов)
- [Task](https://taskfile.dev/) — команды из **корня монорепозитория**

## Конфигурация

Шаблон: [`.env.example`](.env.example)

| Переменная           | Описание                      | По умолчанию           |
| -------------------- | ----------------------------- | ---------------------- |
| `HTTP_ADDR`          | Адрес HTTPS-сервера           | `:8080`                |
| `JWT_SECRET`         | Секрет HS256 (min 32 символа) | —                      |
| `JWT_TTL`            | TTL токена                    | `24h`                  |
| `TLS_CERT_FILE`      | Server certificate            | —                      |
| `TLS_KEY_FILE`       | Server private key            | —                      |
| `TLS_CLIENT_CA_FILE` | CA для mTLS client cert       | —                      |
| `NATS_URL`           | NATS URL                      | `tls://localhost:4222` |
| `NATS_TLS_*`         | Client cert для NATS          | —                      |
| `DATABASE_URL`       | PostgreSQL DSN                | —                      |
| `REDIS_ADDR`         | Redis                         | `localhost:6379`       |
| `USE_MEMORY`         | In-memory repos (без Docker)  | `false`                |

Demo users (seed): `demo@example.com` / `demo123`, `admin@example.com` / `admin123`.

## Команды

| Команда                          | Описание                      |
| -------------------------------- | ----------------------------- |
| `task certs:generate`            | TLS/mTLS сертификаты (dev)    |
| `task infra:up`                  | PostgreSQL, Redis, NATS       |
| `task obs:up`                    | Полный стек (certs + compose) |
| `task run SERVICE=order-service` | запуск локально               |
| `task jwt:mint USER_ID=<uuid>`   | выпуск JWT для тестов         |
| `task test:unit`                 | юнит-тесты                    |
| `task test:integration`          | testcontainers                |
| `task lint`                      | golangci-lint                 |
| `task build`                     | `bin/order-service`           |

## API

**HTTPS-only.** Идентификация: `Authorization: Bearer <jwt>` (получить через `/auth/login`).

| Метод  | Путь                      | Auth       | Описание        |
| ------ | ------------------------- | ---------- | --------------- |
| GET    | `/health`                 | —          | liveness        |
| GET    | `/ready`                  | —          | readiness       |
| POST   | `/auth/login`             | —          | login → JWT     |
| POST   | `/cart/items`             | JWT        | добавить товар  |
| GET    | `/cart`                   | JWT        | корзина         |
| DELETE | `/cart/items/{productID}` | JWT        | удалить позицию |
| POST   | `/orders`                 | JWT        | оформить заказ  |
| GET    | `/orders/{id}`            | JWT / mTLS | заказ           |
| DELETE | `/orders/{id}`            | JWT        | отменить        |

### Пример

```bash
task certs:generate
export $(grep -v '^#' services/order-service/.env.example | xargs)
task infra:up
task run SERVICE=order-service

curl -k https://localhost:8080/ready
curl -k -X POST https://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@example.com","password":"demo123"}'
```

## Docker

```bash
task certs:generate
task docker:build
task docker:run SERVICE=order-service
curl -k https://localhost:8080/health
```

В CI smoke: certs генерируются на лету, `USE_MEMORY=true`, HTTPS health check.

Образ в GHCR: `ghcr.io/<owner>/<repo>/order-service`.

Подробности: [docs/SPRINT4_REPORT.md](../../docs/SPRINT4_REPORT.md).
