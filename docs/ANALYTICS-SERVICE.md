# Бизнес-описание домена «Analytics Service»

Документ описывает бизнес-сущности, правила и сценарии Analytics Service в рамках [архитектуры проекта](DESIGN.md). Фокус — сбор аналитических данных: сохранение чеков в MinIO и агрегированных данных в PostgreSQL.

---

## Описание

Analytics Service — фоновый NATS-воркер. Он слушает финальные события заказов и выполняет две задачи:

1. **Сохранение чека** в MinIO (S3-совместимое хранилище) в формате JSON.
2. **Обновление агрегированной витрины** в PostgreSQL (ежедневные сводки).

Сервис не имеет собственного API. Данные из MinIO и PostgreSQL используются для дашбордов в Grafana.

---

## Входящие события NATS

| Событие           | От кого       | Описание               |
| :---------------- | :------------ | :--------------------- |
| `ORDER_FINALIZED` | Order Service | Заказ успешно завершён |

---

## Бизнес-сущности

### 1. Чек заказа (Receipt)

JSON-документ, сохраняемый в MinIO. Бакет: `receipts`.

**Структура (базовые поля):**

```json
{
  "order_id": "uuid",
  "user_id": "uuid",
  "total_amount": 1000,
  "status": "CONFIRMED",
  "finalized_at": "2026-07-18T12:00:00Z"
}
```

**Правила:**
- Каждый чек сохраняется в отдельный объект MinIO.
- Ключ объекта: `receipts/{order_id}.json`.
- Если чек для данного `order_id` уже существует — операция игнорируется (идемпотентность).

---

### 2. Ежедневная сводка (Daily Summary)

Агрегированные данные по дням. Хранится в PostgreSQL (таблица `daily_summary`).

**Атрибуты:**
- `date` (DATE) — дата сводки
- `total_orders` (int) — количество завершённых заказов за день
- `total_revenue` (int64) — суммарная выручка за день
- `avg_order_value` (float) — средний чек (total_revenue / total_orders)

**Правила:**
- Одна строка на дату.
- При получении `ORDER_FINALIZED`:
  - Если строка за сегодняшнюю дату существует — обновляем: `total_orders += 1`, `total_revenue += amount`, пересчитываем `avg_order_value`.
  - Если строки нет — создаём новую.
- Операция идемпотентна (используем `ON CONFLICT DO UPDATE`).

---

## Варианты использования

### UC-ANALYTICS-1: Сохранение чека в MinIO

1. Order Service публикует `ORDER_FINALIZED { order_id, user_id, total_amount }`.
2. Analytics Service получает событие.
3. Формирует JSON-чек.
4. Сохраняет в MinIO: `PUT receipts/{order_id}.json`.
5. Логирует результат.

**Ошибки:** MinIO недоступен — повтор через NATS (JetStream redelivery).

---

### UC-ANALYTICS-2: Обновление ежедневной сводки

1. Analytics Service получает `ORDER_FINALIZED`.
2. Выполняет SQL:
   ```sql
   INSERT INTO daily_summary (date, total_orders, total_revenue, avg_order_value)
   VALUES (CURRENT_DATE, 1, :amount, :amount)
   ON CONFLICT (date) DO UPDATE SET
     total_orders = daily_summary.total_orders + 1,
     total_revenue = daily_summary.total_revenue + :amount,
     avg_order_value = (daily_summary.total_revenue + :amount)::float / (daily_summary.total_orders + 1);
   ```
3. Логирует результат.

**Ошибки:** PostgreSQL недоступен — повтор через NATS (JetStream redelivery).

---

## Миграция PostgreSQL

```sql
CREATE TABLE IF NOT EXISTS daily_summary (
    date DATE PRIMARY KEY,
    total_orders INT NOT NULL DEFAULT 0,
    total_revenue BIGINT NOT NULL DEFAULT 0,
    avg_order_value DOUBLE PRECISION NOT NULL DEFAULT 0
);
```

---

## MinIO инициализация

Бакет `receipts` создаётся автоматически через init-контейнер в docker-compose (см. `deploy/minio/init-bucket.sh`).

---

## Вне scope Analytics Service

- Управление заказами — Order Service.
- Платежи — Payment Service.
- Склад — Store Service.
- Уведомления — Notification Service.
- Поиск по чекам (только хранение).
- Дашборды Grafana — настройка в `deploy/observability/grafana/dashboards/`.
- Долгосрочное хранение и архивация данных.