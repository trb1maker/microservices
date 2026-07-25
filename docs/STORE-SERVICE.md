# Бизнес-описание домена «Store Service»

Документ описывает бизнес-сущности, правила и сценарии Store Service в рамках [архитектуры проекта](DESIGN.md). Фокус — управление складскими остатками, резервирование товаров и взаимодействие через NATS.

---

## Бизнес-сущности

### 1. Товар (Product)

Каталожная запись товара. Хранится в MongoDB (коллекция `products`).

**Атрибуты:**
- Идентификатор товара (UUID)
- Название
- Артикул (SKU) — уникальный строковый код
- Цена за единицу (целое число, минимальная единица)
- Описание (опционально)
- Дата создания и обновления

**Правила:**
- Название и артикул обязательны.
- Цена > 0.
- Артикул уникален в пределах коллекции.
- Товар создаётся при seed-инициализации (демо-набор товаров).

---

### 2. Складской остаток (Stock)

Текущее количество товара на складе. Хранится в MongoDB (коллекция `stock`).

**Атрибуты:**
- Идентификатор записи (UUID)
- Идентификатор товара (ProductID) — ссылка на `products`
- `available` (int) — доступное для резерва количество
- `reserved` (int) — зарезервированное количество (ожидает подтверждения)
- Версия (для optimistic lock, опционально)
- Дата создания и обновления

**Правила:**
- `available` ≥ 0, `reserved` ≥ 0.
- Общий остаток = `available` + `reserved`.
- При резервировании: `available` уменьшается, `reserved` увеличивается.
- При подтверждении (CONFIRM_ORDER): `reserved` уменьшается (товар списан со склада).
- При освобождении (RELEASE_RESERVATION): `reserved` уменьшается, `available` увеличивается.
- `available` не может уйти в минус при резервировании.
- `reserved` не может уйти в минус при подтверждении/освобождении.

---

## Входящие команды NATS

Store Service — это NATS-воркер. Он не имеет собственного REST/gRPC API, только слушает команды.

| Команда               | От кого       | Описание                                    |
| :-------------------- | :------------ | :------------------------------------------ |
| `RESERVE_ITEMS`       | Order Service | Зарезервировать товар за пользователем      |
| `CONFIRM_ORDER`       | Order Service | Подтвердить заказ (списать товар со склада) |
| `RELEASE_RESERVATION` | Order Service | Освободить резерв (отмена заказа)           |

---

## Исходящие события NATS

| Событие                | Когда                                  | Поля                                                                   |
| :--------------------- | :------------------------------------- | :--------------------------------------------------------------------- |
| `ITEMS_RESERVED`       | Успешное резервирование                | `order_id`, `user_id`, `product_id`, `quantity`, `timestamp`           |
| `RESERVATION_FAILED`   | Неудачное резервирование (нет остатка) | `order_id`, `user_id`, `product_id`, `quantity`, `reason`, `timestamp` |
| `ORDER_CONFIRMED`      | Успешное подтверждение (списание)      | `order_id`, `user_id`, `timestamp`                                     |
| `RESERVATION_RELEASED` | Успешное освобождение резерва          | `order_id`, `user_id`, `product_id`, `quantity`, `timestamp`           |

---

## Варианты использования

### UC-STORE-1: Резервирование товара (RESERVE_ITEMS)

**Предусловия:** Товар существует, `available >= quantity`.

**Поток:**
1. Order Service публикует `RESERVE_ITEMS { user_id, product_id, quantity }`.
2. Store Service получает команду.
3. Находит запись stock по `product_id`.
4. Проверяет: `stock.available >= quantity`.
5. Атомарно обновляет: `available -= quantity`, `reserved += quantity`.
6. Публикует `ITEMS_RESERVED { order_id, user_id, product_id, quantity }`.

**Ошибки:** товар не найден, недостаточно остатка → `RESERVATION_FAILED`.

---

### UC-STORE-2: Подтверждение заказа (CONFIRM_ORDER)

**Предусловия:** Товар зарезервирован (есть запись в stock с `reserved >= quantity`).

**Поток:**
1. Order Service публикует `CONFIRM_ORDER { order_id, user_id }`.
2. Store Service получает команду.
3. Находит все stock-записи для товаров заказа (по `order_id` из контекста).
4. Для каждого товара: `reserved -= quantity` (списание со склада).
5. Публикует `ORDER_CONFIRMED { order_id, user_id }`.

**Ошибки:** Если `reserved < quantity` (неконсистентное состояние) → `RESERVATION_FAILED`.

---

### UC-STORE-3: Освобождение резерва (RELEASE_RESERVATION)

**Предусловия:** Товар зарезервирован.

**Поток:**
1. Order Service публикует `RELEASE_RESERVATION { user_id, order_id }`.
2. Store Service получает команду.
3. Для каждого товара заказа: `reserved -= quantity`, `available += quantity`.
4. Публикует `RESERVATION_RELEASED { order_id, user_id, product_id, quantity }`.

---

### UC-STORE-4: Отказ склада (компенсация)

**Предусловия:** Заказ в статусе `PAID`, отправлено `CONFIRM_ORDER`.

**Поток:**
1. Store Service получает `CONFIRM_ORDER`.
2. Не может выполнить списание (например, `reserved` меньше ожидаемого).
3. Публикует `RESERVATION_FAILED { order_id, user_id, reason }`.
4. Order Service получает `RESERVATION_FAILED` и запускает компенсацию (Refund).

---

## Вне scope Store Service

- Управление заказами и корзиной — Order Service.
- Платёжные операции — Payment Service.
- Уведомления — Notification Service.
- Чеки и аналитика — Analytics Service.
- Категории товаров, теги, поиск, рекомендации.
- Ценообразование, скидки, промокоды.