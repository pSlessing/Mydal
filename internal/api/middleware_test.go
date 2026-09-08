package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mydal/internal/httpx"
)

func TestRequestIDGeneratesWhenAbsent(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	got := rec.Header().Get("X-Request-Id")
	if got == "" {
		t.Fatal("X-Request-Id header not set")
	}
	if seen != got {
		t.Errorf("context id = %q, want it to match the response header %q", seen, got)
	}
}

// A client-supplied request id is echoed back rather than replaced, so a
// caller that already has a correlation id for its own logs keeps using it.
func TestRequestIDEchoesExisting(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "client-supplied-id")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-Id"); got != "client-supplied-id" {
		t.Errorf("X-Request-Id = %q, want the client's own value echoed", got)
	}
	if seen != "client-supplied-id" {
		t.Errorf("context id = %q, want client-supplied-id", seen)
	}
}

// One line per request, carrying enough to correlate with WriteError's own
// line for the same request (see httpx.TestWriteErrorLogsTheRequestID).
func TestLoggingRecordsOneLineWithTheRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := RequestID(Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("hello"))
	})))

	req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
	req.Header.Set("X-Request-Id", "req-abc")
	h.ServeHTTP(httptest.NewRecorder(), req)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
	}
	if line["msg"] != "Request" {
		t.Errorf("msg = %v, want \"Request\"", line["msg"])
	}
	if line["request_id"] != "req-abc" {
		t.Errorf("request_id = %v, want req-abc", line["request_id"])
	}
	if line["method"] != http.MethodGet {
		t.Errorf("method = %v", line["method"])
	}
	if line["path"] != "/some/path" {
		t.Errorf("path = %v", line["path"])
	}
	if line["status"] != float64(http.StatusTeapot) {
		t.Errorf("status = %v, want %d", line["status"], http.StatusTeapot)
	}
	if line["bytes"] != float64(len("hello")) {
		t.Errorf("bytes = %v, want %d", line["bytes"], len("hello"))
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
}

// A handler that never calls WriteHeader still gets logged as a 200, since
// that is what the first Write (or an empty body) actually answers as.
func TestLoggingDefaultsToStatusOKWhenUnset(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not JSON: %v", err)
	}
	if line["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want %d", line["status"], http.StatusOK)
	}
}

// A panic becomes a 500 with the JSON error contract, not a killed
// connection, and is logged once with enough to debug it.
func TestRecoverConvertsAPanicToJSON500(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body)
	}
	if body["code"] != "internal_error" {
		t.Errorf("code = %q, want internal_error", body["code"])
	}

	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("panic value missing from the log line: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"level":"ERROR"`) {
		t.Errorf("panic was not logged at Error level: %s", buf.String())
	}
}

// http.ErrAbortHandler is the documented way to abort a response on purpose
// (net/http's own server treats it specially); Recover must let it continue
// up the stack rather than converting it into a 500.
func TestRecoverLetsErrAbortHandlerThrough(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		rvr := recover()
		if rvr != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler to propagate", rvr)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Fatal("ServeHTTP returned normally; want it to panic with http.ErrAbortHandler")
}

// A handler that already wrote a header before panicking left the response
// in a state Recover cannot repair; it must not attempt a second WriteHeader.
func TestRecoverDoesNotDoubleWriteAfterHeadersSent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Logging(logger)(Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		panic("late boom")
	})))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want the original 200 to survive", rec.Code)
	}
}
