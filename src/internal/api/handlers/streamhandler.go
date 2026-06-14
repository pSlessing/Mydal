package handlers

import (
	"log/slog"
	"mydal/src/internal/service"
	"net/http"
)

type StreamHandler struct {
	trackService *service.TrackService
	minioService *service.Minioservice
	logger       *slog.Logger
}

func NewStreamHandler(trackService *service.TrackService, minioService *service.Minioservice, logger *slog.Logger) *StreamHandler {
	return &StreamHandler{
		trackService: trackService,
		minioService: minioService,
		logger:       logger,
	}
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

	track, err := h.trackService.GetTrackByID(id)
	if err != nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	if track.StorageKey == "" {
		http.Error(w, "Track file not yet uploaded", http.StatusNotFound)
		return
	}

	obj, err := h.minioService.GetTrackObject(r.Context(), "mydal", track.StorageKey)
	if err != nil {
		h.logger.Error("Failed to retrieve track object", "error", err)
		http.Error(w, "Failed to retrieve track", http.StatusInternalServerError)
		return
	}
	defer obj.Close()

	stat, err := obj.Stat()
	if err != nil {
		h.logger.Error("Failed to stat object", "error", err)
		http.Error(w, "Failed to retrieve track", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", stat.ContentType)
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, stat.Key, stat.LastModified, obj)
}
