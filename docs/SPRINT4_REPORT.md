# Отчёт спринта 4: Безопасность (JWT, TLS, mTLS, bcrypt)

**Период:** 13.07 – 19.07  
**Сервис:** `order-service`  
**Цель:** защитить REST API, подготовить mTLS для межсервисного взаимодействия.

---

## 1. JWT-аутентификация

Заголовок `X-User-ID` заменён на **Bearer JWT** (HS256, `github.com/golang-jwt/jwt/v5`).

### Схема

1. `POST /auth/login` — email + password → `access_token`
2. Защищённые эндпоинты — заголовок `Authorization: Bearer <token>`
3. Claim `sub` содержит UUID пользователя

### Публичные эндпоинты (без JWT)

- `GET /health`, `GET /ready`, `GET /metrics`
- `POST /auth/login`

### Demo-пользователи (seed в миграции)

| Email             | Password | User ID                              |
| ----------------- | -------- | ------------------------------------ |
| demo@example.com  | demo123  | 11111111-1111-4111-8111-111111111111 |
| admin@example.com | admin123 | 22222222-2222-4222-8222-222222222222 |

### Примеры curl

```bash
# 1. Генерация сертификатов (первый запуск)
task certs:generate
cp .env.example .env
task obs:up

# 2. Login
curl -k -X POST https://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@example.com","password":"demo123"}'

# 3. API с токеном
TOKEN="<access_token from login>"
curl -k https://localhost:8080/cart \
  -H "Authorization: Bearer ${TOKEN}"
```

Mint JWT для load test / CI без login:

```bash
task jwt:mint USER_ID=11111111-1111-4111-8111-111111111111
```

---

## 2. bcrypt

Пароли хранятся как bcrypt-хеш (cost 12) в таблице `users`.  
Пакет: `pkg/auth/password.go`, проверка при login в `AuthService`.

---

## 3. TLS для REST (HTTPS-only)

Сервер слушает **только HTTPS** (`ListenAndServeTLS`). Self-signed сертификаты для dev.

| Env                  | Описание                    |
| -------------------- | --------------------------- |
| `TLS_CERT_FILE`      | server certificate          |
| `TLS_KEY_FILE`       | server private key          |
| `TLS_CLIENT_CA_FILE` | CA для проверки client cert |

Для dev: `curl -k` / `--insecure`.

---

## 4. mTLS

### Иерархия сертификатов

```
deploy/certs/
  ca/ca.crt + ca.key
  nats/server.crt + server.key
  order-service/server.crt + server.key + client.crt + client.key
  payment-service/client.crt + client.key   # заготовка для спринта 5
```

Генерация: `scripts/certs/generate.sh` → `task certs:generate`.  
Артефакты в `.gitignore`; CI генерирует на лету.

### NATS mTLS

NATS запускается с `--tls --verify`. Клиент order-service подключается с client cert:

```
NATS_URL=tls://nats:4222
NATS_TLS_CERT_FILE=/certs/client.crt
NATS_TLS_KEY_FILE=/certs/client.key
NATS_TLS_CA_FILE=/certs/ca.crt
```

### REST mTLS (service-to-service)

- TLS config: `ClientAuth: RequestClientCert`
- Если client cert подписан CA и CN в whitelist (`order-service`, `payment-service`) — JWT не требуется
- Внешние клиенты (curl, frontend) — HTTPS + JWT, client cert не нужен

Пример для будущего payment-service:

```bash
curl -k https://localhost:8080/orders/<id> \
  --cert deploy/certs/payment-service/client.crt \
  --key deploy/certs/payment-service/client.key \
  --cacert deploy/certs/ca/ca.crt
```

---

## 5. CI/CD

В `.github/workflows/ci.yml` перед Docker build:

1. `bash scripts/certs/generate.sh`
2. COPY certs в образ
3. Smoke: `curl -kfsS https://localhost:8080/health` с env `JWT_SECRET`, `TLS_*`

---

## 6. Новые пакеты и файлы

| Путь                        | Назначение                 |
| --------------------------- | -------------------------- |
| `pkg/auth/`                 | JWT issue/validate, bcrypt |
| `pkg/middleware/auth.go`    | JWT + mTLS middleware      |
| `pkg/tlsutil/`              | TLS config helpers         |
| `scripts/certs/generate.sh` | OpenSSL CA + certs         |
| `scripts/jwt/mint.go`       | CLI для mint JWT           |

---

## 7. Сознательно отложено

- Keycloak / OIDC (опциональная «звёздочка»)
- Register endpoint (только seed users)
- TLS для PostgreSQL / Redis
- Отдельный auth-service (login в order-service как BFF)

---

## 8. Проверка

```bash
task certs:generate
task lint
task test:unit
task test:integration   # testcontainers
task obs:up
task obs:demo
```
