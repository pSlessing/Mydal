package handlers

import (
	"encoding/json"
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/service"
	"net/http"
)

type PlaylistHandler struct {
	service *service.PlaylistService
	logger  *slog.Logger
}

func NewPlaylistHandler(service *service.PlaylistService, logger *slog.Logger) *PlaylistHandler {
	return &PlaylistHandler{service: service, logger: logger}
}

// GetPlaylist retrieves a playlist by ID
// @Summary      Get playlist by ID
// @Description  Retrieve a single playlist by its unique identifier
// @Tags         playlists
// @Produce      json
// @Param        id   path      string  true  "Playlist ID"
// @Success      200  {object}  domain.Playlist
// @Failure      404  {object}  map[string]string
// @Router       /playlists/{id} [get]
func (h *PlaylistHandler) GetPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	playlist, err := h.service.GetPlaylistByID(id)
	if err != nil {
		h.logger.Error("Failed to get playlist", "error", err)
		http.Error(w, "Playlist not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(playlist)
}

// CreatePlaylist creates a new playlist
// @Summary      Create a new playlist
// @Description  Add a new playlist to the database
// @Tags         playlists
// @Accept       json
// @Produce      json
// @Param        playlist  body      domain.Playlist  true  "Playlist payload"
// @Success      201       {object}  domain.Playlist
// @Failure      400       {object}  map[string]string
// @Failure      500       {object}  map[string]string
// @Router       /playlists [post]
func (h *PlaylistHandler) CreatePlaylist(w http.ResponseWriter, r *http.Request) {
	var playlist domain.Playlist
	if err := json.NewDecoder(r.Body).Decode(&playlist); err != nil {
		h.logger.Error("Failed to decode playlist", "error", err)
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if err := h.service.CreatePlaylist(&playlist); err != nil {
		h.logger.Error("Failed to create playlist", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(playlist)
}

// DeletePlaylist deletes a playlist
// @Summary      Delete a playlist
// @Description  Remove a playlist from the database by ID
// @Tags         playlists
// @Param        id   path      string  true  "Playlist ID"
// @Success      204  "No Content"
// @Failure      500  {object}  map[string]string
// @Router       /playlists/{id} [delete]
func (h *PlaylistHandler) DeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.service.DeletePlaylist(id); err != nil {
		h.logger.Error("Failed to delete playlist", "error", err)
		http.Error(w, "Failed to delete playlist", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddTrackToPlaylist adds a track to a playlist
// @Summary      Add track to playlist
// @Description  Add an existing track to an existing playlist
// @Tags         playlists
// @Param        id        path  string  true  "Playlist ID"
// @Param        trackId   path  string  true  "Track ID"
// @Success      204  "No Content"
// @Failure      500  {object}  map[string]string
// @Router       /playlists/{id}/tracks/{trackId} [put]
func (h *PlaylistHandler) AddTrackToPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistID := r.PathValue("id")
	trackID := r.PathValue("trackId")
	if err := h.service.AddTrack(playlistID, trackID); err != nil {
		h.logger.Error("Failed to add track to playlist", "error", err)
		http.Error(w, "Failed to add track", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveTrackFromPlaylist removes a track from a playlist
// @Summary      Remove track from playlist
// @Description  Remove a track from an existing playlist
// @Tags         playlists
// @Param        id        path  string  true  "Playlist ID"
// @Param        trackId   path  string  true  "Track ID"
// @Success      204  "No Content"
// @Failure      500  {object}  map[string]string
// @Router       /playlists/{id}/tracks/{trackId} [delete]
func (h *PlaylistHandler) RemoveTrackFromPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistID := r.PathValue("id")
	trackID := r.PathValue("trackId")
	if err := h.service.RemoveTrack(playlistID, trackID); err != nil {
		h.logger.Error("Failed to remove track from playlist", "error", err)
		http.Error(w, "Failed to remove track", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
