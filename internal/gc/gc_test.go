package gc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"testing"
	"time"

	"mydal/internal/domain"
	"mydal/internal/storage"
)

const testGrace = time.Hour

// stale is old enough that the grace window does not protect it; every object
// a test wants swept is written at this age.
var stale = time.Now().Add(-24 * time.Hour)

type fakeBlobs struct {
	objects   map[string]time.Time // key -> LastModified
	deleted   []string
	deleteErr map[string]error
	// onList runs after List has produced its snapshot, standing in for
	// whatever the live server does between the sweep's two reads.
	onList func()
}

func newFakeBlobs(keys ...string) *fakeBlobs {
	objects := make(map[string]time.Time, len(keys))
	for _, k := range keys {
		objects[k] = stale
	}
	return &fakeBlobs{objects: objects}
}

func (f *fakeBlobs) List(ctx context.Context) ([]storage.ObjectInfo, error) {
	keys := make([]string, 0, len(f.objects))
	for k := range f.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	objects := make([]storage.ObjectInfo, 0, len(keys))
	for _, k := range keys {
		objects = append(objects, storage.ObjectInfo{Key: k, LastModified: f.objects[k]})
	}
	if f.onList != nil {
		f.onList()
	}
	return objects, nil
}

func (f *fakeBlobs) Stat(ctx context.Context, key string) (storage.ObjectInfo, error) {
	modified, ok := f.objects[key]
	if !ok {
		return storage.ObjectInfo{}, fmt.Errorf("object %s: %w", key, domain.ErrNotFound)
	}
	return storage.ObjectInfo{Key: key, LastModified: modified}, nil
}

func (f *fakeBlobs) Delete(ctx context.Context, key string) error {
	if err := f.deleteErr[key]; err != nil {
		return err
	}
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

// refs adapts a fixed set of keys to the ReferencedKeys callback.
func refs(keys ...string) ReferencedKeys {
	return func(context.Context) ([]string, error) { return keys, nil }
}

func TestSweepDeletesOrphansAndReportsMissingRows(t *testing.T) {
	blobs := newFakeBlobs("tracks/a", "tracks/orphan")

	report, err := Sweep(context.Background(), blobs, refs("tracks/a", "tracks/missing"), testGrace, false)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if !slices.Equal(report.Orphaned, []string{"tracks/orphan"}) {
		t.Errorf("Orphaned = %v, want [tracks/orphan]", report.Orphaned)
	}
	if !slices.Equal(report.Missing, []string{"tracks/missing"}) {
		t.Errorf("Missing = %v, want [tracks/missing]", report.Missing)
	}
	if !slices.Equal(blobs.deleted, []string{"tracks/orphan"}) {
		t.Errorf("deleted = %v, want [tracks/orphan]", blobs.deleted)
	}
	if _, ok := blobs.objects["tracks/orphan"]; ok {
		t.Error("orphan still present in the bucket after a non-dry sweep")
	}
	if _, ok := blobs.objects["tracks/a"]; !ok {
		t.Error("a referenced object was deleted")
	}
}

func TestSweepDryRunReportsWithoutDeleting(t *testing.T) {
	blobs := newFakeBlobs("tracks/orphan")

	report, err := Sweep(context.Background(), blobs, refs(), testGrace, true)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if !slices.Equal(report.Orphaned, []string{"tracks/orphan"}) {
		t.Errorf("Orphaned = %v, want [tracks/orphan]", report.Orphaned)
	}
	if len(blobs.deleted) != 0 {
		t.Errorf("dry run deleted %v", blobs.deleted)
	}
	if _, ok := blobs.objects["tracks/orphan"]; !ok {
		t.Error("dry run removed the object from the bucket")
	}
}

// A delete failure on one orphan must not stop the sweep from reporting or
// deleting the rest.
func TestSweepContinuesPastADeleteFailure(t *testing.T) {
	blobs := newFakeBlobs("tracks/bad", "tracks/good")
	blobs.deleteErr = map[string]error{"tracks/bad": errors.New("boom")}

	report, err := Sweep(context.Background(), blobs, refs(), testGrace, false)
	if err == nil {
		t.Fatal("Sweep: want an error for the failed delete")
	}
	if !slices.Contains(report.Orphaned, "tracks/bad") || !slices.Contains(report.Orphaned, "tracks/good") {
		t.Errorf("Orphaned = %v, want both objects reported", report.Orphaned)
	}
	if _, ok := blobs.objects["tracks/good"]; ok {
		t.Error("tracks/good should have been deleted despite the other failure")
	}
}

func TestSweepWithNothingToDoReportsNothing(t *testing.T) {
	blobs := newFakeBlobs("tracks/a")
	report, err := Sweep(context.Background(), blobs, refs("tracks/a"), testGrace, false)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(report.Orphaned) != 0 || len(report.Missing) != 0 || len(report.Skipped) != 0 {
		t.Errorf("report = %+v, want an empty report", report)
	}
}

// An upload writes its object before it commits the row naming it, so a
// freshly written object with no row is far more likely to be an upload
// seconds from committing than an orphan. The sweep used to delete it, and
// the upload then committed a row pointing at nothing.
func TestSweepLeavesAnObjectYoungerThanTheGraceWindow(t *testing.T) {
	blobs := newFakeBlobs()
	blobs.objects["tracks/in-flight"] = time.Now().Add(-time.Minute)

	report, err := Sweep(context.Background(), blobs, refs(), testGrace, false)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if len(report.Orphaned) != 0 {
		t.Errorf("Orphaned = %v, want nothing: the object is younger than the grace window", report.Orphaned)
	}
	if len(blobs.deleted) != 0 {
		t.Errorf("deleted %v, want nothing", blobs.deleted)
	}
	if !slices.Equal(report.Skipped, []string{"tracks/in-flight"}) {
		t.Errorf("Skipped = %v, want [tracks/in-flight]", report.Skipped)
	}
}

// The catalogue must be read after the bucket listing, so a row that commits
// mid-sweep still counts as a reference. Reading it first left the window
// this test pins: the object is listed, the row commits, and the key set the
// sweep compares against was already stale.
func TestSweepReadsTheCatalogueAfterListingTheBucket(t *testing.T) {
	blobs := newFakeBlobs()
	blobs.objects["tracks/committing"] = stale

	// The row commits between List and the catalogue read.
	var committed bool
	blobs.onList = func() { committed = true }
	referenced := func(context.Context) ([]string, error) {
		if committed {
			return []string{"tracks/committing"}, nil
		}
		return nil, nil
	}

	report, err := Sweep(context.Background(), blobs, referenced, testGrace, false)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(report.Orphaned) != 0 {
		t.Errorf("Orphaned = %v, want nothing: the row committed before the keys were read", report.Orphaned)
	}
	if len(blobs.deleted) != 0 {
		t.Errorf("deleted %v, want nothing", blobs.deleted)
	}
}

// The mirror of the case above: a key that commits after the listing names an
// object the listing could not have seen, which is not the same as a broken
// track. Confirm with a stat before reporting it missing.
func TestSweepStatsBeforeCallingAReferencedObjectMissing(t *testing.T) {
	blobs := newFakeBlobs()
	blobs.onList = func() { blobs.objects["tracks/just-uploaded"] = time.Now() }

	report, err := Sweep(context.Background(), blobs, refs("tracks/just-uploaded"), testGrace, false)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(report.Missing) != 0 {
		t.Errorf("Missing = %v, want nothing: the object exists, it was just written after the listing", report.Missing)
	}
}

// Covers are referenced too. Sweeping against tracks alone would delete every
// cover object in the bucket the first time one is uploaded.
func TestSweepKeepsObjectsReferencedByAnythingInTheCatalogue(t *testing.T) {
	blobs := newFakeBlobs("tracks/a.flac", "covers/album-1")

	report, err := Sweep(context.Background(), blobs, refs("tracks/a.flac", "covers/album-1"), testGrace, false)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(report.Orphaned) != 0 {
		t.Errorf("Orphaned = %v, want nothing", report.Orphaned)
	}
}
