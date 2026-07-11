package middleware

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/trb1maker/microservices/pkg/auth"

	"github.com/google/uuid"
)

const bearerPartsCount = 2

type contextKey int

const (
	userIDContextKey contextKey = iota
	serviceNameContextKey
)

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return userID, ok
}

func ServiceNameFromContext(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(serviceNameContextKey).(string)
	return name, ok
}

// ContextWithUserID attaches a user ID to the context. Intended for tests.
func ContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// ContextWithServiceName attaches a service identity to the context. Intended for tests.
func ContextWithServiceName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, serviceNameContextKey, name)
}

// AuthSkip определяет, нужно ли пропустить JWT/mTLS-проверку для запроса.
type AuthSkip func(*http.Request) bool

func JWTAuth(
	secret string,
	skip AuthSkip,
	serviceCAs *x509.CertPool,
	serviceCNs map[string]struct{},
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip != nil && skip(r) {
				next.ServeHTTP(w, r)
				return
			}

			if serviceName, ok := serviceIdentity(r, serviceCAs, serviceCNs); ok {
				ctx := context.WithValue(r.Context(), serviceNameContextKey, serviceName)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			token, err := bearerToken(r.Header.Get("Authorization"))
			if err != nil {
				writeUnauthorized(w, err.Error())
				return
			}

			userID, err := auth.ValidateToken(secret, token)
			if err != nil {
				writeUnauthorized(w, "unauthorized")
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func serviceIdentity(r *http.Request, pool *x509.CertPool, allowed map[string]struct{}) (string, bool) {
	if pool == nil || len(allowed) == 0 || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return "", false
	}

	for _, cert := range r.TLS.PeerCertificates {
		if _, err := cert.Verify(x509.VerifyOptions{
			Roots:     pool,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}); err != nil {
			continue
		}

		cn := cert.Subject.CommonName
		if _, ok := allowed[cn]; ok {
			return cn, true
		}
	}

	return "", false
}

func bearerToken(header string) (string, error) {
	if header == "" {
		return "", errMissingAuthorization
	}

	parts := strings.SplitN(header, " ", bearerPartsCount)
	if len(parts) != bearerPartsCount || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", errInvalidAuthorization
	}

	return parts[1], nil
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

var (
	errMissingAuthorization = &authError{"missing authorization header"}
	errInvalidAuthorization = &authError{"invalid authorization header"}
)

type authError struct {
	message string
}

func (e *authError) Error() string {
	return e.message
}
