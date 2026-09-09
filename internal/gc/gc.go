// Package gc reclaims blob-store objects the catalogue no longer references,
// and reports catalogue rows whose object is missing.
//
// A delete is "row first, then object" everywhere in this codebase (see
// artistservice.go and trackservice.go), and each of those call sites already
// tries to clean up the object it just orphaned. This sweep exists for the
// case that cleanup misses: a crash between the row commit and the object
// delete, or a bug. Every "Orphaned object" log line elsewhere in the
// codebase is a promise that a sweep like this one exists.
//
// The sweep runs against a live server, so it has to be wrong in the safe
// direction. An upload writes its object before it commits the row that names
// it, which means an object with no row is not proof of an orphan - it may be
// an upload a few milliseconds from committing. Two things keep that upload's
// object: the bucket is listed *before* the catalogue is read, so any row that
// commits during the sweep is still seen; and any object younger than the
// grace window is left alone regardless, which covers the upload that had not
// written its object yet when the listing ran.
package gc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mydal/internal/domain"
	"mydal/internal/storage"
)

// BlobLister is the part of storage.BlobStore the sweep needs. Declared here,
// at the consumer, so the sweep can be tested without MinIO.
type BlobLister interface {
	List(ctx context.Context) ([]storage.ObjectInfo, error)
	Stat(ctx context.Context, key string) (storage.ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

// ReferencedKeys reports every object key the catalogue currently points at -
// track audio and album covers alike. It is a function rather than a slice
// because Sweep must be the one to decide when it is read: after the bucket
// listing, never before.
type ReferencedKeys func(ctx context.Context) ([]string, error)

// Report is what one sweep found.
type Report struct {
	// Orphaned holds every object key present in the bucket that nothing in
	// the catalogue references. Each is deleted immediately unless the sweep
	// runs dry.
	Orphaned []string
	// Missing holds every key the catalogue references that the bucket has no
	// object for - a broken track or album, which this sweep can only report.
	Missing []string
	// Skipped holds every unreferenced object left alone for being younger
	// than the grace window: probably an in-flight upload, and not this
	// sweep's business either way.
	Skipped []string
}

// Sweep compares the bucket's own listing against the keys the catalogue
// references and reports the difference in both directions. With dryRun
// false, every orphan is deleted; a delete failure is collected rather than
// stopping the sweep, so one flaky object cannot hide the rest of the report.
//
// grace is how young an unreferenced object may be and still be presumed
// in-flight rather than orphaned. It must comfortably exceed the longest an
// upload can take (uploadReadTimeout), because that is the window between an
// object being written and its row committing.
func Sweep(ctx context.Context, blobs BlobLister, referenced ReferencedKeys, grace time.Duration, dryRun bool) (Report, error) {
	// The listing comes first: a row that commits after this point is still
	// read below, so its object is never mistaken for an orphan.
	objects, err := blobs.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("list bucket: %w", err)
	}

	keys, err := referenced(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("list referenced keys: %w", err)
	}
	referencedSet := make(map[string]bool, len(keys))
	for _, key := range keys {
		referencedSet[key] = true
	}

	cutoff := time.Now().Add(-grace)

	var report Report
	var errs []error
	present := make(map[string]bool, len(objects))
	for _, obj := range objects {
		present[obj.Key] = true
		if referencedSet[obj.Key] {
			continue
		}
		if obj.LastModified.After(cutoff) {
			report.Skipped = append(report.Skipped, obj.Key)
			continue
		}
		report.Orphaned = append(report.Orphaned, obj.Key)
		if dryRun {
			continue
		}
		if err := blobs.Delete(ctx, obj.Key); err != nil {
			errs = append(errs, fmt.Errorf("delete orphaned object %q: %w", obj.Key, err))
		}
	}

	for _, key := range keys {
		if present[key] {
			continue
		}
		// Absent from a listing taken before the catalogue read is not proof
		// the object is gone: an upload that committed in between wrote its
		// object after the listing. Confirm with a stat before calling a
		// track broken.
		if _, err := blobs.Stat(ctx, key); err == nil {
			continue
		} else if !errors.Is(err, domain.ErrNotFound) {
			errs = append(errs, fmt.Errorf("stat referenced object %q: %w", key, err))
			continue
		}
		report.Missing = append(report.Missing, key)
	}

	return report, errors.Join(errs...)
}
