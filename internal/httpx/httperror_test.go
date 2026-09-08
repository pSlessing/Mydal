package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mydal/internal/domain"
)

func TestStatusForError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"not found", domain.ErrNotFound, http.StatusNotFound},
		{"wrapped not found", fmt.Errorf("track x: %w", domain.ErrNotFound), http.StatusNotFound},
		{"invalid input", domain.ErrInvalidInput, http.StatusBadRequest},
		{"conflict", domain.ErrConflict, http.StatusConflict},
		{"unclassified", errors.New("boom"), http.StatusInternalServerError},
	} {
		if got := StatusForError(tc.err); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

// CodeForError is the stable string a client branches on instead of parsing
// the message, which for a classified error can echo a detail (a constraint
// name) that is not itself part of the contract.
func TestCodeForError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"not found", domain.ErrNotFound, "not_found"},
		{"wrapped not found", fmt.Errorf("track x: %w", domain.ErrNotFound), "not_found"},
		{"invalid input", domain.ErrInvalidInput, "invalid_input"},
		{"conflict", domain.ErrConflict, "conflict"},
		{"duplicate audio", domain.ErrDuplicateAudio, "duplicate_audio"},
		{"wrapped duplicate audio", fmt.Errorf("%w: tracks_content_hash_key already exists", domain.ErrDuplicateAudio), "duplicate_audio"},
		{"unclassified", errors.New("boom"), "internal_error"},
	} {
		if got := CodeForError(tc.err); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// ErrDuplicateAudio must still answer 409 like any other conflict - it is a
// specific kind of conflict, not a different status.
func TestDuplicateAudioIsStillAConflict(t *testing.T) {
	if got := StatusForError(domain.ErrDuplicateAudio); got != http.StatusConflict {
		t.Errorf("status = %d, want 409", got)
	}
	if !errors.Is(domain.ErrDuplicateAudio, domain.ErrConflict) {
		t.Error("ErrDuplicateAudio does not satisfy errors.Is(_, ErrConflict)")
	}
}

// A classified error carries its message; anything else is answered
// generically so SQL and driver text cannot reach a client.
func TestWriteErrorDoesNotLeakUnclassifiedDetail(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	WriteError(rec, req, quiet, fmt.Errorf("album 5: %w", domain.ErrNotFound))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "album 5: not found" {
		t.Fatalf("classified message lost: %q", body["error"])
	}

	rec = httptest.NewRecorder()
	WriteError(rec, req, quiet, errors.New(`pq: relation "tracks" does not exist`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "internal server error" {
		t.Fatalf("leaked: %q", body["error"])
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatal("error body is not JSON")
	}
}

// The 500 log line is the one place an unclassified error is logged - a
// repository or service logging it too would either double log it or, for a
// classified error, log a plain 404 as an Error. That line needs the request
// id to be correlatable with the access log line the Logging middleware
// writes for the same request.
func TestWriteErrorLogsTheRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(WithRequestID(req.Context(), "req-123"))

	WriteError(httptest.NewRecorder(), req, logger, errors.New("boom"))

	if !strings.Contains(buf.String(), "req-123") {
		t.Fatalf("log line missing request id: %s", buf.String())
	}
}

// A classified error (4xx) is the client's mistake, not the server's, and is
// not logged at all - only the unclassified 500 path logs.
func TestWriteErrorDoesNotLogAClassifiedError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	WriteError(httptest.NewRecorder(), req, logger, domain.ErrNotFound)

	if buf.Len() != 0 {
		t.Fatalf("a 404 was logged: %s", buf.String())
	}
}
