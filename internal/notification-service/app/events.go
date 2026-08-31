package app

import "github.com/trb1maker/microservices/internal/platform/events"

type (
	OrderFinalized   = events.OrderFinalized
	OrderCancelled   = events.OrderCancelled
	PaymentSucceeded = events.PaymentSucceeded
	RefundSucceeded  = events.RefundSucceeded
)
