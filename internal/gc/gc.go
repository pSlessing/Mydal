// Package gc reclaims blob-store objects the catalogue no longer references,
// and reports catalogue rows whose object is missing.
//
// A delete is "row first, then object" everywhere in this codebase (see
// artistservice.go and trackservice.go), and each of those call sites already
// tries to clean up the object it just orphaned. This sweep exists for the
// case that cleanup misses: a crash between the row commit and the object
// delete, or a bug. Every "Orphaned object" log line elsewhere in the
// codebase is a promise that a sweep like this one exists.
package gc

import (
	"context"
	"errors"
	"fmt"
)

// BlobLister is the part of storage.BlobStore the sweep needs. Declared here,
// at the consumer, so the sweep can be tested without MinIO.
type BlobLister interface {
	List(ctx context.Context) ([]string, error)
	Delete(ctx context.Context, key string) error
}

// Report is what one sweep found.
type Report struct {
	// Orphaned holds every object key present in the bucket that no track
	// references. Each is deleted immediately unless the sweep runs dry.
	Orphaned []string
	// Missing holds every storage_key a track references that the bucket has
	// no object for - a broken track, which this sweep can only report.
	Missing []string
}

// Sweep compares the bucket's own listing against storageKeys - every
// storage_key currently recorded in the catalogue - and reports the
// difference in both directions. With dryRun false, every orphan is deleted;
// a delete failure is collected rather than stopping the sweep, so one flaky
// object cannot hide the rest of the report.
func Sweep(ctx context.Context, blobs BlobLister, storageKeys []string, dryRun bool) (Report, error) {
	objects, err := blobs.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("list bucket: %w", err)
	}

	referenced := make(map[string]bool, len(storageKeys))
	for _, key := range storageKeys {
		referenced[key] = true
	}
	present := make(map[string]bool, len(objects))
	for _, key := range objects {
		present[key] = true
	}

	var report Report
	var errs []error
	for _, key := range objects {
		if referenced[key] {
			continue
		}
		report.Orphaned = append(report.Orphaned, key)
		if dryRun {
			continue
		}
		if err := blobs.Delete(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete orphaned object %q: %w", key, err))
		}
	}
	for _, key := range storageKeys {
		if !present[key] {
			report.Missing = append(report.Missing, key)
		}
	}
	return report, errors.Join(errs...)
}
