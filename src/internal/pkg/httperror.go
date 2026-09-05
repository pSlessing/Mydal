package pkg

import (
	"errors"
	"log/slog"
	"mydal/src/internal/domain"
	"net/http"
)

// StatusForError maps a domain error to an HTTP status code. Anything
// unrecognised is a 500, on the assumption that an error we did not classify
// is a bug rather than a client mistake.
func StatusForError(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// WriteError maps err to a status and writes a JSON error body. Only
// classified errors carry their message to the client; anything else is logged
// in full and answered with a generic message, so internals and SQL text do
// not leak.
func WriteError(w http.ResponseWriter, logger *slog.Logger, err error) {
	status := StatusForError(err)
	if status == http.StatusInternalServerError {
		logger.Error("Unhandled error", "error", err)
		RespondWithError(w, status, "internal server error")
		return
	}
	RespondWithError(w, status, err.Error())
}
