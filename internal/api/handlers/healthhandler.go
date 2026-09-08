package handlers

import (
	"context"
	"database/sql"
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

func NewHealthHandler(db *sql.DB, blobs storage.BlobStore, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{db: db, blobs: blobs, logger: logger}
}

// Healthz reports that the process is up
// @Summary      Liveness probe
// @Description  Reports that the process is running. Dependencies are deliberately not checked - restarting the server does not fix a database outage.
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /healthz [get]
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	httpx.RespondWithJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz reports whether the server can serve traffic
// @Summary      Readiness probe
// @Description  Checks the database and the blob store. Answers 503 with a per-dependency breakdown when either is unreachable.
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      503  {object}  map[string]any
// @Router       /readyz [get]
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
