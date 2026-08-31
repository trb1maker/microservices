package app

import "github.com/trb1maker/microservices/internal/platform/events"

type OrderFinalized = events.OrderFinalized

type Receipt struct {
	OrderID         string `json:"order_id"`
	UserID          string `json:"user_id"`
	TotalAmount     int64  `json:"total_amount"`
	Status          string `json:"status"`
	FinalizedAt     string `json:"finalized_at"`
	DeliveryAddress string `json:"delivery_address"`
}
