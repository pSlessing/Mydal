package handlers

import (
	"fmt"
	"io"
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
		httpx.WriteError(w, h.logger, err)
		return
	}

	track, err := h.trackService.GetTrackByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, h.logger, err)
		return
	}
	if track.StorageKey == "" {
		httpx.WriteError(w, h.logger,
			fmt.Errorf("track %s has no audio file: %w", id, domain.ErrNotFound))
		return
	}

	// The blob store wraps a missing object as domain.ErrNotFound, so a row
	// pointing at an object that is gone answers 404 rather than 500.
	info, err := h.blobs.Stat(r.Context(), track.StorageKey)
	if err != nil {
		httpx.WriteError(w, h.logger, fmt.Errorf("stat audio for track %s: %w", id, err))
		return
	}

	body, err := h.blobs.Get(r.Context(), track.StorageKey)
	if err != nil {
		httpx.WriteError(w, h.logger, fmt.Errorf("open audio for track %s: %w", id, err))
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", info.ContentType)

	if track.ContentHash != "" {
		w.Header().Set("ETag", strconv.Quote(track.ContentHash))
	}

	// Range requests need a seekable body. MinIO objects are; a store whose
	// reader is not gets a plain sequential response rather than a broken one.
	if rs, ok := body.(io.ReadSeeker); ok {
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, info.Key, info.LastModified, rs)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Error("Failed to stream track", "error", err)
	}
}
