package service

import (
	"fmt"
	"mydal/internal/domain"
	"strings"

	"github.com/google/uuid"
)

// The services validate what the wire cannot: the handlers check the ids in
// the path, but ids that arrive inside a request body reach Postgres unchecked
// otherwise, where a malformed one is an unclassified driver error and so a
// 500 rather than the 400 it is.

func requireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s must not be empty", domain.ErrInvalidInput, field)
	}
	return nil
}

func requireUUID(field, value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("%w: %s must be a uuid, got %q", domain.ErrInvalidInput, field, value)
	}
	return nil
}

// requireInRange rejects a numeric field outside [min, max]. This catches not
// only a client-sent negative value but also one too large for the Postgres
// column behind it: an out-of-range INSERT is SQLSTATE 22003, which classify
// does not recognise, so it would otherwise reach the client as a 500.
func requireInRange(field string, value, min, max int64) error {
	if value < min || value > max {
		return fmt.Errorf("%w: %s must be between %d and %d, got %d", domain.ErrInvalidInput, field, min, max, value)
	}
	return nil
}
