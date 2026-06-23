# Отчёт: Спринт 1 — CI/CD и первый релиз order-service

**Период:** 22.06 – 28.06  
**Цель:** пайплайны, линтеры, тесты, контейнеризация `order-service`.

---

## Выполненные задачи

| Задача                                                | Статус |
| ----------------------------------------------------- | ------ |
| REST-эндпоинты (заглушки с валидацией)                | Готово |
| Юнит-тесты бизнес-логики                              | Готово |
| golangci-lint + CI                                    | Готово |
| Dockerfile (multistage, scratch)                      | Готово |
| GitHub Actions (lint, test, build, docker, push GHCR) | Готово |
| Контейнер локально и в CI (smoke `/health`)           | Готово |

---

## Архитектура order-service

```
internal/
  domain/                 — Cart, Order, OrderItem
  app/                    — use cases + порты репозиториев
  adapters/
    input/http/           — REST handlers
    output/memory/        — in-memory repos (спринт 1)
```

Эндпоинты: `GET /health`, `POST /cart/items`, `GET /cart`, `DELETE /cart/items/{productID}`, `POST /orders`, `GET /orders/{id}`, `DELETE /orders/{id}`.

Идентификация: заголовок `X-User-ID`.

---

## Локальная проверка

```bash
task lint
task test:unit
task build
task docker:build    # образ order-service:dev
task docker:run      # порт 8080
curl localhost:8080/health
```

Пример оформления заказа:

```bash
USER_ID=550e8400-e29b-41d4-a716-446655440000
PRODUCT_ID=660e8400-e29b-41d4-a716-446655440001

curl -X POST localhost:8080/cart/items \
  -H "Content-Type: application/json" \
  -H "X-User-ID: $USER_ID" \
  -d "{\"product_id\":\"$PRODUCT_ID\",\"quantity\":1,\"unit_price\":100}"

curl -X POST localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "X-User-ID: $USER_ID" \
  -d '{"delivery_address":"Moscow, Red Square 1"}'
```

Ошибка валидации (пустая корзина):

```bash
curl -X POST localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "X-User-ID: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{"delivery_address":"Moscow"}'
# HTTP 400, {"error":"cart is empty"}
```

---

## CI/CD

Workflow: [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)

| Job       | Описание                                                                   |
| --------- | -------------------------------------------------------------------------- |
| `changes` | Определяет затронутые сервисы (гибрид: PR — diff, `main` — все)            |
| `ci`      | Matrix по сервисам: lint, test, build, docker + smoke, push GHCR на `main` |

Образы в GHCR: `ghcr.io/<owner>/<repo>/<service>:<sha>` и `:latest` (по одному на сервис).

Локально — `task`; в CI — явные `go` / `golangci-lint` / `docker` команды (без Taskfile).

---

## Артефакты для скриншотов

> После push в `main` добавьте скриншоты в этот раздел.

1. **GitHub Actions** — зелёный pipeline (jobs: `changes`, `ci` matrix).
2. **Контейнер** — `docker ps` + ответ `curl localhost:8080/health`.
3. **GHCR** — страница пакета `order-service` в GitHub Packages.

---

## Что дальше (Спринт 2)

- Интеграционные тесты с testcontainers (PostgreSQL, Redis, NATS).
- Замена `adapters/output/memory` на Postgres/Redis.
- State machine заказа и публикация событий NATS.
