package main

import (
	"fmt"
	"os"
	"time"

	"github.com/trb1maker/microservices/pkg/auth"

	"github.com/google/uuid"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mint-jwt <user-uuid>")
		os.Exit(1)
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		fmt.Fprintln(os.Stderr, "JWT_SECRET is required")
		os.Exit(1)
	}

	userID, err := uuid.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid user uuid")
		os.Exit(1)
	}

	token, err := auth.IssueToken(secret, userID, 24*time.Hour)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(token)
}
