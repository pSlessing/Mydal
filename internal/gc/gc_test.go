package gc

import (
	"context"
	"errors"
	"slices"
	"sort"
	"testing"
)

type fakeBlobs struct {
	objects   map[string]bool
	deleted   []string
	deleteErr map[string]error
}

func newFakeBlobs(keys ...string) *fakeBlobs {
	objects := make(map[string]bool, len(keys))
	for _, k := range keys {
		objects[k] = true
	}
	return &fakeBlobs{objects: objects}
}

func (f *fakeBlobs) List(ctx context.Context) ([]string, error) {
	keys := make([]string, 0, len(f.objects))
	for k := range f.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (f *fakeBlobs) Delete(ctx context.Context, key string) error {
	if err := f.deleteErr[key]; err != nil {
		return err
	}
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

func TestSweepDeletesOrphansAndReportsMissingRows(t *testing.T) {
	blobs := newFakeBlobs("tracks/a", "tracks/orphan")
	storageKeys := []string{"tracks/a", "tracks/missing"}

	report, err := Sweep(context.Background(), blobs, storageKeys, false)
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
	if blobs.objects["tracks/orphan"] {
		t.Error("orphan still present in the bucket after a non-dry sweep")
	}
	if !blobs.objects["tracks/a"] {
		t.Error("a referenced object was deleted")
	}
}

func TestSweepDryRunReportsWithoutDeleting(t *testing.T) {
	blobs := newFakeBlobs("tracks/orphan")

	report, err := Sweep(context.Background(), blobs, nil, true)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if !slices.Equal(report.Orphaned, []string{"tracks/orphan"}) {
		t.Errorf("Orphaned = %v, want [tracks/orphan]", report.Orphaned)
	}
	if len(blobs.deleted) != 0 {
		t.Errorf("dry run deleted %v", blobs.deleted)
	}
	if !blobs.objects["tracks/orphan"] {
		t.Error("dry run removed the object from the bucket")
	}
}

// A delete failure on one orphan must not stop the sweep from reporting or
// deleting the rest.
func TestSweepContinuesPastADeleteFailure(t *testing.T) {
	blobs := newFakeBlobs("tracks/bad", "tracks/good")
	blobs.deleteErr = map[string]error{"tracks/bad": errors.New("boom")}

	report, err := Sweep(context.Background(), blobs, nil, false)
	if err == nil {
		t.Fatal("Sweep: want an error for the failed delete")
	}
	if !slices.Contains(report.Orphaned, "tracks/bad") || !slices.Contains(report.Orphaned, "tracks/good") {
		t.Errorf("Orphaned = %v, want both objects reported", report.Orphaned)
	}
	if blobs.objects["tracks/good"] {
		t.Error("tracks/good should have been deleted despite the other failure")
	}
}

func TestSweepWithNothingToDoReportsNothing(t *testing.T) {
	blobs := newFakeBlobs("tracks/a")
	report, err := Sweep(context.Background(), blobs, []string{"tracks/a"}, false)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(report.Orphaned) != 0 || len(report.Missing) != 0 {
		t.Errorf("report = %+v, want an empty report", report)
	}
}
