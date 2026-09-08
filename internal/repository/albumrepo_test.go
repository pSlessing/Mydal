package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"mydal/internal/domain"
	"mydal/internal/testutil"
)

// AlbumRepository only ever touched id, title and artist_id. A client could
// post a release date, see it echoed back in the 201, and never find it again;
// cover_key was in the schema and the domain struct but read and written
// nowhere.
func TestAlbumRoundTripsEveryColumn(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	quiet := testutil.Quiet()
	repo := NewAlbumRepository(db, quiet)
	artistID := seedArtist(t, NewArtistRepository(db, quiet))

	released := time.Date(1979, 8, 17, 0, 0, 0, 0, time.UTC)
	album := &domain.Album{
		Title:       "Unknown Pleasures",
		ArtistID:    artistID,
		ReleaseDate: &released,
		CoverKey:    "covers/up.jpg",
	}
	if err := repo.CreateAlbum(ctx, album); err != nil {
		t.Fatalf("create: %v", err)
	}
	if album.ID == "" || album.CreatedAt.IsZero() {
		t.Fatalf("create did not populate id/created_at: %+v", album)
	}

	got, err := repo.GetAlbumByID(ctx, album.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != album.Title || got.ArtistID != artistID {
		t.Fatalf("title/artist lost: %+v", got)
	}
	if got.ReleaseDate == nil || !got.ReleaseDate.Equal(released) {
		t.Fatalf("release_date = %v, want %v", got.ReleaseDate, released)
	}
	if got.CoverKey != "covers/up.jpg" {
		t.Fatalf("cover_key = %q", got.CoverKey)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("created_at not read back")
	}
}

// An unknown release date is a normal state, not a zero year.
func TestAlbumReleaseDateIsNullable(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	quiet := testutil.Quiet()
	repo := NewAlbumRepository(db, quiet)
	artistID := seedArtist(t, NewArtistRepository(db, quiet))

	album := &domain.Album{Title: "Untitled", ArtistID: artistID}
	if err := repo.CreateAlbum(ctx, album); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetAlbumByID(ctx, album.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReleaseDate != nil {
		t.Fatalf("null release_date came back as %v", got.ReleaseDate)
	}
	if got.CoverKey != "" {
		t.Fatalf("default cover_key = %q", got.CoverKey)
	}
}

// GetAlbumByID did not map sql.ErrNoRows, so a missing album was an
// unclassified error.
func TestAlbumMissingIsNotFound(t *testing.T) {
	db := testutil.DB(t)
	ctx := context.Background()
	repo := NewAlbumRepository(db, testutil.Quiet())
	missing := "00000000-0000-0000-0000-000000000000"

	if _, err := repo.GetAlbumByID(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("get = %v, want ErrNotFound", err)
	}
	if err := repo.DeleteAlbum(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("delete = %v, want ErrNotFound", err)
	}
}
