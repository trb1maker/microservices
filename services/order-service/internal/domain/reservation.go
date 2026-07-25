package domain

type ReservationStatus string

const (
	ReservationStatusPending  ReservationStatus = "PENDING"
	ReservationStatusReserved ReservationStatus = "RESERVED"
	ReservationStatusFailed   ReservationStatus = "FAILED"
)
