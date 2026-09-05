package api

import (
	"context"
	"log/slog"
	"mydal/internal/httpx"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
)

type contextKey int

const requestIDKey contextKey = iota

// RequestIDFromContext returns the id assigned to this request, if any.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// responseRecorder captures the status and size for the log line.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (rec *responseRecorder) WriteHeader(code int) {
	if rec.wroteHeader {
		return
	}
	rec.status = code
	rec.wroteHeader = true
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *responseRecorder) Write(b []byte) (int, error) {
	if !rec.wroteHeader {
		rec.WriteHeader(http.StatusOK)
	}
	n, err := rec.ResponseWriter.Write(b)
	rec.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer, which
// matters for the flushing and deadline control that streaming will need.
func (rec *responseRecorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }

// headerTracker reports whether a response has already begun.
type headerTracker interface{ headerWritten() bool }

func (rec *responseRecorder) headerWritten() bool { return rec.wroteHeader }

// RequestID assigns each request an id, echoes it in X-Request-Id and puts it
// in the context so handlers and the log line can refer to the same request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// Logging records one line per request with method, path, status, size and
// duration.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			logger.Info("Request",
				"request_id", RequestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// Recover turns a panic into a 500 instead of a killed connection.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rvr := recover()
				if rvr == nil {
					return
				}
				// http.ErrAbortHandler is the documented way to abort a
				// response on purpose; leave it to the server.
				if rvr == http.ErrAbortHandler {
					panic(rvr)
				}
				logger.Error("Panic recovered",
					"request_id", RequestIDFromContext(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"panic", rvr,
					"stack", string(debug.Stack()),
				)
				// If the handler already started writing, the status is set
				// and a second WriteHeader would only log a warning.
				if tracker, ok := w.(headerTracker); ok && tracker.headerWritten() {
					return
				}
				httpx.RespondWithError(w, http.StatusInternalServerError, "internal server error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}
