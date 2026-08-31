package natsx

import (
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const defaultMaxAge = 7 * 24 * time.Hour

var defaultStreams = []jetstream.StreamConfig{
	{
		Name:      "ORDERS",
		Subjects:  []string{"orders.>"},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    defaultMaxAge,
	},
	{
		Name:      "CART",
		Subjects:  []string{"cart.>"},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    defaultMaxAge,
	},
	{
		Name:      "STORE",
		Subjects:  []string{"store.>"},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    defaultMaxAge,
	},
	{
		Name:      "PAYMENT",
		Subjects:  []string{"payment.>"},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		MaxAge:    defaultMaxAge,
	},
}

// StreamForSubject returns the JetStream stream name for a NATS subject.
func StreamForSubject(subject string) string {
	switch {
	case strings.HasPrefix(subject, "orders."):
		return "ORDERS"
	case strings.HasPrefix(subject, "cart."):
		return "CART"
	case strings.HasPrefix(subject, "store."):
		return "STORE"
	case strings.HasPrefix(subject, "payment."):
		return "PAYMENT"
	default:
		return ""
	}
}
