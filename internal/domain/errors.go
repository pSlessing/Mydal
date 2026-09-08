package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors the layers agree on. Repositories and services return these
// (wrapped, for context) and the HTTP layer maps them to status codes, so no
// layer below the handler needs to know about HTTP.
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")

	// ErrDuplicateAudio is the conflict raised when an upload's content hash
	// already belongs to another track. It wraps ErrConflict, so it still
	// maps to 409 and satisfies errors.Is(err, ErrConflict) everywhere that
	// already checks for a plain conflict; it exists as its own sentinel so
	// the HTTP layer can give a client a code distinct from a generic one,
	// for the one conflict a client is likely to want to handle specially
	// ("this exact file is already in the library").
	ErrDuplicateAudio = fmt.Errorf("duplicate audio: %w", ErrConflict)
)
