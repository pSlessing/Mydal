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

func (h *AlbumHandler) DeleteAlbum(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/albums/"):]
	if err := h.service.DeleteAlbum(id); err != nil {
		h.logger.Error("Failed to delete album", "error", err)
		http.Error(w, "Failed to delete album", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
