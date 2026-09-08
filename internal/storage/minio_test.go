package storage_test

import (
	"context"
	"errors"
	"testing"

	"mydal/internal/domain"
	"mydal/internal/testutil"
)

// notFound used to treat any 404 - including NoSuchBucket - as a missing key,
// so a deleted bucket answered domain.ErrNotFound just like a missing object
// would. A caller (the streaming and get-track handlers) maps ErrNotFound to
// a 404 "not found", which is the wrong answer for an outage: it should be a
// 500, loud enough to get attention, not a quiet 404 that looks like the
// track was deleted.
func TestMissingBucketIsNotReportedAsNotFound(t *testing.T) {
	blobs, client, bucket := testutil.BlobsWithClient(t)
	ctx := context.Background()

	if err := client.RemoveBucket(ctx, bucket); err != nil {
		t.Fatalf("remove bucket: %v", err)
	}

	_, err := blobs.Stat(ctx, "tracks/whatever.flac")
	if err == nil {
		t.Fatal("Stat against a deleted bucket: want an error")
	}
	if errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a deleted bucket was classified as ErrNotFound (a missing key), not an outage: %v", err)
	}
}

// The behaviour the fix must not disturb: a real missing key is still
// ErrNotFound.
func TestMissingKeyIsStillReportedAsNotFound(t *testing.T) {
	blobs := testutil.Blobs(t)

	_, err := blobs.Stat(context.Background(), "tracks/does-not-exist.flac")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Stat on a missing key = %v, want ErrNotFound", err)
	}
}

// Ping is what the readiness probe relies on to tell "MinIO is reachable and
// the configured bucket exists" apart from every other kind of failure.
func TestPingSucceedsWhenTheBucketExists(t *testing.T) {
	blobs := testutil.Blobs(t)

	if err := blobs.Ping(context.Background()); err != nil {
		t.Fatalf("Ping against an existing bucket: %v", err)
	}
}

func TestPingFailsWhenTheBucketIsGone(t *testing.T) {
	blobs, client, bucket := testutil.BlobsWithClient(t)
	ctx := context.Background()

	if err := client.RemoveBucket(ctx, bucket); err != nil {
		t.Fatalf("remove bucket: %v", err)
	}

	if err := blobs.Ping(ctx); err == nil {
		t.Fatal("Ping against a deleted bucket: want an error")
	}
}
