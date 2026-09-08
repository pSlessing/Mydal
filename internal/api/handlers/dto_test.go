package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"mydal/internal/testutil"
)

func responseKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("not a JSON object: %s", raw)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func postJSON(fn http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

// A client-supplied storage_key used to reach the insert, which let a caller
// point a new track row at any object already in the bucket - and then stream
// it. The same hole existed for an album's cover_key.
func TestServerOwnedFieldsAreNotSettable(t *testing.T) {
	quiet := testutil.Quiet()

	repo := &fakeTrackRepo{}
	trackH := NewTrackHandler(newTestTrackService(repo), nil, 1<<20, quiet)
	rec := postJSON(trackH.CreateTrack, `{
		"title":"T","artist_id":"11111111-1111-1111-1111-111111111111",
		"storage_key":"tracks/someone-elses.mp3","StorageKey":"tracks/someone-elses.mp3",
		"content_hash":"deadbeef","ContentHash":"deadbeef",
		"id":"client-chosen","ID":"client-chosen","created_at":"1999-01-01T00:00:00Z"
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body)
	}
	if repo.created.StorageKey != "" {
		t.Errorf("client set storage_key: %q", repo.created.StorageKey)
	}
	if repo.created.ContentHash != "" {
		t.Errorf("client set content_hash: %q", repo.created.ContentHash)
	}
	if repo.created.ID != "server-generated" {
		t.Errorf("client set id: %q", repo.created.ID)
	}

	albums := &fakeAlbumService{}
	albumH := NewAlbumHandler(albums, quiet)
	rec = postJSON(albumH.CreateAlbum, `{"title":"A","artist_id":"11111111-1111-1111-1111-111111111111","cover_key":"covers/hijack"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("album create = %d: %s", rec.Code, rec.Body)
	}
	if albums.created.CoverKey != "" {
		t.Errorf("client set cover_key: %q", albums.created.CoverKey)
	}
}

// The domain structs carry no JSON tags, so decoding into them put Go field
// names on the wire.
func TestResponsesAreSnakeCase(t *testing.T) {
	quiet := testutil.Quiet()
	id := "11111111-1111-1111-1111-111111111111"

	for _, tc := range []struct {
		name string
		rec  *httptest.ResponseRecorder
		want []string
	}{
		{
			"track",
			postJSON(NewTrackHandler(newTestTrackService(&fakeTrackRepo{}), nil, 1<<20, quiet).CreateTrack,
				`{"title":"T","artist_id":"`+id+`"}`),
			[]string{"artist_id", "bitrate", "created_at", "disc_number", "duration_ms",
				"file_size", "format", "id", "title", "track_number"},
		},
		{
			"album",
			postJSON(NewAlbumHandler(&fakeAlbumService{}, quiet).CreateAlbum,
				`{"title":"A","artist_id":"`+id+`"}`),
			[]string{"artist_id", "created_at", "id", "release_date", "title"},
		},
		{
			"playlist",
			postJSON(NewPlaylistHandler(&fakePlaylistService{}, quiet).CreatePlaylist,
				`{"title":"P"}`),
			[]string{"created_at", "description", "id", "title", "track_ids", "updated_at"},
		},
		{
			"artist",
			postJSON(NewArtistHandler(&fakeArtistService{}, quiet).CreateArtist, `{"name":"N"}`),
			[]string{"bio", "created_at", "id", "name"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.rec.Code != http.StatusCreated {
				t.Fatalf("status = %d: %s", tc.rec.Code, tc.rec.Body)
			}
			got := responseKeys(t, tc.rec.Body.Bytes())
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("keys = %v, want %v", got, tc.want)
			}
		})
	}
}

// release_date is a DATE: no time, no zone, and absent means null rather than
// a zero year.
func TestReleaseDateIsABareDate(t *testing.T) {
	quiet := testutil.Quiet()
	albums := &fakeAlbumService{}
	h := NewAlbumHandler(albums, quiet)
	id := "11111111-1111-1111-1111-111111111111"

	rec := postJSON(h.CreateAlbum, `{"title":"UP","artist_id":"`+id+`","release_date":"1979-08-17"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if albums.created.ReleaseDate == nil || albums.created.ReleaseDate.Format("2006-01-02") != "1979-08-17" {
		t.Fatalf("parsed date = %v", albums.created.ReleaseDate)
	}
	if !strings.Contains(rec.Body.String(), `"release_date":"1979-08-17"`) {
		t.Fatalf("not echoed as a bare date: %s", rec.Body)
	}

	if rec = postJSON(h.CreateAlbum, `{"title":"X","artist_id":"`+id+`"}`); !strings.Contains(rec.Body.String(), `"release_date":null`) {
		t.Fatalf("absent date should be null: %s", rec.Body)
	}
	if rec = postJSON(h.CreateAlbum, `{"title":"X","artist_id":"`+id+`","release_date":"17-08-1979"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed date = %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestEmptyPlaylistSerialisesAsEmptyArray(t *testing.T) {
	rec := postJSON(NewPlaylistHandler(&fakePlaylistService{}, testutil.Quiet()).CreatePlaylist, `{"title":"P"}`)
	if !strings.Contains(rec.Body.String(), `"track_ids":[]`) {
		t.Fatalf("want [] not null: %s", rec.Body)
	}
}
