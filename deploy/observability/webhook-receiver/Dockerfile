FROM golang:1.26.4-alpine AS builder

WORKDIR /src
COPY deploy/observability/webhook-receiver/main.go ./main.go
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o webhook-receiver ./main.go

FROM scratch
COPY --from=builder /src/webhook-receiver /webhook-receiver
EXPOSE 5001
ENTRYPOINT ["/webhook-receiver"]
