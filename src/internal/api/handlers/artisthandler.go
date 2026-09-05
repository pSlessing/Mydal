package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mydal/src/internal/domain"
	"mydal/src/internal/httpx"
	"net/http"

	"github.com/google/uuid"
)

// ArtistService is the behaviour the artist handler needs, declared here so
// the handler can be tested against a fake.
type ArtistService interface {
	GetArtistByID(ctx context.Context, id string) (*domain.Artist, error)
	CreateArtist(ctx context.Context, artist *domain.Artist) error
	DeleteArtist(ctx context.Context, id string) error
}

type ArtistHandler struct {
	artistService ArtistService
	logger        *slog.Logger
}

func NewArtistHandler(artistService ArtistService, logger *slog.Logger) *ArtistHandler {
	return &ArtistHandler{artistService: artistService, logger: logger}
}

// pathUUID reads a path variable and rejects anything that is not a UUID, so a
// malformed id is a 400 here rather than a database error mapped to a 500.
func pathUUID(r *http.Request, name string) (string, error) {
	id := r.PathValue(name)
	if _, err := uuid.Parse(id); err != nil {
		return "", fmt.Errorf("%w: %q is not a valid uuid", domain.ErrInvalidInput, id)
	}
	return id, nil
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
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, h.logger, err)
		return
	}
	artist, err := h.artistService.GetArtistByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusOK, newArtistResponse(artist))
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
	var req createArtistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, h.logger, fmt.Errorf("%w: malformed JSON body", domain.ErrInvalidInput))
		return
	}
	artist := domain.Artist{Name: req.Name, Bio: req.Bio}
	if err := h.artistService.CreateArtist(r.Context(), &artist); err != nil {
		httpx.WriteError(w, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusCreated, newArtistResponse(&artist))
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
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, h.logger, err)
		return
	}
	if err := h.artistService.DeleteArtist(r.Context(), id); err != nil {
		httpx.WriteError(w, h.logger, err)
		return
	}
	httpx.RespondNoContent(w)
}
