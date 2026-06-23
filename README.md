# Микросервисная платформа обработки заказов

Учебный проект курса «Микросервисы на Go» — e-commerce с Saga-хореографией и гибридным REST / gRPC / NATS взаимодействием.

## Архитектура

Монорепозиторий: общая документация и инфраструктура в корне, каждый микросервис — отдельный Go-модуль в `services/<name>/`.

```
microservices/
  api/              — proto, OpenAPI/Swagger
  docs/             — DESIGN, PLAN, ADR, отчёты
  services/
    order-service/  — корзина, заказы, Saga
    ...             — остальные сервисы (по мере реализации)
  go.work           — workspace для локальной разработки
  Taskfile.yml      — общие задачи (lint, test, build)
```

Пять сервисов платформы:

| Сервис                                   | Назначение                             | Статус       |
| ---------------------------------------- | -------------------------------------- | ------------ |
| [order-service](services/order-service/) | Корзина, заказы, BFF (REST/gRPC), Saga | В разработке |
| inventory-service                        | Каталог, остатки, резервирование       | Планируется  |
| payment-service                          | Платежи (gRPC Charge/Refund)           | Планируется  |
| notification-service                     | Уведомления пользователю               | Планируется  |
| analytics-service                        | Чеки, витрины, MinIO                   | Планируется  |

Подробнее: [docs/DESIGN.md](docs/DESIGN.md)

## Документация

| Документ                                    | Описание                    |
| ------------------------------------------- | --------------------------- |
| [DESIGN.md](docs/DESIGN.md)                 | Архитектура, ADR, диаграммы |
| [PLAN.md](docs/PLAN.md)                     | План-график спринтов        |
| [OREDER-SERVICE.md](docs/OREDER-SERVICE.md) | Бизнес-домен Order Service  |
| [SPRINT1_REPORT.md](docs/SPRINT1_REPORT.md) | Отчёт спринта 1 (CI/CD)     |

## Микросервисы

Каждый сервис содержит свой `README.md` с API, структурой пакетов и инструкциями по запуску.

- **[order-service](services/order-service/README.md)** — REST-заглушки, домен, CI/CD

## Локальная разработка

Требуется Go 1.26.4 и [Task](https://taskfile.dev/). Команды выполняются из **корня репозитория**:

| Команда                          | Описание                                  |
| -------------------------------- | ----------------------------------------- |
| `task lint`                      | golangci-lint (все сервисы из `SERVICES`) |
| `task test:unit`                 | юнит-тесты                                |
| `task build`                     | сборка бинарников в `bin/`                |
| `task run SERVICE=<name>`        | запуск одного сервиса                     |
| `task docker:build`              | dev-образы `<service>:dev`                |
| `task docker:run SERVICE=<name>` | запуск контейнера                         |

Список сервисов для циклических задач задаётся в [`Taskfile.yml`](Taskfile.yml) (`vars.SERVICES`). При добавлении нового микросервиса достаточно дописать имя в этот список.

## CI/CD

GitHub Actions: [`.github/workflows/ci.yml`](.github/workflows/ci.yml)

| Событие         | Какие сервисы проверяются                                                 |
| --------------- | ------------------------------------------------------------------------- |
| `pull_request`  | Только изменённые (`services/<name>/**`); при правке общих конфигов — все |
| `push` в `main` | Все сервисы с `services/*/go.mod`                                         |

Общие конфиги (триггер «все сервисы» на PR): `.golangci.yaml`, `go.work`, `go.work.sum`, `Taskfile.yml`, `.github/workflows/*`.

На каждый сервис в matrix: lint, test, build, docker (если есть `deploy/Dockerfile`), smoke `/health`. Push образа в GHCR — только на `main`: `ghcr.io/<owner>/<repo>/<service>:<sha>`.

Локальная автоматизация — [`Taskfile.yml`](Taskfile.yml); CI использует явные команды в workflow.
