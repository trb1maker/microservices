package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/trb1maker/microservices/services/order-service/internal/app"
)

var errUnauthorized = app.ErrUnauthorized

type AuthService interface {
	Login(r *http.Request, email, password string) (accessToken string, expiresIn int64, err error)
}

type loginHandler struct {
	auth AuthService
}

func (h *loginHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: msgInvalidRequestBody})
		return
	}

	token, expiresIn, err := h.auth.Login(r, req.Email, req.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
	})
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: err.Error()})
	case errors.Is(err, app.ErrAuthUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: app.ErrAuthUnavailable.Error()})
	default:
		writeError(w, err)
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type appAuthAdapter struct {
	service *app.AuthService
}

func NewAppAuthAdapter(service *app.AuthService) AuthService {
	return &appAuthAdapter{service: service}
}

func (a *appAuthAdapter) Login(r *http.Request, email, password string) (string, int64, error) {
	token, ttl, err := a.service.Login(r.Context(), email, password)
	if err != nil {
		return "", 0, fmt.Errorf("login: %w", err)
	}

	return token, int64(ttl.Seconds()), nil
}
