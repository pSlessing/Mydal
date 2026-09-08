package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

// A classified error carries its message; anything else is answered
// generically so SQL and driver text cannot reach a client.
func TestWriteErrorDoesNotLeakUnclassifiedDetail(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	rec := httptest.NewRecorder()
	WriteError(rec, quiet, fmt.Errorf("album 5: %w", domain.ErrNotFound))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "album 5: not found" {
		t.Fatalf("classified message lost: %q", body["error"])
	}

	rec = httptest.NewRecorder()
	WriteError(rec, quiet, errors.New(`pq: relation "tracks" does not exist`))
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
