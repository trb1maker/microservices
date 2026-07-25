# Бизнес-описание домена «Notification Service»

Документ описывает бизнес-сущности, правила и сценарии Notification Service в рамках [архитектуры проекта](DESIGN.md). Фокус — асинхронная отправка уведомлений пользователям через NATS-воркер.

---

## Описание

Notification Service — самый простой сервис в системе. Он не имеет собственного API, не использует базу данных. Его единственная задача — слушать финальные события из NATS и "отправлять" уведомления пользователю.

В рамках учебного проекта реальная отправка email/SMS не реализуется. Вместо этого сервис пишет структурированное уведомление в stdout (JSON), которое собирается Loki для просмотра в Grafana.

---

## Входящие события NATS

| Событие             | От кого         | Описание                                   |
| :------------------ | :-------------- | :----------------------------------------- |
| `ORDER_FINALIZED`   | Order Service   | Заказ успешно завершён                     |
| `ORDER_CANCELLED`   | Order Service   | Заказ отменён (пользователем или системой) |
| `PAYMENT_SUCCEEDED` | Payment Service | Платёж успешно проведён                    |
| `REFUND_SUCCEEDED`  | Payment Service | Возврат средств успешно проведён           |

---

## Шаблоны уведомлений

При получении события сервис формирует "уведомление" и выводит его в лог.

| Событие             | Текст уведомления (пример)                                                                 |
| :------------------ | :----------------------------------------------------------------------------------------- |
| `ORDER_FINALIZED`   | `[NOTIFICATION] Order {order_id} confirmed. Thank you for your purchase!`                  |
| `ORDER_CANCELLED`   | `[NOTIFICATION] Order {order_id} has been cancelled.`                                      |
| `PAYMENT_SUCCEEDED` | `[NOTIFICATION] Payment {transaction_id} for order {order_id} succeeded. Amount: {amount}` |
| `REFUND_SUCCEEDED`  | `[NOTIFICATION] Refund {transaction_id} for order {order_id} processed. Amount: {amount}`  |

---

## Формат лога

Каждое уведомление выводится как JSON-строка через `slog`:

```json
{
  "time": "2026-07-18T12:00:00Z",
  "level": "INFO",
  "msg": "[NOTIFICATION] Order 123 confirmed. Thank you for your purchase!",
  "service": "notification-service",
  "event_type": "ORDER_FINALIZED",
  "order_id": "123",
  "user_id": "456"
}
```

---

## Варианты использования

### UC-NOTIF-1: Уведомление об успешном заказе

1. Order Service публикует `ORDER_FINALIZED`.
2. Notification Service получает событие.
3. Формирует текст: `[NOTIFICATION] Order {order_id} confirmed. Thank you for your purchase!`
4. Выводит в лог через `slog.Info`.

---

### UC-NOTIF-2: Уведомление об отмене заказа

1. Order Service публикует `ORDER_CANCELLED`.
2. Notification Service получает событие.
3. Формирует текст: `[NOTIFICATION] Order {order_id} has been cancelled.`
4. Выводит в лог.

---

### UC-NOTIF-3: Уведомление об успешном платеже

1. Payment Service публикует `PAYMENT_SUCCEEDED`.
2. Notification Service получает событие.
3. Формирует текст: `[NOTIFICATION] Payment {transaction_id} for order {order_id} succeeded.`
4. Выводит в лог.

---

### UC-NOTIF-4: Уведомление о возврате средств

1. Payment Service публикует `REFUND_SUCCEEDED`.
2. Notification Service получает событие.
3. Формирует текст: `[NOTIFICATION] Refund {transaction_id} for order {order_id} processed.`
4. Выводит в лог.

---

## Вне scope Notification Service

- Управление заказами — Order Service.
- Платежи — Payment Service.
- Склад — Store Service.
- Аналитика — Analytics Service.
- Реальная отправка email/SMS (только симуляция через лог).
- Шаблонизация HTML-писем.
- Очереди недоставленных уведомлений.