# Бизнес-описание домена «Payment Service»

Документ описывает бизнес-сущности, правила и сценарии Payment Service в рамках [архитектуры проекта](DESIGN.md). Фокус — обработка платежей и возвратов, взаимодействие с Order Service по gRPC и публикация событий в NATS.

---

## Бизнес-сущности

### 1. Пользовательский счёт (Account)

Упрощённая модель платёжного счёта пользователя. Хранится в PostgreSQL.

**Атрибуты:**
- Идентификатор пользователя (UUID)
- Текущий баланс (целое число, минимальная единица — копейки/центы)
- Версия (optimistic lock для конкурентного списания)
- Дата создания и обновления

**Правила:**
- Баланс не может быть отрицательным.
- Все изменения баланса производятся только через транзакции (Charge/Refund).
- Версия используется для optimistic lock: перед списанием читаем версию, при записи проверяем, что версия не изменилась (pg `UPDATE ... WHERE version = :old_version`).
- Начальный баланс задаётся при seed-инициализации (несколько демо-пользователей).

---

### 2. Платёжная транзакция (Transaction)

Журнал всех операций списания и возврата. Хранится в PostgreSQL.

**Атрибуты:**
- Идентификатор транзакции (UUID)
- Идентификатор заказа (OrderID)
- Идентификатор пользователя (UserID)
- Тип: `charge` (списание) или `refund` (возврат)
- Сумма (целое число, > 0)
- Статус: `PENDING` → `SUCCEEDED` | `FAILED`
- Идентификатор оригинальной транзакции (для refund — ссылка на charge-транзакцию)
- Причина отказа (если статус `FAILED`)
- Дата создания и обновления

**Правила:**
- Сумма транзакции всегда > 0.
- Для `refund` обязательно указан `original_transaction_id` — ссылка на успешную `charge`-транзакцию.
- Одна `charge`-транзакция может быть возвращена только один раз (идемпотентность).
- Статус `PENDING` — транзакция создана, но ещё не проведена (на случай асинхронных сценариев в будущем).
- Статус `SUCCEEDED` — деньги списаны/возвращены, баланс обновлён.
- Статус `FAILED` — операция не выполнена (недостаточно средств, повторный refund и т.д.).

---

## gRPC API

### `Charge(ChargeRequest) returns (ChargeResponse)`

Синхронный унарный вызов от Order Service.

**ChargeRequest:**
- `order_id` (string) — идентификатор заказа
- `user_id` (string) — идентификатор пользователя
- `amount` (int64) — сумма в минимальных единицах

**ChargeResponse:**
- `transaction_id` (string) — ID созданной транзакции
- `status` (enum: `SUCCEEDED`, `FAILED`)
- `message` (string) — человекочитаемое описание (при ошибке)

**Бизнес-логика:**
1. Проверить, что `amount > 0`.
2. Прочитать баланс пользователя (с версией).
3. Если баланс < amount → статус `FAILED`, причина `INSUFFICIENT_FUNDS`.
4. Создать транзакцию со статусом `PENDING`.
5. В одной pg-транзакции: списать баланс (с проверкой версии), обновить статус транзакции на `SUCCEEDED`.
6. Если optimistic lock не сошёлся — повторить (retry) или вернуть `FAILED` с причиной `CONCURRENT_MODIFICATION`.
7. Опубликовать событие `PAYMENT_SUCCEEDED` / `PAYMENT_FAILED` в NATS.
8. Вернуть результат.

---

### `Refund(RefundRequest) returns (RefundResponse)`

Синхронный унарный вызов от Order Service (компенсация при отказе склада или отмена заказа после оплаты).

**RefundRequest:**
- `order_id` (string) — идентификатор заказа
- `user_id` (string) — идентификатор пользователя
- `amount` (int64) — сумма возврата
- `original_transaction_id` (string) — ID оригинальной charge-транзакции

**RefundResponse:**
- `transaction_id` (string) — ID транзакции возврата
- `status` (enum: `SUCCEEDED`, `FAILED`)
- `message` (string) — описание (при ошибке)

**Бизнес-логика:**
1. Проверить, что `amount > 0`.
2. Найти оригинальную charge-транзакцию по `original_transaction_id`.
3. Если оригинальная транзакция не найдена или её статус не `SUCCEEDED` → `FAILED`, причина `INVALID_ORIGINAL_TRANSACTION`.
4. Если для этой charge-транзакции уже есть успешный refund → `FAILED`, причина `ALREADY_REFUNDED` (идемпотентность).
5. Создать refund-транзакцию со статусом `PENDING`.
6. В одной pg-транзакции: начислить баланс, обновить статус refund-транзакции на `SUCCEEDED`.
7. Опубликовать событие `REFUND_SUCCEEDED` / `REFUND_FAILED` в NATS.
8. Вернуть результат.

---

## События NATS (исходящие)

| Событие             | Когда             | Поля (расширенные)                                                                        |
| :------------------ | :---------------- | :---------------------------------------------------------------------------------------- |
| `PAYMENT_SUCCEEDED` | Успешный Charge   | `order_id`, `user_id`, `amount`, `transaction_id`, `timestamp`                            |
| `PAYMENT_FAILED`    | Неуспешный Charge | `order_id`, `user_id`, `amount`, `reason`, `timestamp`                                    |
| `REFUND_SUCCEEDED`  | Успешный Refund   | `order_id`, `user_id`, `amount`, `transaction_id`, `original_transaction_id`, `timestamp` |
| `REFUND_FAILED`     | Неуспешный Refund | `order_id`, `user_id`, `amount`, `original_transaction_id`, `reason`, `timestamp`         |

---

## Варианты использования

### UC-PAY-1: Успешная оплата заказа

**Предусловия:** Пользователь существует, баланс >= суммы заказа.

**Поток:**
1. Order Service вызывает `Charge(order_id, user_id, amount)`.
2. Payment Service проверяет баланс — достаточно средств.
3. В pg-транзакции: списание баланса + создание транзакции `SUCCEEDED`.
4. Публикация `PAYMENT_SUCCEEDED`.
5. Order Service получает `transaction_id` и статус `SUCCEEDED`.

---

### UC-PAY-2: Отказ оплаты (недостаточно средств)

**Предусловия:** Пользователь существует, баланс < суммы заказа.

**Поток:**
1. Order Service вызывает `Charge(order_id, user_id, amount)`.
2. Payment Service проверяет баланс — недостаточно средств.
3. Создаётся транзакция со статусом `FAILED`, причина `INSUFFICIENT_FUNDS`.
4. Публикация `PAYMENT_FAILED`.
5. Order Service получает статус `FAILED`, заказ остаётся в `RESERVED`.

---

### UC-PAY-3: Возврат средств (Refund)

**Предусловия:** Есть успешная charge-транзакция, refund для неё ещё не проводился.

**Поток:**
1. Order Service вызывает `Refund(order_id, user_id, amount, original_transaction_id)`.
2. Payment Service находит оригинальную транзакцию, проверяет, что refund ещё не был сделан.
3. В pg-транзакции: начисление баланса + создание refund-транзакции `SUCCEEDED`.
4. Публикация `REFUND_SUCCEEDED`.

---

### UC-PAY-4: Идемпотентный отказ повторного Refund

**Предусловия:** Refund для данной charge-транзакции уже был успешно проведён.

**Поток:**
1. Order Service вызывает `Refund` с тем же `original_transaction_id`.
2. Payment Service находит существующий refund для этой транзакции.
3. Возвращается статус `FAILED`, причина `ALREADY_REFUNDED`.
4. Order Service понимает, что это повтор, и не предпринимает дополнительных действий.

---

## Вне scope Payment Service

- Управление заказами и корзиной — Order Service.
- Резервирование товаров — Store Service.
- Уведомления пользователя — Notification Service.
- Чеки и витрины — Analytics Service.
- Детали биллинга, налоги, валюты, международные платежи.