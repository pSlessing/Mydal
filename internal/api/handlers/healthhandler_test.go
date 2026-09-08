package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mydal/internal/storage"
	"mydal/internal/testutil"
)

type stubPinger struct{ err error }

func (s stubPinger) PingContext(ctx context.Context) error { return s.err }

// stubBlobs implements just enough of BlobStore for the readiness probe.
type stubBlobs struct{ err error }

func (s stubBlobs) Put(context.Context, string, io.Reader, int64, string) error { return nil }
func (s stubBlobs) Get(context.Context, string) (io.ReadSeekCloser, storage.ObjectInfo, error) {
	return nil, storage.ObjectInfo{}, nil
}
func (s stubBlobs) Stat(context.Context, string) (storage.ObjectInfo, error) {
	return storage.ObjectInfo{}, nil
}
func (s stubBlobs) Delete(context.Context, string) error   { return nil }
func (s stubBlobs) List(context.Context) ([]string, error) { return nil, nil }
func (s stubBlobs) Ping(context.Context) error             { return s.err }
func (s stubBlobs) PresignedGetURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func probe(h *HealthHandler, fn http.HandlerFunc) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	fn(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	return rec
}

// Liveness must not depend on the database: restarting the process does not
// fix an outage, and a probe that says otherwise causes a restart loop.
func TestHealthzIgnoresDependencies(t *testing.T) {
	h := NewHealthHandler(stubPinger{err: errors.New("down")}, stubBlobs{err: errors.New("down")}, testutil.Quiet())
	rec := probe(h, h.Healthz)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200 even with dependencies down", rec.Code)
	}
}

func TestReadyzReportsEachDependency(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dbErr      error
		blobErr    error
		wantStatus int
		wantChecks map[string]string
	}{
		{"all up", nil, nil, http.StatusOK,
			map[string]string{"database": "ok", "blobstore": "ok"}},
		{"database down", errors.New("x"), nil, http.StatusServiceUnavailable,
			map[string]string{"database": "unavailable", "blobstore": "ok"}},
		{"blobstore down", nil, errors.New("x"), http.StatusServiceUnavailable,
			map[string]string{"database": "ok", "blobstore": "unavailable"}},
		{"both down", errors.New("x"), errors.New("x"), http.StatusServiceUnavailable,
			map[string]string{"database": "unavailable", "blobstore": "unavailable"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHealthHandler(stubPinger{err: tc.dbErr}, stubBlobs{err: tc.blobErr}, testutil.Quiet())
			rec := probe(h, h.Readyz)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.wantStatus, rec.Body)
			}
			var body struct {
				Status string            `json:"status"`
				Checks map[string]string `json:"checks"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not JSON: %s", rec.Body)
			}
			for dep, want := range tc.wantChecks {
				if body.Checks[dep] != want {
					t.Errorf("checks[%s] = %q, want %q", dep, body.Checks[dep], want)
				}
			}
		})
	}
}
