# Отчёт: Спринт 2 — тестирование и реальные зависимости

**Период:** 29.06 – 05.07  
**Цель:** PostgreSQL/Redis/NATS для order-service, интеграционные и нагрузочные тесты.

---

## Выполненные задачи

| Задача                                                          | Статус |
| --------------------------------------------------------------- | ------ |
| Переименование inventory → store-service в документации         | Готово |
| docker-compose (PostgreSQL, Redis, NATS) + `task infra:up/down` | Готово |
| Адаптеры postgres / redis / nats, EventPublisher                | Готово |
| Миграции goose (`orders`, `order_items`)                        | Готово |
| Конфигурация caarlos0/env + `.env.example`                      | Готово |
| Логирование slog (JSON), без пакета `log`                       | Готово |
| Пробы `/health` (liveness) + `/ready` (readiness)               | Готово |
| Интеграционные тесты (testcontainers)                           | Готово |
| Integration job в GitHub Actions                                | Готово |
| Нагрузочный тест (vegeta, `POST /orders`)                       | Готово |

---

## Архитектура order-service (спринт 2)

```
internal/
  config/                 — caarlos0/env
  domain/                 — Cart, Order, OrderItem
  app/                    — use cases + ports
  adapters/
    http/                 — REST handlers
    cart_repository/{memory,redis}/
    order_repository/{memory,postgres}/
    event_publisher/nats/ — ORDER_CREATED, ORDER_CANCELLED, …
migrations/               — goose SQL + migrate.go
```

Событие при checkout: `orders.created` (JSON: `order_id`, `user_id`, `total_price`).

---

## Локальная проверка

```bash
task infra:up
# скопировать services/order-service/.env.example → .env или экспортировать переменные
task run SERVICE=order-service

curl localhost:8080/health
curl localhost:8080/ready

task test:unit
task test:integration
```

---

## CI/CD

Workflow: [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)

- Unit-тесты: `go test -race`
- Integration: `go test -tags=integration` (testcontainers, Docker на runner)
- Docker smoke: `USE_MEMORY=true` + `GET /health`

---

## Нагрузочное тестирование

**Дата:** 24.06.2026  
**Сервис:** order-service (PostgreSQL + Redis + NATS)  
**Инструмент:** [vegeta](https://github.com/tsenart/vegeta)

### Методика

1. Поднята инфраструктура: `task infra:up` (PostgreSQL, Redis, NATS).
2. Запущен `order-service` с реальными адаптерами (`USE_MEMORY=false`).
3. Проверка готовности: `GET /ready` → `200`.
4. Подготовка данных: для каждого виртуального пользователя — `POST /cart/items` (1 позиция).
5. Атака: `POST /orders` с уникальным `X-User-ID` на каждый запрос (vegeta targets).
6. Скрипт: [`scripts/load/orders.sh`](../scripts/load/orders.sh).

Параметры прогона:

| Параметр              | Значение |
| --------------------- | -------- |
| Rate                  | 20 RPS   |
| Duration              | 15 s     |
| Подготовленных корзин | 350      |
| Всего запросов        | 300      |

### Результаты

| Метрика      | Значение                  |
| ------------ | ------------------------- |
| Throughput   | **20.06 RPS**             |
| Success rate | **100%** (300 × HTTP 201) |
| Latency p50  | **6.6 ms**                |
| Latency p95  | **9.7 ms**                |
| Latency p99  | **15.4 ms**               |
| Latency max  | **67.6 ms**               |

Артефакты: `docs/load_results/orders_20250624T151645Z.{json,html}` (локально, в git не коммитятся).

### Выводы

1. При rate 20 RPS сервис стабильно обрабатывает checkout с записью в PostgreSQL, очисткой корзины в Redis и публикацией `ORDER_CREATED` в NATS.
2. p95 < 10 ms — приемлемый baseline для одного инстанса на локальной Docker-инфраструктуре; основная задержка — сетевой стек и I/O БД/брокера.
3. Для повторного прогона на той же корзине пользователя заказ вернёт `400 cart is empty` — при нагрузочном тесте нужно готовить не меньше корзин, чем планируется запросов (`rate × duration`).

### Повторный запуск

```bash
task infra:up
# в отдельном терминале — экспортировать переменные из services/order-service/.env.example
task run SERVICE=order-service

# после GET /ready == 200
go install github.com/tsenart/vegeta@latest
PREP_COUNT=350 RATE=20 DURATION=15s ./scripts/load/orders.sh
```

---

## Что дальше (Спринт 3)

- Loki, Prometheus, Jaeger
- Метрики HTTP и `orders_created_total`
- Дашборды Grafana
