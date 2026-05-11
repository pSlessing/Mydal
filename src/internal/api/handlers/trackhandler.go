package handlers

import (
	"log/slog"
	"mydal/src/internal/service"
	"net/http"
)

type TrackHandler struct {
	trackService *service.TrackService
	minioService *service.Minioservice
	logger       *slog.Logger
}

func NewTrackHandler(trackService *service.TrackService, minioService *service.Minioservice, logger *slog.Logger) *TrackHandler {
	return &TrackHandler{trackService: trackService, minioService: minioService, logger: logger}
}

// Endpoints for just track information
func (h *TrackHandler) GetTrack(w http.ResponseWriter, r *http.Request) {
	// Implement logic to get track information by ID
}

func (h *TrackHandler) CreateTrack(w http.ResponseWriter, r *http.Request) {
	// Implement logic to create a new track
}

func (h *TrackHandler) DeleteTrack(w http.ResponseWriter, r *http.Request) {
	// Implement logic to delete a track by ID
}

//Endpoints for track file management (upload/download/delete) using minioService
