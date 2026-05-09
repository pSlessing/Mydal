package handlers

import (
	"encoding/json"
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/service"
	"net/http"
)

type ArtistHandler struct {
	artistService *service.ArtistService
	logger        *slog.Logger
}

func NewArtistHandler(artistService *service.ArtistService, logger *slog.Logger) *ArtistHandler {
	return &ArtistHandler{artistService: artistService, logger: logger}
}

func (h *ArtistHandler) GetArtist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/artists/"):]
	artist, err := h.artistService.GetArtistByID(id)
	if err != nil {
		h.logger.Error("Failed to get artist", "error", err)
		http.Error(w, "Artist not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(artist)
}

func (h *ArtistHandler) CreateArtist(w http.ResponseWriter, r *http.Request) {
	var artist domain.Artist
	if err := json.NewDecoder(r.Body).Decode(&artist); err != nil {
		h.logger.Error("Failed to decode artist", "error", err)
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if err := h.artistService.CreateArtist(&artist); err != nil {
		h.logger.Error("Failed to create artist", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(artist)
}

func (h *ArtistHandler) DeleteArtist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/artists/"):]
	if err := h.artistService.DeleteArtist(id); err != nil {
		h.logger.Error("Failed to delete artist", "error", err)
		http.Error(w, "Failed to delete artist", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
