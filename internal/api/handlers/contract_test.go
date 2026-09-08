package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/testutil"
)

func call(fn http.HandlerFunc, method, body string, pathValues map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

func assertJSONError(t *testing.T, name string, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("%s: status = %d, want %d (%s)", name, rec.Code, want, rec.Body)
		return
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("%s: Content-Type = %q, want application/json", name, ct)
		return
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Errorf("%s: body is not JSON: %s", name, rec.Body)
		return
	}
	if body["error"] == "" {
		t.Errorf("%s: no error message: %s", name, rec.Body)
	}
}

const badID = "not-a-uuid"

var goodID = "11111111-1111-1111-1111-111111111111"

// Three of the four resources used to answer plain text, and each mapped
// errors differently: DeleteTrack made every failure a 500, GetAlbum and
// GetPlaylist made every failure a 404.
func TestErrorsAlwaysCarryJSON(t *testing.T) {
	quiet := testutil.Quiet()
	notFound := domain.ErrNotFound
	id := map[string]string{"id": goodID}
	pair := map[string]string{"id": goodID, "trackId": goodID}

	trackH := NewTrackHandler(newTestTrackService(&fakeTrackRepo{getErr: notFound}), nil, 1<<20, quiet)
	albumH := NewAlbumHandler(&fakeAlbumService{err: notFound}, quiet)
	playlistH := NewPlaylistHandler(&fakePlaylistService{err: notFound}, quiet)
	artistH := NewArtistHandler(&fakeArtistService{err: notFound}, quiet)

	for _, tc := range []struct {
		name string
		rec  *httptest.ResponseRecorder
		want int
	}{
		{"GET track missing", call(trackH.GetTrack, "GET", "", id), http.StatusNotFound},
		{"DELETE track missing", call(trackH.DeleteTrack, "DELETE", "", id), http.StatusNotFound},
		{"GET album missing", call(albumH.GetAlbum, "GET", "", id), http.StatusNotFound},
		{"DELETE album missing", call(albumH.DeleteAlbum, "DELETE", "", id), http.StatusNotFound},
		{"GET playlist missing", call(playlistH.GetPlaylist, "GET", "", id), http.StatusNotFound},
		{"DELETE playlist missing", call(playlistH.DeletePlaylist, "DELETE", "", id), http.StatusNotFound},
		{"PUT playlist track missing", call(playlistH.AddTrackToPlaylist, "PUT", "", pair), http.StatusNotFound},
		{"DEL playlist track missing", call(playlistH.RemoveTrackFromPlaylist, "DELETE", "", pair), http.StatusNotFound},
		{"GET artist missing", call(artistH.GetArtist, "GET", "", id), http.StatusNotFound},

		{"POST track malformed", call(trackH.CreateTrack, "POST", "{", nil), http.StatusBadRequest},
		{"POST album malformed", call(albumH.CreateAlbum, "POST", "{", nil), http.StatusBadRequest},
		{"POST playlist malformed", call(playlistH.CreatePlaylist, "POST", "{", nil), http.StatusBadRequest},
		{"POST artist malformed", call(artistH.CreateArtist, "POST", "{", nil), http.StatusBadRequest},
	} {
		assertJSONError(t, tc.name, tc.rec, tc.want)
	}
}

// An unclassified failure must be a 500 whose body says nothing: no SQL, no
// driver text, no constraint names.
func TestUnclassifiedErrorsDoNotLeak(t *testing.T) {
	leaky := errors.New(`pq: duplicate key value violates unique constraint "artists_pkey" DETAIL: Key (id)=(x)`)
	rec := call(NewAlbumHandler(&fakeAlbumService{err: leaky}, testutil.Quiet()).GetAlbum,
		"GET", "", map[string]string{"id": goodID})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "internal server error" {
		t.Fatalf("leaked detail to the client: %q", body["error"])
	}
}

// A malformed id used to reach Postgres, where "invalid input syntax for type
// uuid" is an unclassified error and so a 500.
func TestMalformedIDsAreRejectedBeforeTheRepository(t *testing.T) {
	quiet := testutil.Quiet()
	repo := &fakeTrackRepo{}
	trackH := NewTrackHandler(newTestTrackService(repo), nil, 1<<20, quiet)
	albums := &fakeAlbumService{}
	playlists := &fakePlaylistService{}
	albumH := NewAlbumHandler(albums, quiet)
	playlistH := NewPlaylistHandler(playlists, quiet)

	bad := map[string]string{"id": badID}
	badPair := map[string]string{"id": badID, "trackId": badID}
	goodThenBad := map[string]string{"id": goodID, "trackId": badID}

	for _, tc := range []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"GET /tracks/{id}", call(trackH.GetTrack, "GET", "", bad)},
		{"PUT /tracks/{id}/file", call(trackH.UploadTrackFile, "PUT", "body", bad)},
		{"DELETE /tracks/{id}", call(trackH.DeleteTrack, "DELETE", "", bad)},
		{"GET /albums/{id}", call(albumH.GetAlbum, "GET", "", bad)},
		{"DELETE /albums/{id}", call(albumH.DeleteAlbum, "DELETE", "", bad)},
		{"GET /playlists/{id}", call(playlistH.GetPlaylist, "GET", "", bad)},
		{"DELETE /playlists/{id}", call(playlistH.DeletePlaylist, "DELETE", "", bad)},
		{"PUT playlist track", call(playlistH.AddTrackToPlaylist, "PUT", "", badPair)},
		{"DEL playlist track", call(playlistH.RemoveTrackFromPlaylist, "DELETE", "", badPair)},
		// The second id is checked too, not only the first.
		{"PUT playlist bad trackId", call(playlistH.AddTrackToPlaylist, "PUT", "", goodThenBad)},
	} {
		assertJSONError(t, tc.name, tc.rec, http.StatusBadRequest)
	}

	if len(repo.reached) != 0 {
		t.Errorf("a malformed id reached the track repository: %v", repo.reached)
	}
	if len(albums.reached) != 0 {
		t.Errorf("a malformed id reached the album service: %v", albums.reached)
	}
	if len(playlists.reached) != 0 {
		t.Errorf("a malformed id reached the playlist service: %v", playlists.reached)
	}
}

// The album and playlist layers took no context at all, so a client
// disconnect could not cancel the query behind it.
func TestRequestContextReachesTheService(t *testing.T) {
	quiet := testutil.Quiet()
	type key struct{}

	withCtx := func(fn http.HandlerFunc, method, body string, vals map[string]string, marker string) {
		req := httptest.NewRequest(method, "/x", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), key{}, marker))
		for k, v := range vals {
			req.SetPathValue(k, v)
		}
		fn(httptest.NewRecorder(), req)
	}
	id := map[string]string{"id": goodID}
	pair := map[string]string{"id": goodID, "trackId": goodID}

	albums := &fakeAlbumService{}
	albumH := NewAlbumHandler(albums, quiet)
	for _, tc := range []struct {
		name string
		fn   http.HandlerFunc
		verb string
		body string
		vals map[string]string
	}{
		{"GetAlbum", albumH.GetAlbum, "GET", "", id},
		{"CreateAlbum", albumH.CreateAlbum, "POST", `{"title":"a","artist_id":"` + goodID + `"}`, nil},
		{"DeleteAlbum", albumH.DeleteAlbum, "DELETE", "", id},
	} {
		albums.gotCtx = nil
		withCtx(tc.fn, tc.verb, tc.body, tc.vals, tc.name)
		if albums.gotCtx == nil || albums.gotCtx.Value(key{}) != tc.name {
			t.Errorf("%s did not thread the request context", tc.name)
		}
	}

	playlists := &fakePlaylistService{}
	playlistH := NewPlaylistHandler(playlists, quiet)
	for _, tc := range []struct {
		name string
		fn   http.HandlerFunc
		verb string
		body string
		vals map[string]string
	}{
		{"GetPlaylist", playlistH.GetPlaylist, "GET", "", id},
		{"CreatePlaylist", playlistH.CreatePlaylist, "POST", `{"title":"p"}`, nil},
		{"DeletePlaylist", playlistH.DeletePlaylist, "DELETE", "", id},
		{"AddTrack", playlistH.AddTrackToPlaylist, "PUT", "", pair},
		{"RemoveTrack", playlistH.RemoveTrackFromPlaylist, "DELETE", "", pair},
	} {
		playlists.gotCtx = nil
		withCtx(tc.fn, tc.verb, tc.body, tc.vals, tc.name)
		if playlists.gotCtx == nil || playlists.gotCtx.Value(key{}) != tc.name {
			t.Errorf("%s did not thread the request context", tc.name)
		}
	}
}
