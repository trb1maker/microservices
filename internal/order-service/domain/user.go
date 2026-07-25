package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type UserID uuid.UUID

type User struct {
	id        UserID
	email     string
	password  string
	createdAt time.Time
}

func NewUser(id UserID, email, passwordHash string, createdAt time.Time) *User {
	return &User{
		id:        id,
		email:     strings.ToLower(strings.TrimSpace(email)),
		password:  passwordHash,
		createdAt: createdAt,
	}
}

func (u *User) ID() UserID {
	return u.id
}

func (u *User) Email() string {
	return u.email
}

func (u *User) PasswordHash() string {
	return u.password
}

func (u *User) CreatedAt() time.Time {
	return u.createdAt
}
