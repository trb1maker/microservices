package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const cookieName = "ui_session"

var (
	errInvalidSession = errors.New("invalid session")
	errExpiredSession = errors.New("expired session")
)

type Data struct {
	AccessToken string `json:"access_token"`
	UserEmail   string `json:"user_email"`
	UserID      string `json:"user_id"`
	ExpiresAt   int64  `json:"expires_at"`
}

type Store struct {
	secret []byte
	maxAge time.Duration
}

func NewStore(secret string, maxAge time.Duration) *Store {
	return &Store{secret: []byte(secret), maxAge: maxAge}
}

func (s *Store) Save(w http.ResponseWriter, data Data) error {
	data.ExpiresAt = time.Now().Add(s.maxAge).Unix()
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(s.secret, encoded)
	value := encoded + "." + sig
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.maxAge.Seconds()),
	})
	return nil
}

func (s *Store) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func (s *Store) Load(r *http.Request) (Data, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return Data{}, errInvalidSession
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return Data{}, errInvalidSession
	}
	if !hmac.Equal([]byte(sign(s.secret, parts[0])), []byte(parts[1])) {
		return Data{}, errInvalidSession
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Data{}, errInvalidSession
	}
	var data Data
	if err := json.Unmarshal(raw, &data); err != nil {
		return Data{}, errInvalidSession
	}
	if time.Now().Unix() > data.ExpiresAt {
		return Data{}, errExpiredSession
	}
	if data.AccessToken == "" || data.UserID == "" {
		return Data{}, errInvalidSession
	}
	return data, nil
}

func sign(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
