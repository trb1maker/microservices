FROM golang:1.26.4-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o order-service ./cmd/main.go

FROM scratch

EXPOSE 8080

COPY --from=builder /app/order-service /order-service

ENTRYPOINT ["/order-service"]
