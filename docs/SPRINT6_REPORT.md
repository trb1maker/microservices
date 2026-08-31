# Отчёт спринта 6: БД, кэш и отказоустойчивость

**Период:** 04.08 – 16.08  
**Цель:** PostgreSQL/MongoDB replication, Redis (корзина + TTL, Rate Limiter, Distributed Mutex), тестирование отказов.

---

## 1. Что реализовано

| Задача                                     | Статус         | Реализация                                                                                           |
| ------------------------------------------ | -------------- | ---------------------------------------------------------------------------------------------------- |
| goose-миграции Order / Payment / Analytics | Готово (ранее) | embedded `goose` на старте сервисов                                                                  |
| PostgreSQL master + replica                | Готово         | `postgres` + `postgres-replica` в [docker-compose.yml](../docker-compose.yml), streaming replication |
| MongoDB 3 узла                             | Готово         | `mongodb`, `mongodb-2`, `mongodb-3` + `mongodb-init` (`rs0`)                                         |
| Redis: корзина + TTL                       | Готово         | `CART_TTL` (default 24h), ключ `cart:{user_id}`                                                      |
| Rate Limiter                               | Готово         | 100 req/min на user (JWT), IP для `/auth/login`, HTTP 429                                            |
| Distributed Mutex                          | Готово         | `lock:stock:{product_id}` в store-service при резерве/confirm/release                                |
| GET /orders                                | Готово (ранее) | пагинация, Swagger, интеграционные тесты                                                             |
| Тест отказов + отчёт                       | Готово         | `scripts/sprint6-failover.sh`, `task sprint6:failover`                                               |

---

## 2. Redis-паттерны

### 2.1 Кэш корзины (TTL)

- Пакет: [internal/order-service/adapters/cart_repository/redis](../internal/order-service/adapters/cart_repository/redis/)
- Конфиг: `CART_TTL=24h`
- Каждый `Save` обновляет TTL (скользящее окно)

### 2.2 Rate Limiter

- Пакет: [internal/platform/middleware/ratelimit.go](../internal/platform/middleware/ratelimit.go)
- Алгоритм: fixed window, ключ `rl:{user\|ip}:{window_start}`
- Конфиг: `RATE_LIMIT_REQUESTS=100`, `RATE_LIMIT_WINDOW=1m`, `RATE_LIMIT_ENABLED=true`
- Метрика: `order_service_rate_limit_exceeded_total`

### 2.3 Distributed Mutex

- Пакет: [internal/platform/redisx](../internal/platform/redisx/) — `SET NX EX` + Lua unlock
- Store: [internal/store-service/adapters/redis/locker.go](../internal/store-service/adapters/redis/locker.go)
- Ключ: `lock:stock:{product_id}`, TTL 5s, retry 20×50ms
- При неудаче lock → `store.reservation_failed` с reason `lock not acquired`

---

## 3. Репликация БД

### 3.1 PostgreSQL (streaming, 2 контейнера)

```
postgres (primary, :5432)  --WAL-->  postgres-replica (standby, :5433)
```

- Primary: `wal_level=replica`, `max_wal_senders=10`, `hot_standby=on`
- Init: [deploy/postgres/init-replication.sql](../deploy/postgres/init-replication.sql) — пользователь `replicator`
- Replica: [deploy/postgres/replica-entrypoint.sh](../deploy/postgres/replica-entrypoint.sh) — `pg_basebackup -R`
- **Приложения пишут только в primary** (`postgres:5432`). Replica — для проверки копии данных и сценария отказа.

### 3.2 MongoDB (replica set rs0, 3 узла)

- URI store-service: `mongodb://mongodb:27017,mongodb-2:27017,mongodb-3:27017/?replicaSet=rs0`
- Init: [deploy/mongodb/init-replica-set.sh](../deploy/mongodb/init-replica-set.sh)
- Go-драйвер автоматически переключается на новый PRIMARY

### 3.3 Почему не Patroni

Учебный стек (`Patroni + etcd + HAProxy`) решает **автопромоут** leader при падении primary. ДЗ спринта 6 просит **репликацию** и сценарий «остановить master и посмотреть реакцию». Streaming replication на двух контейнерах закрывает это без 7 дополнительных сервисов. Patroni остаётся референсом для production auto-failover.

---

## 4. Сценарии отказоустойчивости

Запуск: `task demo:up` → `task sprint6:failover` (или `bash scripts/sprint6-failover.sh`).

| Сценарий             | Действие                        | Ожидаемое поведение                                                                          |
| -------------------- | ------------------------------- | -------------------------------------------------------------------------------------------- |
| Baseline             | `GET /ready`, checkout          | 200, корзина в Redis с TTL > 0                                                               |
| PG primary down      | `docker compose stop postgres`  | order/payment/analytics `/ready` → 503; checkout падает; replica `:5433` читает те же данные |
| PG primary up        | `docker compose start postgres` | сервисы снова ready после reconnect пула                                                     |
| Mongo secondary down | stop одного secondary           | store продолжает резерв (majority + PRIMARY)                                                 |
| Mongo PRIMARY down   | stop текущего primary           | краткий всплеск ошибок; новый PRIMARY; резерв восстанавливается                              |

### Наблюдаемость

- **Логи:** JSON через Promtail → Loki (`service=order-service|store-service`)
- **Метрики:** Prometheus — HTTP errors, `rate_limit_exceeded_total`, postgres-exporter
- **Health:** `/ready` агрегирует postgres, redis, nats (order-service); mongodb, redis, nats (store-service)

---

## 5. Команды

```bash
task demo:up              # поднять стек с replication
task mongo:rs-status    # PRIMARY/SECONDARY
task sprint6:failover   # сценарии отказов

# PostgreSQL replication check
docker compose exec postgres psql -U orders -d orders -c "SELECT pg_is_in_recovery();"
docker compose exec postgres-replica psql -U orders -d orders -c "SELECT pg_is_in_recovery();"

# Redis
docker compose exec redis redis-cli TTL cart:<user-uuid>
```

---

## 6. Тесты

- Unit: `internal/platform/redisx`, `internal/platform/middleware/ratelimit`, store lock
- Integration: cart TTL (`TestIntegration_CartTTL`), store/order существующие сценарии
- HA-сценарии: ручной прогон через `sprint6-failover.sh` (не в CI — хрупко)

---

## 7. Выводы

1. **MongoDB replica set** даёт прозрачный failover записи для store-service без изменения кода — достаточно URI с `replicaSet`.
2. **PostgreSQL streaming replication** без Patroni означает: при падении primary запись недоступна до восстановления или ручного promote; данные на replica сохранены.
3. **Redis mutex** устраняет race при параллельном резерве одного товара; **rate limiter** защищает API от перегрузки на пользователя.
4. TTL корзины ограничивает рост ключей в Redis и задаёт явный срок жизни сессии покупателя.
