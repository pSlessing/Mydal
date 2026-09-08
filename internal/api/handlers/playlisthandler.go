package handlers

import (
	"context"
	"log/slog"
	"mydal/internal/domain"
	"mydal/internal/httpx"
	"net/http"
)

// PlaylistService is the behaviour the playlist handler needs, declared here
// so the handler can be tested against a fake.
type PlaylistService interface {
	GetPlaylistByID(ctx context.Context, id string) (*domain.Playlist, error)
	CreatePlaylist(ctx context.Context, p *domain.Playlist) error
	DeletePlaylist(ctx context.Context, id string) error
	AddTrack(ctx context.Context, playlistID, trackID string) error
	RemoveTrack(ctx context.Context, playlistID, trackID string) error
}

type PlaylistHandler struct {
	service PlaylistService
	logger  *slog.Logger
}

func NewPlaylistHandler(service PlaylistService, logger *slog.Logger) *PlaylistHandler {
	return &PlaylistHandler{service: service, logger: logger}
}

// GetPlaylist retrieves a playlist by ID
// @Summary      Get playlist by ID
// @Description  Retrieve a single playlist by its unique identifier
// @Tags         playlists
// @Produce      json
// @Param        id   path      string  true  "Playlist ID"
// @Success      200  {object}  playlistResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /playlists/{id} [get]
func (h *PlaylistHandler) GetPlaylist(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	playlist, err := h.service.GetPlaylistByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusOK, newPlaylistResponse(playlist))
}

// CreatePlaylist creates a new playlist
// @Summary      Create a new playlist
// @Description  Add a new playlist to the database
// @Tags         playlists
// @Accept       json
// @Produce      json
// @Param        playlist  body      createPlaylistRequest  true  "Playlist payload"
// @Success      201       {object}  playlistResponse
// @Failure      400       {object}  map[string]string
// @Failure      409       {object}  map[string]string
// @Failure      500       {object}  map[string]string
// @Router       /playlists [post]
func (h *PlaylistHandler) CreatePlaylist(w http.ResponseWriter, r *http.Request) {
	var req createPlaylistRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	playlist := req.toDomain()
	if err := h.service.CreatePlaylist(r.Context(), &playlist); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusCreated, newPlaylistResponse(&playlist))
}

// DeletePlaylist deletes a playlist
// @Summary      Delete a playlist
// @Description  Remove a playlist from the database by ID
// @Tags         playlists
// @Produce      json
// @Param        id   path      string  true  "Playlist ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /playlists/{id} [delete]
func (h *PlaylistHandler) DeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.service.DeletePlaylist(r.Context(), id); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondNoContent(w)
}

// AddTrackToPlaylist adds a track to a playlist
// @Summary      Add track to playlist
// @Description  Add an existing track to an existing playlist
// @Tags         playlists
// @Produce      json
// @Param        id        path  string  true  "Playlist ID"
// @Param        trackId   path  string  true  "Track ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /playlists/{id}/tracks/{trackId} [put]
func (h *PlaylistHandler) AddTrackToPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistID, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	trackID, err := pathUUID(r, "trackId")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.service.AddTrack(r.Context(), playlistID, trackID); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondNoContent(w)
}

// RemoveTrackFromPlaylist removes a track from a playlist
// @Summary      Remove track from playlist
// @Description  Remove a track from an existing playlist
// @Tags         playlists
// @Produce      json
// @Param        id        path  string  true  "Playlist ID"
// @Param        trackId   path  string  true  "Track ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /playlists/{id}/tracks/{trackId} [delete]
func (h *PlaylistHandler) RemoveTrackFromPlaylist(w http.ResponseWriter, r *http.Request) {
	playlistID, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	trackID, err := pathUUID(r, "trackId")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.service.RemoveTrack(r.Context(), playlistID, trackID); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondNoContent(w)
}
