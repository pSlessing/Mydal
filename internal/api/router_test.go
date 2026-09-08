package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mydal/internal/api/handlers"
	"mydal/internal/domain"
	"mydal/internal/service"
	"mydal/internal/testutil"
)

// The three handler interfaces below are satisfied trivially: every case here
// exercises routing (an unmatched path or the wrong method on a known one),
// so no handler body ever runs.

type noopArtistService struct{}

func (noopArtistService) GetArtistByID(ctx context.Context, id string) (*domain.Artist, error) {
	return nil, nil
}
func (noopArtistService) CreateArtist(ctx context.Context, a *domain.Artist) error { return nil }
func (noopArtistService) DeleteArtist(ctx context.Context, id string) error        { return nil }

type noopAlbumService struct{}

func (noopAlbumService) GetAlbumByID(ctx context.Context, id string) (*domain.Album, error) {
	return nil, nil
}
func (noopAlbumService) CreateAlbum(ctx context.Context, a *domain.Album) error { return nil }
func (noopAlbumService) DeleteAlbum(ctx context.Context, id string) error       { return nil }

type noopPlaylistService struct{}

func (noopPlaylistService) GetPlaylistByID(ctx context.Context, id string) (*domain.Playlist, error) {
	return nil, nil
}
func (noopPlaylistService) CreatePlaylist(ctx context.Context, p *domain.Playlist) error {
	return nil
}
func (noopPlaylistService) DeletePlaylist(ctx context.Context, id string) error { return nil }
func (noopPlaylistService) AddTrack(ctx context.Context, playlistID, trackID string) error {
	return nil
}
func (noopPlaylistService) RemoveTrack(ctx context.Context, playlistID, trackID string) error {
	return nil
}

// newTestRouter builds a real Router with every dependency a handler needs to
// be constructed, but none it needs to actually run one: nothing here reaches
// a database or a blob store, because no test in this file calls a handler -
// they all stop at the mux.
func newTestRouter(t *testing.T) *Router {
	t.Helper()
	quiet := testutil.Quiet()
	trackService := service.NewTrackService(nil, nil, quiet)
	return NewRouter(
		handlers.NewArtistHandler(noopArtistService{}, quiet),
		handlers.NewTrackHandler(trackService, nil, 1<<20, quiet),
		handlers.NewAlbumHandler(noopAlbumService{}, quiet),
		handlers.NewStreamHandler(trackService, nil, quiet),
		handlers.NewPlaylistHandler(noopPlaylistService{}, quiet),
		handlers.NewHealthHandler(nil, nil, quiet),
		quiet,
	)
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (body: %s)", err, rec.Body.String())
	}
	return body
}

// A path that matches no registered route used to fall through to the stdlib
// mux's own NotFoundHandler, which answers plain text - breaking the JSON
// error contract every routed endpoint keeps.
func TestUnknownPathAnswersJSON404(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	if body := decodeErrorBody(t, rec); body["error"] != "not found" {
		t.Fatalf("error = %q, want %q", body["error"], "not found")
	}
}

// A registered path called with the wrong method used to get the stdlib
// mux's own plain-text 405, still with the Allow header it always set.
func TestWrongMethodOnKnownPathAnswersJSON405(t *testing.T) {
	r := newTestRouter(t)

	for _, tc := range []struct {
		name      string
		method    string
		path      string
		wantAllow []string
	}{
		{
			name:      "artist by id",
			method:    http.MethodPatch,
			path:      "/api/v1/artists/11111111-1111-1111-1111-111111111111",
			wantAllow: []string{"DELETE", "GET"},
		},
		{
			name:      "artists collection",
			method:    http.MethodGet,
			path:      "/api/v1/artists",
			wantAllow: []string{"POST"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405: %s", rec.Code, rec.Body)
			}
			allow := rec.Header().Get("Allow")
			for _, want := range tc.wantAllow {
				if !strings.Contains(allow, want) {
					t.Errorf("Allow = %q, want it to contain %q", allow, want)
				}
			}
			if body := decodeErrorBody(t, rec); body["error"] != "method not allowed" {
				t.Fatalf("error = %q, want %q", body["error"], "method not allowed")
			}
		})
	}
}

// The rewrite must not disturb a response that was never a 405.
func TestNormalResponsePassesThroughUnchanged(t *testing.T) {
	r := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (body: %s)", err, rec.Body.String())
	}
	if body["status"] != "ok" {
		t.Fatalf(`status field = %q, want "ok"`, body["status"])
	}
}
