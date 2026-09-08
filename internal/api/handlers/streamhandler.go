package handlers

import (
	"fmt"
	"log/slog"
	"mydal/internal/domain"
	"mydal/internal/httpx"
	"mydal/internal/service"
	"mydal/internal/storage"
	"net/http"
	"strconv"
)

type StreamHandler struct {
	trackService *service.TrackService
	blobs        storage.BlobStore
	logger       *slog.Logger
}

func NewStreamHandler(trackService *service.TrackService, blobs storage.BlobStore, logger *slog.Logger) *StreamHandler {
	return &StreamHandler{trackService: trackService, blobs: blobs, logger: logger}
}

// StreamTrack streams an audio track file
// @Summary      Stream track audio
// @Description  Stream the audio file for a track via HTTP range requests
// @Tags         streaming
// @Produce      audio/*
// @Param        id   path      string  true  "Track ID"
// @Success      200  {file}    binary
// @Success      206  {file}    binary
// @Success      304  "Not Modified"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      416  {string}  string  "Plain text, not JSON: written by net/http's own range handling"
// @Failure      500  {object}  map[string]string
// @Router       /tracks/{id}/stream [get]
func (h *StreamHandler) StreamTrack(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	track, err := h.trackService.GetTrackByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if track.StorageKey == "" {
		httpx.WriteError(w, r, h.logger,
			fmt.Errorf("track %s has no audio file: %w", id, domain.ErrNotFound))
		return
	}

	// Get returns the object's metadata alongside its body, so this is the
	// only round trip to the store before the first byte goes out - the
	// metadata used to come from a separate Stat call first. The blob store
	// wraps a missing object as domain.ErrNotFound, so a row pointing at an
	// object that is gone answers 404 rather than 500.
	body, info, err := h.blobs.Get(r.Context(), track.StorageKey)
	if err != nil {
		httpx.WriteError(w, r, h.logger, fmt.Errorf("open audio for track %s: %w", id, err))
		return
	}
	defer body.Close()

	// http.ServeContent sets Accept-Ranges itself.
	w.Header().Set("Content-Type", info.ContentType)

	if track.ContentHash != "" {
		w.Header().Set("ETag", strconv.Quote(track.ContentHash))
	}

	http.ServeContent(w, r, info.Key, info.LastModified, body)
}
