# order-service

Центральный сервис платформы: корзина, заказы, оркестрация Saga (Choreography). BFF для фронтенда (REST), взаимодействие с Store и Payment через NATS и gRPC.

Домен и сценарии: [docs/OREDER-SERVICE.md](../../docs/OREDER-SERVICE.md)

## Структура

```
services/order-service/
  cmd/                         — точка входа, wiring
  deploy/Dockerfile            — multistage (scratch)
  internal/
    domain/                    — Cart, Order, OrderItem
    app/                       — use cases, порты репозиториев
    adapters/
      input/http/              — REST handlers
      output/memory/           — in-memory repos (спринт 1)
```

## Требования

- Go 1.26.4
- [Task](https://taskfile.dev/) — команды запускаются из **корня монорепозитория**

## Команды

| Команда                                 | Описание                          |
| --------------------------------------- | --------------------------------- |
| `task run SERVICE=order-service`        | запуск сервиса локально (`:8080`) |
| `task build`                            | сборка в `bin/order-service`      |
| `task test:unit`                        | юнит-тесты                        |
| `task lint`                             | golangci-lint                     |
| `task docker:build`                     | образ `order-service:dev`         |
| `task docker:run SERVICE=order-service` | контейнер на `:8080`              |

## API (заглушки)

Идентификация: заголовок `X-User-ID: <uuid>`.

| Метод  | Путь                      | Описание                 |
| ------ | ------------------------- | ------------------------ |
| GET    | `/health`                 | health check             |
| POST   | `/cart/items`             | добавить товар в корзину |
| GET    | `/cart`                   | получить корзину         |
| DELETE | `/cart/items/{productID}` | удалить позицию          |
| POST   | `/orders`                 | оформить заказ           |
| GET    | `/orders/{id}`            | получить заказ           |
| DELETE | `/orders/{id}`            | отменить заказ           |

### Примеры

```bash
curl localhost:8080/health

curl -X POST localhost:8080/cart/items \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 550e8400-e29b-41d4-a716-446655440000' \
  -d '{"product_id":"660e8400-e29b-41d4-a716-446655440001","quantity":1,"unit_price":100}'

curl -X POST localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -H 'X-User-ID: 550e8400-e29b-41d4-a716-446655440000' \
  -d '{"delivery_address":"Moscow, Red Square 1"}'
```

## Docker

```bash
task docker:build
task docker:run SERVICE=order-service
```

В CI образ публикуется в GHCR: `ghcr.io/<owner>/<repo>/order-service`.
