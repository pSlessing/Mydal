package handlers

import (
	"encoding/json"
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/service"
	"net/http"
)

type AlbumHandler struct {
	service *service.AlbumService
	logger  *slog.Logger
}

func NewAlbumHandler(service *service.AlbumService, logger *slog.Logger) *AlbumHandler {
	return &AlbumHandler{service: service, logger: logger}
}

// GetAlbum retrieves an album by ID
// @Summary      Get album by ID
// @Description  Retrieve a single album by its unique identifier
// @Tags         albums
// @Produce      json
// @Param        id   path      string  true  "Album ID"
// @Success      200  {object}  domain.Album
// @Failure      404  {object}  map[string]string
// @Router       /albums/{id} [get]
func (h *AlbumHandler) GetAlbum(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/albums/"):]
	album, err := h.service.GetAlbumByID(id)
	if err != nil {
		h.logger.Error("Failed to get album", "error", err)
		http.Error(w, "Album not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(album)
}

// CreateAlbum creates a new album
// @Summary      Create a new album
// @Description  Add a new album to the database
// @Tags         albums
// @Accept       json
// @Produce      json
// @Param        album  body      domain.Album  true  "Album payload"
// @Success      201    {object}  domain.Album
// @Failure      400    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /albums [post]
func (h *AlbumHandler) CreateAlbum(w http.ResponseWriter, r *http.Request) {
	var album domain.Album
	if err := json.NewDecoder(r.Body).Decode(&album); err != nil {
		h.logger.Error("Failed to decode album", "error", err)
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if err := h.service.CreateAlbum(&album); err != nil {
		h.logger.Error("Failed to create album", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(album)
}

// DeleteAlbum deletes an album
// @Summary      Delete an album
// @Description  Remove an album from the database by ID
// @Tags         albums
// @Param        id   path      string  true  "Album ID"
// @Success      204  "No Content"
// @Failure      500  {object}  map[string]string
// @Router       /albums/{id} [delete]
func (h *AlbumHandler) DeleteAlbum(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/albums/"):]
	if err := h.service.DeleteAlbum(id); err != nil {
		h.logger.Error("Failed to delete album", "error", err)
		http.Error(w, "Failed to delete album", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
