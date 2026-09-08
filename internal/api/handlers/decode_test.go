package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mydal/internal/testutil"
)

// json.NewDecoder alone read an unbounded body, accepted unknown fields
// silently, and accepted trailing bytes after the first JSON value.
// decodeJSON, used by every create endpoint, closes all three.
func TestCreateEndpointsCapAndValidateTheBody(t *testing.T) {
	quiet := testutil.Quiet()
	id := "11111111-1111-1111-1111-111111111111"

	for _, tc := range []struct {
		name string
		body string
	}{
		{"oversized body", `{"title":"` + strings.Repeat("A", maxJSONBodyBytes) + `","artist_id":"` + id + `"}`},
		{"trailing garbage", `{"title":"T","artist_id":"` + id + `"}{"extra":true}`},
		{"trailing garbage after whitespace", `{"title":"T","artist_id":"` + id + `"}   garbage`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeTrackRepo{}
			rec := postJSON(NewTrackHandler(newTestTrackService(repo), nil, 1<<20, quiet).CreateTrack, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if len(repo.reached) != 0 {
				t.Errorf("a rejected body still reached the repository: %v", repo.reached)
			}
		})
	}
}

// A trailing newline is how curl and most JSON encoders end a body; it must
// not be mistaken for trailing garbage.
func TestDecodeJSONAllowsTrailingWhitespace(t *testing.T) {
	rec := postJSON(
		NewTrackHandler(newTestTrackService(&fakeTrackRepo{}), nil, 1<<20, testutil.Quiet()).CreateTrack,
		`{"title":"T","artist_id":"11111111-1111-1111-1111-111111111111"}`+"\n",
	)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
}

// decodeJSON itself, independent of any handler, for the exact size boundary.
func TestDecodeJSONEnforcesTheSizeCap(t *testing.T) {
	pad := maxJSONBodyBytes - len(`{"a":""}`) - 1
	ok := `{"a":"` + strings.Repeat("x", pad) + `"}`
	if len(ok) > maxJSONBodyBytes {
		t.Fatalf("test body of %d bytes already exceeds the %d cap", len(ok), maxJSONBodyBytes)
	}

	var dst struct {
		A string `json:"a"`
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(ok))
	if err := decodeJSON(rec, req, &dst); err != nil {
		t.Fatalf("body of %d bytes (at the cap) was rejected: %v", len(ok), err)
	}

	tooBig := ok + "x"
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tooBig))
	if err := decodeJSON(rec, req, &dst); err == nil {
		t.Fatalf("body of %d bytes (over the cap) was accepted", len(tooBig))
	}
}
