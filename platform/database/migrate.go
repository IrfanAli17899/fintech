package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies every pending migration in files, tracking versions in
// table so each module owns its own history. table may be schema-qualified
// ("ledger.schema_migrations"); the schema is created first so the tracking
// table has somewhere to live before the module's own migrations run.
// Schema and table names come from module constants, never from input, so
// interpolating them into DDL is safe.
func Migrate(ctx context.Context, db *sql.DB, dbURL string, files fs.FS, table string) error {
	u, err := url.Parse(dbURL)
	if err != nil {
		return fmt.Errorf("db url: %w", err)
	}
	q := u.Query()

	if schema, name, ok := strings.Cut(table, "."); ok {
		if _, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
			return fmt.Errorf("create schema %s: %w", schema, err)
		}
		q.Set("x-migrations-table", fmt.Sprintf(`"%s"."%s"`, schema, name))
		q.Set("x-migrations-table-quoted", "1")
	} else {
		q.Set("x-migrations-table", table)
	}
	u.RawQuery = q.Encode()

	src, err := iofs.New(files, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, u.String())
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
