// go:build integration

package e2e

import (
	"time"

	storetestwire "github.com/trb1maker/microservices/services/store-service/testwire"
)

const (
	demoUserID                 = "11111111-1111-4111-8111-111111111111"
	demoEmail                  = "demo@example.com"
	demoPassword               = "demo123"
	testJWTSecret              = "integration-test-secret-minimum-32-characters"
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
	minioAccessKey             = "minioadmin"
	minioSecretKey             = "minioadmin"
	minioBucket                = "receipts"
)
