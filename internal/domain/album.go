package domain

import "time"

type Album struct {
	ID       string
	Title    string
	ArtistID string
	// ReleaseDate is nullable: the schema stores a full DATE, and an album
	// whose release date is unknown is a normal state rather than a zero year.
	ReleaseDate *time.Time
	CoverKey    string
	CreatedAt   time.Time
}
