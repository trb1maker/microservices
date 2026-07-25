FROM golang:1.26.4-alpine AS builder

WORKDIR /src

COPY go.work go.work.sum ./
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY scripts/ui/go.mod scripts/ui/go.sum ./scripts/ui/
COPY services/analytics-service/go.mod services/analytics-service/go.sum ./services/analytics-service/
COPY services/notification-service/go.mod services/notification-service/go.sum ./services/notification-service/
COPY services/order-service/go.mod services/order-service/go.sum ./services/order-service/
COPY services/payment-service/go.mod services/payment-service/go.sum ./services/payment-service/
COPY services/store-service/go.mod services/store-service/go.sum ./services/store-service/
COPY tests/e2e/go.mod tests/e2e/go.sum ./tests/e2e/
RUN go mod download

COPY pkg ./pkg
COPY services/store-service ./services/store-service

RUN CGO_ENABLED=0 go build -C services/store-service -ldflags="-s -w" -o store-service ./cmd/main.go

FROM scratch

EXPOSE 9092

COPY --from=builder /src/services/store-service/store-service /store-service
COPY deploy/certs/store-service/client.crt /certs/client.crt
COPY deploy/certs/store-service/client.key /certs/client.key
COPY deploy/certs/ca/ca.crt /certs/ca.crt

ENTRYPOINT ["/store-service"]