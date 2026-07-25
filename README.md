# Микросервисная платформа обработки заказов

Учебный проект курса «Микросервисы на Go» — e-commerce с Saga-хореографией и гибридным REST / gRPC / NATS взаимодействием.

## Архитектура

Монорепозиторий: общая документация и инфраструктура в корне, общие пакеты в `pkg/`, каждый микросервис — отдельный Go-модуль в `services/<name>/`.

```
microservices/
  pkg/              — общая инфраструктура, сгенерированные proto-контракты (logging, health, proto/…)
  api/              — исходные proto-схемы (генерация: task proto:gen → pkg/proto/)
  docs/             — DESIGN, PLAN, ADR, отчёты
  services/
    order-service/  — корзина, заказы, Saga
    ...             — остальные сервисы (по мере реализации)
  go.work           — workspace для локальной разработки
  Taskfile.yml      — общие задачи (lint, test, build)
```

Пять сервисов платформы:

| Сервис                                                         | Назначение                             | Статус     |
| -------------------------------------------------------------- | -------------------------------------- | ---------- |
| [order-service](services/order-service/)                       | Корзина, заказы, BFF (REST/gRPC), Saga | Реализован |
| [store-service](docs/STORE-SERVICE.md)                         | Каталог, остатки, резервирование       | Реализован |
| [payment-service](docs/PAYMENT-SERVICE.md)                     | Платежи (gRPC Charge/Refund)           | Реализован |
| [notification-service](docs/NOTIFICATION-SERVICE.md)           | Уведомления пользователю               | Реализован |
| [analytics-service](docs/ANALYTICS-SERVICE.md)                 | Чеки, витрины, MinIO                   | Реализован |

Подробнее: [docs/DESIGN.md](docs/DESIGN.md)

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

Доменные описания — в `docs/*-SERVICE.md`; подробный API и запуск order-service — в его README.

- **[order-service](services/order-service/README.md)** — REST/gRPC API, Saga, Swagger, health dashboard
- **[payment-service](docs/PAYMENT-SERVICE.md)** — gRPC Charge/Refund, PostgreSQL, NATS
- **[store-service](docs/STORE-SERVICE.md)** — MongoDB, NATS worker резервирования
- **[notification-service](docs/NOTIFICATION-SERVICE.md)** — NATS consumer, slog-уведомления
- **[analytics-service](docs/ANALYTICS-SERVICE.md)** — чеки MinIO, daily summary PostgreSQL

## Локальная разработка

Требуется Go 1.26.4 и [Task](https://taskfile.dev/). Команды выполняются из **корня репозитория**:

| Команда                          | Описание                                  |
| -------------------------------- | ----------------------------------------- |
| `task lint`                      | golangci-lint (все сервисы + UI)          |
| `task test`                      | unit + integration + E2E                  |
| `task test:unit`                 | юнит-тесты                                |
| `task test:integration`          | интеграционные тесты (Docker)             |
| `task test:e2e`                  | сквозные E2E-тесты saga (Testcontainers)   |
| `task certs:generate`            | TLS/mTLS сертификаты (первый запуск)      |
| `task jwt:mint USER_ID=<uuid>`   | Mint JWT без login (load test / CI)       |
| `task demo:up`                   | полный dev-стек (сервисы + UI + observability) |
| `task demo:traffic`              | демо-трафик для метрик и алертов          |
| `task demo:down`                 | остановить compose (volumes сохраняются)  |
| `task demo:reset`                | `docker compose down -v` (destructive)    |
| `task ui:run`                    | локальный UI gateway (нужен order-service) |
| `task proto:gen`                 | генерация Go из `api/**/*.proto` → `pkg/proto/` |
| `task build`                     | сборка бинарников в `bin/` (включая UI)   |
| `task docker:build`              | dev-образы `<service>:dev` + UI           |

Список сервисов для циклических задач задаётся в [`Taskfile.yml`](Taskfile.yml) (`vars.SERVICES`). При добавлении нового микросервиса достаточно дописать имя в этот список.

## Go-модули и workspace

| Модуль       | Путь в `go.mod`                                      |
| ------------ | ---------------------------------------------------- |
| shared `pkg` | `github.com/trb1maker/microservices/pkg`             |
| сервис       | `github.com/trb1maker/microservices/services/<name>` |

[`go.work`](go.work) подключает сервисы, `scripts/ui` и `tests/e2e` через `use`; локальные пути для зависимостей `v0.0.0` задаются versioned `replace` в том же файле. Модуль [`pkg/`](pkg/) не входит в корневой `use` (чтобы IDE и `gopls` не ломали его зависимости); для работы в `pkg/` используется отдельный [`pkg/go.work`](pkg/go.work). Proto-схемы лежат в `api/`, сгенерированный Go — в `pkg/proto/` (`task proto:gen`).

**Новый сервис:**

1. `go mod init github.com/trb1maker/microservices/services/<name>`
2. `go work use ./services/<name>` (из корня)
3. `require github.com/trb1maker/microservices/pkg v0.0.0` в `go.mod` сервиса (без `replace`; локальные пути — в [`go.work`](go.work) через `use` и versioned `replace`)
4. Имя в `Taskfile.yml` → `vars.SERVICES`

Команды (`lint`, `test`, `go mod tidy`, `task test:e2e`) — из **корня репозитория** (активен [`go.work`](go.work)).

## CI/CD

GitHub Actions: [`.github/workflows/ci.yml`](.github/workflows/ci.yml)

| Событие         | Какие сервисы проверяются                                                 |
| --------------- | ------------------------------------------------------------------------- |
| `pull_request`  | Только изменённые (`services/<name>/**`); при правке общих конфигов — все |
| `push` в `main` | Все сервисы с `services/*/go.mod`                                         |

Общие конфиги (триггер «все сервисы» на PR): `.golangci.yaml`, `go.work`, `go.work.sum`, `Taskfile.yml`, `docker-compose.yml`, `api/**`, `pkg/**`, `scripts/ui/**`, `.github/workflows/*`.

На каждый сервис в matrix: lint, test, build, docker (если есть `deploy/Dockerfile`), smoke `/health`. Push образа в GHCR — только на `main`: `ghcr.io/<owner>/<repo>/<service>:<sha>`.

Локальная автоматизация — [`Taskfile.yml`](Taskfile.yml); CI использует явные команды в workflow.

## Наблюдаемость

Полный стек в [`docker-compose.yml`](docker-compose.yml): Loki, Promtail, Prometheus, Alertmanager, Jaeger, Grafana, exporters.

Перед первым запуском:

```bash
task certs:generate
cp .env.example .env
```

| Сервис            | URL                                 |
| ----------------- | ----------------------------------- |
| order-service API | https://localhost:8080 (`curl -k`)  |
| **order UI**      | http://localhost:8081               |
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
task demo:up       # поднять всё (генерирует certs автоматически)
task demo:traffic  # сгенерировать трафик
task demo:down     # остановить (volumes сохраняются)
# task demo:reset  # down -v — удаляет данные БД
```

Конфиги: [`deploy/observability/`](deploy/observability/). Подробности — [SPRINT3_REPORT.md](docs/SPRINT3_REPORT.md).

## Demo UI (HTMX)

Go UI gateway в [`scripts/ui/`](scripts/ui/) — браузерный интерфейс для корзины, checkout, оплаты и отмены заказа. Статусы заказа приходят через **SSE**; gateway транслирует внутренний gRPC `WatchOrderStatus` в браузер (JWT хранится только в HttpOnly session-cookie на сервере).

```bash
task certs:generate
cp .env.example .env   # при необходимости
task demo:up
open http://localhost:8081
```

**Demo-пользователи** (только dev/compose fixtures):

| Email               | Password  | User ID                                |
| ------------------- | --------- | -------------------------------------- |
| demo@example.com    | demo123   | `11111111-1111-4111-8111-111111111111` |
| admin@example.com   | admin123  | `22222222-2222-4222-8222-222222222222` |

**Каталог UI** использует фиксированные UUID-товары из Store seed (`22222222-…`, `33333333-…`, `44444444-…`); legacy `prod-*` не проходят UUID-валидацию Cart API. Счета demo-пользователей создаёт миграция Payment Service.

Локально без compose: поднимите order-service (с gRPC, не `USE_MEMORY=true`) и выполните `task ui:run`.
