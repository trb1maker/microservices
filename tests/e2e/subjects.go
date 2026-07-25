//go:build e2e

package e2e

import (
	"os"
	"time"

	storetestwire "github.com/trb1maker/microservices/internal/store-service/testwire"
)

const (
	demoUserID                 = "11111111-1111-4111-8111-111111111111"
	demoEmail                  = "demo@example.com"
	demoPassword               = "demo123"
	testProductID              = storetestwire.E2EProductID
	testUnitPrice              = storetestwire.E2EProductPrice
	startupTimeout             = 3 * time.Minute
	pollInterval               = 100 * time.Millisecond
	orderCreatedSubject        = "orders.created"
	orderCancelledSubject      = "orders.cancelled"
	releaseReservationSubject  = "cart.release_reservation"
	reserveItemsSubject        = "cart.reserve_items"
	itemsReservedSubject       = "store.items_reserved"
	reservationFailedSubject   = "store.reservation_failed"
	confirmOrderSubject        = "orders.confirm"
	orderConfirmedSubject      = "store.order_confirmed"
	orderFinalizedSubject      = "orders.finalized"
	paymentSucceededSubject    = "payment.succeeded"
	paymentFailedSubject       = "payment.failed"
	refundSucceededSubject     = "payment.refund_succeeded"
	refundFailedSubject        = "payment.refund_failed"
	reservationReleasedSubject = "store.reservation_released"
	minioBucket                = "receipts"
)

var (
	testJWTSecret  = envOr("JWT_SECRET", "dev-jwt-secret-minimum-32-characters-long")
	minioAccessKey = envOr("MINIO_ACCESS_KEY", "minioadmin")
	minioSecretKey = envOr("MINIO_SECRET_KEY", "minioadmin")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
