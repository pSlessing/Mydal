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

// contentHashConstraint is the unique index that makes a second copy of
// already-stored audio a conflict. Its violation gets its own sentinel
// (ErrDuplicateAudio) instead of the generic one, so a client can tell "this
// exact file is already in the library" apart from any other conflict.
const contentHashConstraint = "tracks_content_hash_key"

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
		if pgErr.ConstraintName == contentHashConstraint {
			return fmt.Errorf("%w: %s already exists", domain.ErrDuplicateAudio, pgErr.ConstraintName)
		}
		return fmt.Errorf("%w: %s already exists", domain.ErrConflict, pgErr.ConstraintName)
	case foreignKeyViolation:
		return fmt.Errorf("%w: %s references a row that does not exist", domain.ErrInvalidInput, pgErr.ConstraintName)
	default:
		return err
	}
}
