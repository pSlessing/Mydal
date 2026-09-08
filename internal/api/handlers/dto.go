package handlers

import (
	"fmt"
	"mydal/internal/domain"
	"time"
)

// Request and response types are deliberately separate from the domain
// structs: the wire format is an API contract that should not shift every time
// a domain field is added, and decoding straight into a domain struct lets a
// client set server-owned fields such as ID.

type createArtistRequest struct {
	Name string `json:"name"`
	Bio  string `json:"bio"`
}

type artistResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Bio       string    `json:"bio"`
	CreatedAt time.Time `json:"created_at"`
}

func newArtistResponse(a *domain.Artist) artistResponse {
	return artistResponse{
		ID:        a.ID,
		Name:      a.Name,
		Bio:       a.Bio,
		CreatedAt: a.CreatedAt,
	}
}

// dateLayout is the wire format for a bare date, matching the DATE columns in
// the schema. RFC3339 would carry a time and a zone that a release date does
// not have.
const dateLayout = "2006-01-02"

type createTrackRequest struct {
	Title       string `json:"title"`
	ArtistID    string `json:"artist_id"`
	AlbumID     string `json:"album_id"`
	DurationMs  int64  `json:"duration_ms"`
	Bitrate     int    `json:"bitrate"`
	Format      string `json:"format"`
	FileSize    int64  `json:"file_size"`
	TrackNumber int    `json:"track_number"`
	DiscNumber  int    `json:"disc_number"`
	// StorageKey is deliberately absent. It names an object in the bucket and
	// is set only by the upload endpoint; accepting it from a client would let
	// a caller point a track row at any object already stored.
}

func (r createTrackRequest) toDomain() domain.Track {
	return domain.Track{
		Title:       r.Title,
		ArtistID:    r.ArtistID,
		AlbumID:     r.AlbumID,
		Duration:    time.Duration(r.DurationMs) * time.Millisecond,
		Bitrate:     r.Bitrate,
		Format:      r.Format,
		FileSize:    r.FileSize,
		TrackNumber: r.TrackNumber,
		DiscNumber:  r.DiscNumber,
	}
}

type trackResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ArtistID    string    `json:"artist_id"`
	AlbumID     string    `json:"album_id,omitempty"`
	DurationMs  int64     `json:"duration_ms"`
	Bitrate     int       `json:"bitrate"`
	Format      string    `json:"format"`
	FileSize    int64     `json:"file_size"`
	TrackNumber int       `json:"track_number"`
	DiscNumber  int       `json:"disc_number"`
	StorageKey  string    `json:"storage_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func newTrackResponse(t *domain.Track) trackResponse {
	return trackResponse{
		ID:          t.ID,
		Title:       t.Title,
		ArtistID:    t.ArtistID,
		AlbumID:     t.AlbumID,
		DurationMs:  t.Duration.Milliseconds(),
		Bitrate:     t.Bitrate,
		Format:      t.Format,
		FileSize:    t.FileSize,
		TrackNumber: t.TrackNumber,
		DiscNumber:  t.DiscNumber,
		StorageKey:  t.StorageKey,
		CreatedAt:   t.CreatedAt,
	}
}

type createAlbumRequest struct {
	Title    string `json:"title"`
	ArtistID string `json:"artist_id"`
	// ReleaseDate is optional and, being a DATE, carries no time or zone.
	ReleaseDate *string `json:"release_date"`
	// CoverKey is deliberately absent, for the same reason as StorageKey: it
	// names an object in the bucket and belongs to the cover upload path.
}

func (r createAlbumRequest) toDomain() (domain.Album, error) {
	album := domain.Album{Title: r.Title, ArtistID: r.ArtistID}
	if r.ReleaseDate != nil && *r.ReleaseDate != "" {
		parsed, err := time.Parse(dateLayout, *r.ReleaseDate)
		if err != nil {
			return domain.Album{}, fmt.Errorf("%w: release_date must be %s", domain.ErrInvalidInput, dateLayout)
		}
		album.ReleaseDate = &parsed
	}
	return album, nil
}

type albumResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ArtistID    string    `json:"artist_id"`
	ReleaseDate *string   `json:"release_date"`
	CoverKey    string    `json:"cover_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func newAlbumResponse(a *domain.Album) albumResponse {
	resp := albumResponse{
		ID:        a.ID,
		Title:     a.Title,
		ArtistID:  a.ArtistID,
		CoverKey:  a.CoverKey,
		CreatedAt: a.CreatedAt,
	}
	if a.ReleaseDate != nil {
		formatted := a.ReleaseDate.Format(dateLayout)
		resp.ReleaseDate = &formatted
	}
	return resp
}

type createPlaylistRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	TrackIDs    []string `json:"track_ids"`
}

func (r createPlaylistRequest) toDomain() domain.Playlist {
	return domain.Playlist{
		Title:       r.Title,
		Description: r.Description,
		TrackIDs:    r.TrackIDs,
	}
}

type playlistResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	TrackIDs    []string  `json:"track_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newPlaylistResponse(p *domain.Playlist) playlistResponse {
	trackIDs := p.TrackIDs
	if trackIDs == nil {
		// An empty playlist is [] on the wire, never null.
		trackIDs = []string{}
	}
	return playlistResponse{
		ID:          p.ID,
		Title:       p.Title,
		Description: p.Description,
		TrackIDs:    trackIDs,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}
