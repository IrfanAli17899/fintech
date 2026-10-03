package ledger

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	"github.com/irfanali17899/fintech/pkg/database"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate brings the ledger schema up to date. Safe to call on every start.
func Migrate(ctx context.Context, db *sql.DB, dbURL string) error {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	return database.Migrate(ctx, db, dbURL, sub, "ledger.schema_migrations")
}
