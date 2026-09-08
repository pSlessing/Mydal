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
// it. The same hole existed for an album's cover_key. decodeJSON now disallows
// any field a request DTO does not declare, so naming a server-owned field is
// a 400 that never reaches the service, rather than a value the DTO happened
// to have no field for.
func TestServerOwnedFieldsAreRejected(t *testing.T) {
	quiet := testutil.Quiet()

	repo := &fakeTrackRepo{}
	trackH := NewTrackHandler(newTestTrackService(repo), nil, 1<<20, quiet)
	rec := postJSON(trackH.CreateTrack, `{
		"title":"T","artist_id":"11111111-1111-1111-1111-111111111111",
		"storage_key":"tracks/someone-elses.mp3"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create with storage_key = %d, want 400: %s", rec.Code, rec.Body)
	}
	if len(repo.reached) != 0 {
		t.Errorf("a rejected body still reached the repository: %v", repo.reached)
	}

	albums := &fakeAlbumService{}
	albumH := NewAlbumHandler(albums, quiet)
	rec = postJSON(albumH.CreateAlbum, `{"title":"A","artist_id":"11111111-1111-1111-1111-111111111111","cover_key":"covers/hijack"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("album create with cover_key = %d, want 400: %s", rec.Code, rec.Body)
	}
	if len(albums.reached) != 0 {
		t.Errorf("a rejected body still reached the service: %v", albums.reached)
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
			[]string{"album_id", "artist_id", "bitrate", "created_at", "disc_number", "duration_ms",
				"file_size", "format", "has_file", "id", "title", "track_number"},
		},
		{
			"album",
			postJSON(NewAlbumHandler(&fakeAlbumService{}, quiet).CreateAlbum,
				`{"title":"A","artist_id":"`+id+`"}`),
			[]string{"artist_id", "cover_key", "created_at", "id", "release_date", "title"},
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
