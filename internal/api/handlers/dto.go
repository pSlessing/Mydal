package handlers

import (
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
