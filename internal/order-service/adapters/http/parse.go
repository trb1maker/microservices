package http

import (
	"net/http"
	"strconv"

	"github.com/trb1maker/microservices/internal/order-service/app"
	"github.com/trb1maker/microservices/internal/order-service/domain"

	"github.com/google/uuid"
)

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

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			limit = value
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			offset = value
		}
	}
	return limit, offset
}
