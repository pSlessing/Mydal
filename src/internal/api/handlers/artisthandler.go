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

// GetArtist retrieves an artist by ID
// @Summary      Get artist by ID
// @Description  Retrieve a single artist by their unique identifier
// @Tags         artists
// @Produce      json
// @Param        id   path      string  true  "Artist ID"
// @Success      200  {object}  domain.Artist
// @Failure      404  {object}  map[string]string
// @Router       /artists/{id} [get]
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

// CreateArtist creates a new artist
// @Summary      Create a new artist
// @Description  Add a new artist to the database
// @Tags         artists
// @Accept       json
// @Produce      json
// @Param        artist  body      domain.Artist  true  "Artist payload"
// @Success      201     {object}  domain.Artist
// @Failure      400     {object}  map[string]string
// @Failure      500     {object}  map[string]string
// @Router       /artists [post]
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

// DeleteArtist deletes an artist
// @Summary      Delete an artist
// @Description  Remove an artist from the database by ID
// @Tags         artists
// @Param        id   path      string  true  "Artist ID"
// @Success      204  "No Content"
// @Failure      500  {object}  map[string]string
// @Router       /artists/{id} [delete]
func (h *ArtistHandler) DeleteArtist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/artists/"):]
	if err := h.artistService.DeleteArtist(id); err != nil {
		h.logger.Error("Failed to delete artist", "error", err)
		http.Error(w, "Failed to delete artist", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
