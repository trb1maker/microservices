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
COPY services/analytics-service ./services/analytics-service

RUN CGO_ENABLED=0 go build -C services/analytics-service -ldflags="-s -w" -o analytics-service ./cmd/main.go

FROM scratch

EXPOSE 9094

COPY --from=builder /src/services/analytics-service/analytics-service /analytics-service
COPY deploy/certs/analytics-service/client.crt /certs/client.crt
COPY deploy/certs/analytics-service/client.key /certs/client.key
COPY deploy/certs/ca/ca.crt /certs/ca.crt

ENTRYPOINT ["/analytics-service"]
