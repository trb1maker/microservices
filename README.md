# Микросервисная платформа обработки заказов

Учебный проект курса «Микросервисы на Go» — e-commerce с Saga-хореографией и гибридным REST / gRPC / NATS взаимодействием.

## Архитектура

Монорепозиторий на **одном Go-модуле** `github.com/trb1maker/microservices`:

```
microservices/
  go.mod / go.sum
  api/                    — исходные proto-схемы (генерация: task gen:proto)
  cmd/<service>/          — точки входа (main.go)
  internal/
    platform/             — общая инфраструктура (logging, health, proto, OTel, …)
    <service>/            — код сервиса (app, adapters, config, domain, migrations)
  tests/
    integration/<service>/  — один сервис + Testcontainers
    e2e/                  — saga всех сервисов (testwire)
    ui/                   — demo UI gateway (HTMX)
  deploy/docker/<service>     — Dockerfile для сборки образов
  Taskfile.yml            — lint, test, build, demo
  golangci-lint.mod       — pinned golangci-lint (go tool -modfile)
```

Пять сервисов платформы:

| Сервис                                                         | Назначение                             | Статус     |
| -------------------------------------------------------------- | -------------------------------------- | ---------- |
| [order-service](docs/ORDER-SERVICE.md)                         | Корзина, заказы, BFF (REST/gRPC), Saga | Реализован |
| [store-service](docs/STORE-SERVICE.md)                         | Каталог, остатки, резервирование       | Реализован |
| [payment-service](docs/PAYMENT-SERVICE.md)                     | Платежи (gRPC Charge/Refund)           | Реализован |
| [notification-service](docs/NOTIFICATION-SERVICE.md)           | Уведомления пользователю               | Реализован |
| [analytics-service](docs/ANALYTICS-SERVICE.md)                 | Чеки, витрины, MinIO                   | Реализован |

Подробнее: [docs/DESIGN.md](docs/DESIGN.md)

## Реструктуризация репозитория

Репозиторий переведён на **один Go-модуль** в корне (`go.mod`), без `go.work`, каталогов `services/` и `pkg/`.

| Было | Стало |
| ---- | ----- |
| `services/<name>/` + отдельные `go.mod` | `cmd/<name>/` + `internal/<name>/` |
| `pkg/` | `internal/platform/` |
| integration-тесты внутри сервисов | `tests/integration/<service>/` |
| E2E saga в сервисах | `tests/e2e/` |
| UI в compose | UI локально: `task demo:ui` |
| копия `.env` | [`.env.example`](.env.example) напрямую (Task `dotenv`, compose) |

**Документация** — только в корне и `docs/`; в `internal/<service>/` остаётся код (миграции, шаблоны, сгенерированный Swagger).

**CI локально:** `task ci` = lint + unit/integration/e2e + docker smoke.

## Документация

| Документ                                    | Описание                                       |
| ------------------------------------------- | ---------------------------------------------- |
| [DESIGN.md](docs/DESIGN.md)                 | Архитектура, ADR, диаграммы                    |
| [PLAN.md](docs/PLAN.md)                     | План-график спринтов                           |
| [ORDER-SERVICE.md](docs/ORDER-SERVICE.md)   | Бизнес-домен Order Service                     |
| [SPRINT1_REPORT.md](docs/SPRINT1_REPORT.md) | Отчёт спринта 1 (CI/CD)                        |
| [SPRINT2_REPORT.md](docs/SPRINT2_REPORT.md) | Отчёт спринта 2 (deps, integration, load test) |
| [SPRINT3_REPORT.md](docs/SPRINT3_REPORT.md) | Отчёт спринта 3 (observability)                |
| [SPRINT4_REPORT.md](docs/SPRINT4_REPORT.md) | Отчёт спринта 4 (JWT, TLS, mTLS)               |
| [SPRINT5_REPORT.md](docs/SPRINT5_REPORT.md) | Отчёт спринта 5 (сервисы, Saga, UI, E2E)       |
| [PAYMENT-SERVICE.md](docs/PAYMENT-SERVICE.md) | Бизнес-домен Payment Service               |
| [STORE-SERVICE.md](docs/STORE-SERVICE.md)     | Бизнес-домен Store Service                 |
| [NOTIFICATION-SERVICE.md](docs/NOTIFICATION-SERVICE.md) | Бизнес-домен Notification Service |
| [ANALYTICS-SERVICE.md](docs/ANALYTICS-SERVICE.md) | Бизнес-домен Analytics Service           |

## Микросервисы

Доменные описания — в `docs/*-SERVICE.md`.

- **[order-service](docs/ORDER-SERVICE.md)** — REST/gRPC API, Saga, Swagger, health dashboard
- **[payment-service](docs/PAYMENT-SERVICE.md)** — gRPC Charge/Refund, PostgreSQL, NATS
- **[store-service](docs/STORE-SERVICE.md)** — MongoDB, NATS worker резервирования
- **[notification-service](docs/NOTIFICATION-SERVICE.md)** — NATS consumer, slog-уведомления
- **[analytics-service](docs/ANALYTICS-SERVICE.md)** — чеки MinIO, daily summary PostgreSQL

## Локальная разработка

Требуется Go 1.27.0 и [Task](https://taskfile.dev/). Команды выполняются из **корня репозитория**:

| Команда                          | Описание                                  |
| -------------------------------- | ----------------------------------------- |
| `task lint`                      | golangci-lint (check-only)                |
| `task lint:fix`                  | golangci-lint с автоисправлением          |
| `task ci`                        | полный CI pipeline (lint + test + docker) |
| `task test:unit`                 | юнит-тесты (`cmd/`, `internal/`, `tests/ui/`) |
| `task test:integration`          | один сервис + Testcontainers              |
| `task test:e2e`                  | saga всех сервисов (testwire)             |
| `task generate:certs`            | TLS/mTLS сертификаты (первый запуск)      |
| `task jwt:mint USER_ID=<uuid>`   | Mint JWT без login (load test / CI)       |
| `task build:apps`                | сборка бинарников сервисов в `bin/`       |
| `task build:images`              | dev-образы сервисов `<service>:dev`       |
| `task gen:proto`                 | генерация Go из `api/**/*.proto` → `internal/platform/proto/` |
| `task gen:swagger`               | Swagger для order-service                 |
| `task demo:up`                   | поднять сервисы + observability (без UI)  |
| `task demo:traffic`              | демо-трафик для метрик и алертов          |
| `task demo:ui`                   | demo UI локально (после `task demo:up`)   |
| `task demo:down`                 | остановить compose и удалить volumes      |

Список сервисов для циклических задач задаётся в [`Taskfile.yml`](Taskfile.yml) (`vars.SERVICES`).

## Go-модуль

Единый модуль `github.com/trb1maker/microservices` в корне. Импорты:

| Код | Import path |
| --- | ----------- |
| platform | `github.com/trb1maker/microservices/internal/platform/...` |
| сервис | `github.com/trb1maker/microservices/internal/<service>/...` |
| main | `./cmd/<service>` |

**Новый сервис:**

1. `cmd/<name>/main.go` — composition root
2. `internal/<name>/` — app, adapters, config, domain
3. `deploy/docker/<name>`
4. Имя в `Taskfile.yml` → `vars.SERVICES`

**Lint:** версия golangci-lint зафиксирована в [`golangci-lint.mod`](golangci-lint.mod); запуск: `go tool -modfile=golangci-lint.mod golangci-lint run ...`.

## CI/CD

GitHub Actions: [`.github/workflows/ci.yml`](.github/workflows/ci.yml)

На каждый `push` / `pull_request` в `main` — один job: `task ci` (lint + test + docker smoke).

Локально тот же pipeline:

```bash
task ci
```

Push образов в GHCR — только на `main`: `ghcr.io/<owner>/<repo>/<service>:<sha>` и `:latest`.

## Наблюдаемость

Полный стек в [`docker-compose.yml`](docker-compose.yml): Loki, Promtail, Prometheus, Alertmanager, Jaeger, Grafana, exporters.

Перед первым запуском:

```bash
task generate:certs
```

Конфигурация — [`.env.example`](.env.example) (используется напрямую для compose, UI и тестов через Task).

| Сервис            | URL                                 |
| ----------------- | ----------------------------------- |
| order-service API | https://localhost:8080 (`curl -k`)  |
| **order UI**      | http://localhost:8081 (`task demo:ui` после `task demo:up`) |
| Prometheus        | http://localhost:9090               |
| Grafana           | http://localhost:3000 (admin/admin) |
| Jaeger UI         | http://localhost:16686              |
| Alertmanager      | http://localhost:9093               |
| Loki              | http://localhost:3100               |

Аутентификация order-service API:

```bash
# Login
curl -k -X POST https://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@example.com","password":"demo123"}'

# API с токеном
curl -k https://localhost:8080/cart -H "Authorization: Bearer <access_token>"
```

```bash
task demo:up       # поднять сервисы и observability
task demo:ui       # в отдельном терминале — локальный UI
open http://localhost:8081
task demo:traffic  # сгенерировать трафик
task demo:down     # остановить и удалить volumes
```

Конфиги: [`deploy/observability/`](deploy/observability/). Подробности — [SPRINT3_REPORT.md](docs/SPRINT3_REPORT.md).

## Demo UI (HTMX)

Go UI gateway в [`tests/ui/`](tests/ui/) — локальный тестовый интерфейс (не входит в docker-compose). Запускается после поднятия сервисов через `task demo:ui`.

```bash
task generate:certs
task demo:up
task demo:ui           # отдельный терминал
open http://localhost:8081
```

**Demo-пользователи** (только dev/compose fixtures):

| Email               | Password  | User ID                                |
| ------------------- | --------- | -------------------------------------- |
| demo@example.com    | demo123   | `11111111-1111-4111-8111-111111111111` |
| admin@example.com   | admin123  | `22222222-2222-4222-8222-222222222222` |

**Каталог UI** использует фиксированные UUID-товары из Store seed (`22222222-…`, `33333333-…`, `44444444-…`); legacy `prod-*` не проходят UUID-валидацию Cart API. Счета demo-пользователей создаёт миграция Payment Service.

Локально без compose: поднимите order-service (с gRPC, не `USE_MEMORY=true`) и выполните `task demo:ui`.
