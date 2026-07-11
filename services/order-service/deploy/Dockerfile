FROM golang:1.26.4-alpine AS builder

WORKDIR /src

COPY go.work go.work.sum ./
COPY pkg/go.mod ./pkg/
COPY services/order-service/go.mod services/order-service/go.sum ./services/order-service/
RUN go mod download

COPY pkg ./pkg
COPY services/order-service ./services/order-service

RUN CGO_ENABLED=0 go build -C services/order-service -ldflags="-s -w" -o order-service ./cmd/main.go

FROM scratch

EXPOSE 8080

COPY --from=builder /src/services/order-service/order-service /order-service

ENTRYPOINT ["/order-service"]
