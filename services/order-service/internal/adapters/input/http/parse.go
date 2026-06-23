package http

import (
	"net/http"
	"order-service/internal/app"
	"order-service/internal/domain"

	"github.com/google/uuid"
)

func parseUserID(r *http.Request) (domain.UserID, error) {
	raw := r.Header.Get("X-User-ID")
	if raw == "" {
		return domain.UserID{}, app.ErrInvalidUserID
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return domain.UserID{}, app.ErrInvalidUserID
	}

	return domain.UserID(id), nil
}

func parseProductID(raw string) (domain.ProductID, error) {
	if raw == "" {
		return domain.ProductID{}, app.ErrInvalidProductID
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return domain.ProductID{}, app.ErrInvalidProductID
	}

	return domain.ProductID(id), nil
}

func parseOrderID(raw string) (domain.OrderID, error) {
	if raw == "" {
		return domain.OrderID{}, app.ErrInvalidOrderID
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return domain.OrderID{}, app.ErrInvalidOrderID
	}

	return domain.OrderID(id), nil
}

func uuidToString[T ~[16]byte](id T) string {
	return uuid.UUID(id).String()
}
