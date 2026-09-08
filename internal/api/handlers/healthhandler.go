package handlers

import (
	"context"
	"log/slog"
	"mydal/internal/httpx"
	"mydal/internal/storage"
	"net/http"
	"time"
)

// readyTimeout bounds the dependency checks: a probe that hangs is worse than
// one that fails, because an orchestrator learns nothing from it.
const readyTimeout = 2 * time.Second

// Pinger is the part of *sql.DB readiness needs.
type Pinger interface {
	PingContext(ctx context.Context) error
}

type HealthHandler struct {
	db     Pinger
	blobs  storage.BlobStore
	logger *slog.Logger
}

// NewHealthHandler takes Pinger rather than *sql.DB - the only thing this
// handler needs from the pool - so a test can pass a fake instead of
// constructing the struct by hand to get around the concrete type.
func NewHealthHandler(db Pinger, blobs storage.BlobStore, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{db: db, blobs: blobs, logger: logger}
}

// Healthz reports that the process is up. Dependencies are deliberately not
// checked - restarting the server does not fix a database outage.
//
// It sits outside /api/v1 (see router.go) and so outside this package's
// generated OpenAPI spec too, which only covers the versioned API; @BasePath
// in main.go applies to every documented path, and would otherwise publish
// this at /api/v1/healthz - a path that 404s. It is documented in the README
// instead. No swag annotations belong on this handler.
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	httpx.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz reports whether the server can serve traffic: it checks the
// database and the blob store, answering 503 with a per-dependency breakdown
// when either is unreachable.
//
// See the comment on Healthz - the same reasoning keeps this out of the
// generated spec.
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	checks := map[string]string{}
	ready := true
	for name, check := range map[string]func(context.Context) error{
		"database":  h.db.PingContext,
		"blobstore": h.blobs.Ping,
	} {
		if err := check(ctx); err != nil {
			h.logger.Error("Readiness check failed", "dependency", name, "error", err)
			checks[name] = "unavailable"
			ready = false
			continue
		}
		checks[name] = "ok"
	}

	status, label := http.StatusOK, "ready"
	if !ready {
		status, label = http.StatusServiceUnavailable, "not ready"
	}
	httpx.RespondWithJSON(w, status, map[string]any{"status": label, "checks": checks})
}
