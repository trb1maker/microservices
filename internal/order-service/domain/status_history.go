package domain

import "time"

type StatusHistoryEntry struct {
	Status    OrderStatus
	Reason    string
	CreatedAt time.Time
}
