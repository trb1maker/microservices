package app

import "github.com/trb1maker/microservices/internal/platform/events"

type (
	OrderCreated       = events.OrderCreated
	ReserveItems       = events.ReserveItems
	ConfirmOrder       = events.ConfirmOrder
	ReleaseReservation = events.ReleaseReservation
	OrderFinalized     = events.OrderFinalized
	OrderCancelled     = events.OrderCancelled
	ItemsReserved      = events.ItemsReserved
	ReservationFailed  = events.ReservationFailed
	OrderConfirmed     = events.OrderConfirmed
)
