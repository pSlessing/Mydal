package migrations_test

import (
	"context"
	"testing"

	"mydal/internal/testutil"
	"mydal/migrations"
)

// Down existed only for tests and local resets, per its own doc comment, and
// had none of its own: nothing verified it actually reverses what Up applies.
func TestDownReversesEveryTableUpCreates(t *testing.T) {
	db := testutil.DB(t) // testutil.DB already runs Up.
	quiet := testutil.Quiet()

	if err := migrations.Down(context.Background(), db, quiet); err != nil {
		t.Fatalf("Down: %v", err)
	}

	for _, table := range []string{"artists", "albums", "tracks", "playlists", "playlist_tracks"} {
		var count int
		err := db.QueryRow(
			`SELECT count(*) FROM information_schema.tables
			 WHERE table_schema = current_schema() AND table_name = $1`, table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("table %s survived Down", table)
		}
	}
}
