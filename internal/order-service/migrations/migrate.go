package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/trb1maker/microservices/internal/platform/migrate"
)

//go:embed *.sql
var fs embed.FS

func Up(db *sql.DB) error {
	if err := migrate.Up(db, fs); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
