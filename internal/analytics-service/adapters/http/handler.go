package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/trb1maker/microservices/internal/analytics-service/app"
	pkgmiddleware "github.com/trb1maker/microservices/internal/platform/middleware"
)

type AnalyticsService interface {
	GetReceiptURL(ctx context.Context, userID, orderID string) (string, time.Duration, error)
	SearchReceipts(ctx context.Context, userID, query string, limit int) ([]app.SearchResult, error)
}

type ReadinessChecker interface {
	Check(ctx context.Context) (ready bool, checks map[string]string)
}

const maxSearchLimit = 100

type Handler struct {
	analytics AnalyticsService
	readiness ReadinessChecker
}

func NewHandler(analytics AnalyticsService, readiness ReadinessChecker) *Handler {
	return &Handler{analytics: analytics, readiness: readiness}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.readiness == nil {
		writeJSON(w, http.StatusOK, readyResponse{Status: "ready", Checks: map[string]string{}})
		return
	}

	ready, checks := h.readiness.Check(r.Context())
	if !ready {
		writeJSON(w, http.StatusServiceUnavailable, readyResponse{Status: "not_ready", Checks: checks})
		return
	}
	writeJSON(w, http.StatusOK, readyResponse{Status: "ready", Checks: checks})
}

func (h *Handler) GetReceipt(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	orderID := r.PathValue("order_id")
	if _, err := uuid.Parse(orderID); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid order_id"})
		return
	}

	url, ttl, err := h.analytics.GetReceiptURL(r.Context(), userID.String(), orderID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, receiptURLResponse{
		URL:       url,
		ExpiresIn: int64(ttl.Seconds()),
	})
}

func (h *Handler) SearchReceipts(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	query := r.URL.Query().Get("q")
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			limit = value
		}
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	results, err := h.analytics.SearchReceipts(r.Context(), userID.String(), query, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	if results == nil {
		results = []app.SearchResult{}
	}
	writeJSON(w, http.StatusOK, searchResponse{Results: results})
}

func userIDFromRequest(r *http.Request) (uuid.UUID, error) {
	userID, ok := pkgmiddleware.UserIDFromContext(r.Context())
	if !ok {
		return uuid.UUID{}, app.ErrUnauthorized
	}
	return userID, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status, message := mapError(err)
	if status == http.StatusInternalServerError {
		slog.Error("unhandled request error", slog.Any("error", err))
	}
	writeJSON(w, status, errorResponse{Error: message})
}

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, app.ErrUnauthorized):
		return http.StatusUnauthorized, err.Error()
	case errors.Is(err, app.ErrReceiptNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, app.ErrReceiptForbidden):
		return http.StatusNotFound, app.ErrReceiptNotFound.Error()
	case errors.Is(err, app.ErrSearchQueryRequired):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

type healthResponse struct {
	Status string `json:"status"`
}

type readyResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type receiptURLResponse struct {
	URL       string `json:"url"`
	ExpiresIn int64  `json:"expires_in"`
}

type searchResponse struct {
	Results []app.SearchResult `json:"results"`
}
