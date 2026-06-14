package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/service"
	"net/http"
)

type TrackHandler struct {
	trackService *service.TrackService
	minioService *service.Minioservice
	logger       *slog.Logger
}

func NewTrackHandler(trackService *service.TrackService, minioService *service.Minioservice, logger *slog.Logger) *TrackHandler {
	return &TrackHandler{
		trackService: trackService,
		minioService: minioService,
		logger:       logger,
	}
}

func (h *TrackHandler) GetTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	track, err := h.trackService.GetTrackByID(id)
	if err != nil {
		h.logger.Error("Failed to get track", "error", err)
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(track)
}

func (h *TrackHandler) CreateTrack(w http.ResponseWriter, r *http.Request) {
	var track domain.Track
	if err := json.NewDecoder(r.Body).Decode(&track); err != nil {
		h.logger.Error("Failed to decode track", "error", err)
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if err := h.trackService.CreateTrack(&track); err != nil {
		h.logger.Error("Failed to create track", "error", err)
		http.Error(w, "Failed to create track", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(track)
}

func (h *TrackHandler) UploadTrackFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	_, err := h.trackService.GetTrackByID(id)
	if err != nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	size := r.ContentLength
	if size <= 0 {
		http.Error(w, "Content-Length is required", http.StatusBadRequest)
		return
	}

	ext := extensionFromContentType(contentType)
	storageKey := fmt.Sprintf("tracks/%s%s", id, ext)

	if err := h.minioService.UploadTrack(r.Context(), storageKey, contentType, size, r.Body); err != nil {
		h.logger.Error("Failed to upload track file", "error", err)
		http.Error(w, "Failed to upload file", http.StatusInternalServerError)
		return
	}

	if err := h.trackService.UpdateStorageKey(id, storageKey); err != nil {
		h.logger.Error("Failed to update storage key", "error", err)
		http.Error(w, "Failed to update track record", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TrackHandler) DeleteTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.trackService.DeleteTrack(id); err != nil {
		h.logger.Error("Failed to delete track", "error", err)
		http.Error(w, "Failed to delete track", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func extensionFromContentType(ct string) string {
	switch ct {
	case "audio/flac":
		return ".flac"
	case "audio/mpeg":
		return ".mp3"
	case "audio/ogg":
		return ".ogg"
	case "audio/wav":
		return ".wav"
	case "audio/aac":
		return ".aac"
	default:
		return ""
	}
}
