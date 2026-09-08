package repository

import (
	"errors"
	"fmt"
	"mydal/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

// Postgres SQLSTATE codes for the integrity violations a client request can
// actually provoke. Anything else stays unclassified and becomes a 500.
const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

// classify turns a constraint violation into the domain sentinel that matches
// it, so the HTTP layer answers 409 or 400 rather than mapping a client
// mistake to 500. Errors that are not constraint violations pass through
// untouched, and the driver's own message is never part of the result - only
// the constraint name, which names the rule the request broke.
func classify(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case uniqueViolation:
		return fmt.Errorf("%w: %s already exists", domain.ErrConflict, pgErr.ConstraintName)
	case foreignKeyViolation:
		return fmt.Errorf("%w: %s references a row that does not exist", domain.ErrInvalidInput, pgErr.ConstraintName)
	default:
		return err
	}
}
