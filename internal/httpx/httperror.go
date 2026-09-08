package httpx

import (
	"errors"
	"log/slog"
	"mydal/internal/domain"
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

// CodeForError maps an error to a stable string a client can branch on
// without parsing the message, which for a classified error can echo detail
// (a constraint name) that is not itself part of the contract. Checked before
// the plainer domain.ErrConflict it wraps, ErrDuplicateAudio is the one
// conflict most clients want to tell apart from any other.
func CodeForError(err error) string {
	switch {
	case errors.Is(err, domain.ErrDuplicateAudio):
		return "duplicate_audio"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	case errors.Is(err, domain.ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, domain.ErrConflict):
		return "conflict"
	default:
		return "internal_error"
	}
}

// WriteError maps err to a status and code and writes a JSON error body. Only
// classified errors carry their message to the client; anything else is logged
// in full, with the request id so the line can be correlated with the access
// log, and answered with a generic message so internals and SQL text do not
// leak. This is the only place an error from a classified failure downward is
// logged - a repository or service that also logged it would either double
// log a 500 or, worse, log a plain 404 as an error.
func WriteError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status := StatusForError(err)
	code := CodeForError(err)
	if status == http.StatusInternalServerError {
		logger.Error("Unhandled error", "request_id", RequestIDFromContext(r.Context()), "error", err)
		RespondWithError(w, status, code, "internal server error")
		return
	}
	RespondWithError(w, status, code, err.Error())
}
