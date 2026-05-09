package handlers

import (
	"encoding/json"
	"mydal/src/internal/domain"
	"mydal/src/internal/service"
	"net/http"
)

type ArtistHandler struct {
	artistService *service.ArtistService
}

func NewArtistHandler(artistService *service.ArtistService) *ArtistHandler {
	return &ArtistHandler{artistService: artistService}
}

func (h *ArtistHandler) GetArtist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/artists/"):]
	artist, err := h.artistService.GetArtistByID(id)
	if err != nil {
		http.Error(w, "Artist not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(artist)
}

func (h *ArtistHandler) CreateArtist(w http.ResponseWriter, r *http.Request) {
	var artist domain.Artist
	if err := json.NewDecoder(r.Body).Decode(&artist); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if err := h.artistService.CreateArtist(&artist); err != nil {
		http.Error(w, "Failed to create artist", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(artist)
}

func (h *ArtistHandler) DeleteArtist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/artists/"):]
	if err := h.artistService.DeleteArtist(id); err != nil {
		http.Error(w, "Failed to delete artist", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
