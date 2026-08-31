//go:build ignore

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/trb1maker/microservices/internal/platform/natsx"
)

func main() {
	count, _ := strconv.Atoi(envOr("PUBLISH_COUNT", "5000"))
	subject := envOr("SUBJECT", "orders.created")
	natsURL := envOr("NATS_URL", "tls://localhost:4222")

	opts := []nats.Option{
		nats.Name("sprint7-load-publisher"),
		nats.Secure(&tls.Config{MinVersion: tls.VersionTLS12}),
	}
	if cert := os.Getenv("NATS_TLS_CERT_FILE"); cert != "" {
		opts = append(opts,
			nats.ClientCert(cert, os.Getenv("NATS_TLS_KEY_FILE")),
			nats.RootCAs(os.Getenv("NATS_TLS_CA_FILE")),
		)
	}

	conn, err := nats.Connect(natsURL, opts...)
	if err != nil {
		log.Fatalf("connect nats: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	client, err := natsx.New(ctx, conn)
	if err != nil {
		log.Fatalf("init jetstream: %v", err)
	}

	start := time.Now()
	for i := 1; i <= count; i++ {
		payload, _ := json.Marshal(map[string]any{
			"order_id":     fmt.Sprintf("load-%06d", i),
			"user_id":      "11111111-1111-4111-8111-111111111111",
			"total_amount": 100,
		})
		if err := client.Publish(ctx, subject, payload); err != nil {
			log.Fatalf("publish %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	fmt.Printf("Go publisher: %d messages in %s (%.0f msg/s)\n", count, elapsed, float64(count)/elapsed.Seconds())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
