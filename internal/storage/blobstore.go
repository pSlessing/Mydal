// Package storage holds the blob store behind the audio files and cover art.
// It is named for what it does rather than which vendor does it, so a
// local-filesystem implementation can sit alongside the MinIO one - which
// matters for a self-hosted music platform pointed at a directory of files.
package storage

import (
	"context"
	"io"
	"time"
)

// ObjectInfo is the metadata a caller needs without fetching the body.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// BlobStore stores and serves opaque objects by key.
type BlobStore interface {
	// Put writes an object. A negative size means the length is unknown and
	// the reader is streamed.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get opens an object for reading and returns its metadata alongside it,
	// so a caller that needs both (streaming, which sets headers from the
	// metadata before serving the body) does it in one round trip rather than
	// a Stat followed by a Get. The caller closes it. Every implementation is
	// expected to return a seekable reader - range requests need one, and
	// every store this interface has, or is expected to gain, is - so this
	// is not io.ReadCloser.
	Get(ctx context.Context, key string) (io.ReadSeekCloser, ObjectInfo, error)
	// Stat returns an object's metadata.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
	// List returns every object currently in the store, for the orphan sweep
	// to compare against the catalogue. It returns metadata rather than bare
	// keys because the sweep needs LastModified to tell an object written
	// seconds ago - by an upload whose row has not committed yet - from one
	// that has been unreferenced since a crash.
	List(ctx context.Context) ([]ObjectInfo, error)
	// Ping reports whether the store is reachable and the bucket usable. It
	// backs the readiness probe, so it must be cheap.
	Ping(ctx context.Context) error
	// PresignedGetURL mints a temporary direct download URL.
	PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}
