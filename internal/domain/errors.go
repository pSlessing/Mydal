package domain

import "errors"

// Sentinel errors the layers agree on. Repositories and services return these
// (wrapped, for context) and the HTTP layer maps them to status codes, so no
// layer below the handler needs to know about HTTP.
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")
)
