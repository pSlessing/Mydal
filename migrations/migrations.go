// Package migrations embeds the SQL schema migrations and applies them.
//
// The .sql files ship inside the binary, so a deployed Mydal carries the
// schema it expects and no separate migration image or manual psql step is
// needed.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var FS embed.FS

// newMigrator builds a migrator over the embedded files and an already-open
// database, so callers reuse the pool and DSN main.go has already validated.
//
// This is migrate's generic "postgres" driver, which talks plain database/sql
// and so works over the pgx stdlib driver the server opens. migrate's own
// pgx/v5 driver is not used: it offers only WithInstance, whose Close
// unconditionally closes the caller's *sql.DB.
//
// It borrows a single *sql.Conn rather than using postgres.WithInstance:
// WithInstance keeps a reference to the *sql.DB and closing the migrator would
// then close the caller's pool out from under the running server.
func newMigrator(ctx context.Context, db *sql.DB) (*migrate.Migrate, error) {
	source, err := iofs.New(FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire migration connection: %w", err)
	}
	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return m, nil
}

// Up applies every pending migration. It is a no-op when the schema is
// already current, so it is safe to call on every startup.
func Up(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	m, err := newMigrator(ctx, db)
	if err != nil {
		return err
	}
	// Returns the borrowed connection to the pool; the *sql.DB stays open.
	defer m.Close()

	switch err := m.Up(); {
	case errors.Is(err, migrate.ErrNoChange):
		logger.Info("Schema is up to date")
	case err != nil:
		return fmt.Errorf("apply migrations: %w", err)
	default:
		logger.Info("Applied migrations")
	}

	version, dirty, err := m.Version()
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	logger.Info("Schema version", "version", version, "dirty", dirty)
	return nil
}

// Down rolls every migration back. It exists for tests and local resets; the
// server never calls it.
func Down(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	m, err := newMigrator(ctx, db)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("roll back migrations: %w", err)
	}
	logger.Info("Rolled back migrations")
	return nil
}
