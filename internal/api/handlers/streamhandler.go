package handlers

import (
	"io"
	"log/slog"
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
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /tracks/{id}/stream [get]
func (h *StreamHandler) StreamTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	track, err := h.trackService.GetTrackByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	if track.StorageKey == "" {
		http.Error(w, "Track file not yet uploaded", http.StatusNotFound)
		return
	}

	info, err := h.blobs.Stat(r.Context(), track.StorageKey)
	if err != nil {
		h.logger.Error("Failed to stat track object", "error", err)
		http.Error(w, "Failed to retrieve track", http.StatusInternalServerError)
		return
	}

	body, err := h.blobs.Get(r.Context(), track.StorageKey)
	if err != nil {
		h.logger.Error("Failed to retrieve track object", "error", err)
		http.Error(w, "Failed to retrieve track", http.StatusInternalServerError)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", info.ContentType)

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
