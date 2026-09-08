package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mydal/internal/domain"
	"mydal/internal/httpx"
	"mydal/internal/service"
	"mydal/internal/storage"
	"net/http"
	"time"
)

// uploadReadTimeout bounds how long a single upload's body may take to
// arrive. serve sets no server-wide ReadTimeout, deliberately, because
// streaming a large audio file back out is a long-lived response - but that
// leaves nothing to stop a slow or stalled client from holding an upload's
// connection open indefinitely. This is generous enough for a real album side
// on a slow link.
const uploadReadTimeout = 15 * time.Minute

type TrackHandler struct {
	trackService   *service.TrackService
	blobs          storage.BlobStore
	maxUploadBytes int64
	logger         *slog.Logger
}

func NewTrackHandler(trackService *service.TrackService, blobs storage.BlobStore, maxUploadBytes int64, logger *slog.Logger) *TrackHandler {
	return &TrackHandler{
		trackService:   trackService,
		blobs:          blobs,
		maxUploadBytes: maxUploadBytes,
		logger:         logger,
	}
}

// GetTrack retrieves a track by ID
// @Summary      Get track by ID
// @Description  Retrieve a single track by its unique identifier
// @Tags         tracks
// @Produce      json
// @Param        id   path      string  true  "Track ID"
// @Success      200  {object}  trackResponse
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /tracks/{id} [get]
func (h *TrackHandler) GetTrack(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	track, err := h.trackService.GetTrackByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusOK, newTrackResponse(track))
}

// CreateTrack creates a new track
// @Summary      Create a new track
// @Description  Add a new track to the database
// @Tags         tracks
// @Accept       json
// @Produce      json
// @Param        track  body      createTrackRequest  true  "Track payload"
// @Success      201    {object}  trackResponse
// @Failure      400    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /tracks [post]
func (h *TrackHandler) CreateTrack(w http.ResponseWriter, r *http.Request) {
	var req createTrackRequest
	if err := decodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	track := req.toDomain()
	if err := h.trackService.CreateTrack(r.Context(), &track); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondWithJSON(w, http.StatusCreated, newTrackResponse(&track))
}

// UploadTrackFile uploads an audio file for a track
// @Summary      Upload track audio file
// @Description  Upload the audio binary for an existing track. The format is detected from the file's own bytes; the Content-Type header is ignored. A chunked body without Content-Length is accepted.
// @Tags         tracks
// @Accept       octet-stream
// @Produce      json
// @Param        id       path  string  true  "Track ID"
// @Param        audio    body  string  true  "Raw audio bytes, sent as the request body - not a multipart form"
// @Success      204      "No Content"
// @Failure      400      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      409      {object}  map[string]string
// @Failure      413      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /tracks/{id}/file [put]
func (h *TrackHandler) UploadTrackFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}

	track, err := h.trackService.GetTrackByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	previousKey := track.StorageKey

	// Bound how long reading the body may take; the recorder and mux-error
	// wrapper in the middleware chain both forward Unwrap, so this reaches the
	// real connection underneath either.
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		httpx.WriteError(w, r, h.logger, fmt.Errorf("set upload read deadline for track %s: %w", id, err))
		return
	}

	// Reject an oversized upload before reading it, when the client declared a
	// length; MaxBytesReader catches the rest, including a chunked body that
	// declares nothing.
	if r.ContentLength > h.maxUploadBytes {
		h.writeTooLarge(w)
		return
	}
	body := http.MaxBytesReader(w, r.Body, h.maxUploadBytes)

	// Identify the audio from its own bytes rather than the client's header.
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(body, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		if isTooLarge(err) {
			h.writeTooLarge(w)
			return
		}
		httpx.WriteError(w, r, h.logger, fmt.Errorf("read upload for track %s: %w", id, err))
		return
	}
	head = head[:n]
	if n == 0 {
		httpx.WriteError(w, r, h.logger, fmt.Errorf("%w: request body is empty", domain.ErrInvalidInput))
		return
	}
	format, ok := sniffAudio(head)
	if !ok {
		httpx.WriteError(w, r, h.logger,
			fmt.Errorf("%w: body is not a recognised audio format", domain.ErrInvalidInput))
		return
	}

	// Hash and count while the body streams past on its way to the blob
	// store, so the bytes are read once. The count is the only trustworthy
	// file_size: the one on createTrackRequest is just the client's claim.
	hasher := sha256.New()
	counter := &countingWriter{}
	content := io.TeeReader(io.MultiReader(bytes.NewReader(head), body), io.MultiWriter(hasher, counter))

	// The key carries a random component so a re-upload can never collide with
	// the object the track currently points at, even when the format (and so
	// the extension) is unchanged. Overwriting that object in place is what let
	// a failed SetTrackFile below delete the track's live audio.
	suffix := make([]byte, 16)
	if _, err := rand.Read(suffix); err != nil {
		httpx.WriteError(w, r, h.logger, fmt.Errorf("generate storage key for track %s: %w", id, err))
		return
	}
	storageKey := fmt.Sprintf("tracks/%s/%s%s", id, hex.EncodeToString(suffix), format.ext)
	// A negative length means "unknown": the BlobStore streams it, which is
	// what makes a chunked upload work.
	size := r.ContentLength
	if size < 0 {
		size = -1
	}
	if err := h.blobs.Put(r.Context(), storageKey, content, size, format.contentType); err != nil {
		if isTooLarge(err) {
			h.writeTooLarge(w)
			return
		}
		httpx.WriteError(w, r, h.logger, fmt.Errorf("upload track file %s: %w", id, err))
		return
	}

	// Cleanup outlives the request: the object is already written, so a client
	// that disconnects here must not leave it behind.
	cleanup := context.WithoutCancel(r.Context())
	contentHash := hex.EncodeToString(hasher.Sum(nil))

	if err := h.trackService.SetTrackFile(cleanup, id, storageKey, contentHash, format.ext[1:], counter.n); err != nil {
		// Nothing points at the object we just wrote, so take it back out.
		// This covers the dedup conflict too: the audio is already stored
		// under another track, and this copy is redundant.
		if delErr := h.blobs.Delete(cleanup, storageKey); delErr != nil {
			h.logger.Error("Orphaned object: upload recorded nowhere and could not be removed",
				"storage_key", storageKey, "error", delErr)
		}
		httpx.WriteError(w, r, h.logger, fmt.Errorf("record file for track %s: %w", id, err))
		return
	}

	// A re-upload always lands under a fresh key (see storageKey above), so a
	// track that already had a file leaves its old object unreferenced now
	// that the row has moved on. The row committed first, so this can only
	// orphan an object, never detach one still referenced.
	if previousKey != "" && previousKey != storageKey {
		if err := h.blobs.Delete(cleanup, previousKey); err != nil {
			h.logger.Error("Orphaned object: replaced track file could not be removed",
				"track_id", id, "storage_key", previousKey, "error", err)
		}
	}

	httpx.RespondNoContent(w)
}

// countingWriter counts bytes written to it, for measuring an upload's true
// size as it streams past rather than trusting whatever the client claimed.
type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// isTooLarge reports whether err is the body cap being hit. MaxBytesReader
// surfaces it from whichever read trips it, which may be deep inside the blob
// store's own copy loop.
func isTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// writeTooLarge answers 413. It is not a domain sentinel because nothing below
// the HTTP layer has an opinion about request size.
func (h *TrackHandler) writeTooLarge(w http.ResponseWriter) {
	httpx.RespondWithError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "upload exceeds the maximum allowed size")
}

// DeleteTrack deletes a track
// @Summary      Delete a track
// @Description  Remove a track from the database by ID
// @Tags         tracks
// @Produce      json
// @Param        id   path      string  true  "Track ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /tracks/{id} [delete]
func (h *TrackHandler) DeleteTrack(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id")
	if err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	if err := h.trackService.DeleteTrack(r.Context(), id); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	httpx.RespondNoContent(w)
}
