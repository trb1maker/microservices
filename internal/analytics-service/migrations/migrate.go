package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/trb1maker/microservices/internal/platform/migrate"
)

//go:embed *.sql
var embedMigrations embed.FS

func Up(db *sql.DB) error {
	if err := migrate.Up(db, embedMigrations); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
