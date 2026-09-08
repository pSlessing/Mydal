package handlers

import (
	"context"
	"log/slog"
	"mydal/internal/domain"
	"mydal/internal/httpx"
	"net/http"
)

// AlbumService is the behaviour the album handler needs, declared here so the
// handler can be tested against a fake.
type AlbumService interface {
	GetAlbumByID(ctx context.Context, id string) (*domain.Album, error)
	CreateAlbum(ctx context.Context, album *domain.Album) error
	DeleteAlbum(ctx context.Context, id string) error
}

type AlbumHandler struct {
	service AlbumService
	logger  *slog.Logger
}

func NewAlbumHandler(service AlbumService, logger *slog.Logger) *AlbumHandler {
	return &AlbumHandler{service: service, logger: logger}
}

// GetAlbum retrieves an album by ID
// @Summary      Get album by ID
// @Description  Retrieve a single album by its unique identifier
// @Tags         albums
// @Produce      json
// @Param        id   path      string  true  "Album ID"
// @Success      200  {object}  albumResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /albums/{id} [get]
func (h *AlbumHandler) GetAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	album, err := h.service.GetAlbumByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusOK, newAlbumResponse(album))
}

// CreateAlbum creates a new album
// @Summary      Create a new album
// @Description  Add a new album to the database
// @Tags         albums
// @Accept       json
// @Produce      json
// @Param        album  body      createAlbumRequest  true  "Album payload"
// @Success      201    {object}  albumResponse
// @Failure      400    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /albums [post]
func (h *AlbumHandler) CreateAlbum(w http.ResponseWriter, r *http.Request) {
	var req createAlbumRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	album, err := req.toDomain()
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.service.CreateAlbum(r.Context(), &album); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusCreated, newAlbumResponse(&album))
}

// DeleteAlbum deletes an album
// @Summary      Delete an album
// @Description  Remove an album from the database by ID
// @Tags         albums
// @Produce      json
// @Param        id   path      string  true  "Album ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /albums/{id} [delete]
func (h *AlbumHandler) DeleteAlbum(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.service.DeleteAlbum(r.Context(), id); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondNoContent(w)
}
