// Package testutil provides the integration harness: a real Postgres with the
// embedded migrations applied, and a real MinIO bucket.
//
// Both are opt-in. A test that needs one calls DB or Blobs, and the test skips
// unless the corresponding endpoint is configured, so `go test ./...` works on
// a machine with no Docker while CI - which already runs both services - gets
// the full suite. `-short` skips them regardless.
//
// This exists because the two worst defects in this codebase, a repository
// querying a column that did not exist and another silently dropping three
// columns, were both invisible to any test that did not talk to the real
// schema.
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"mydal/internal/storage"
	"mydal/migrations"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Quiet is a logger that discards, so a passing test prints nothing.
func Quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// env reads the test-specific variable, falling back to the server's own so
// CI and a local compose stack both work without extra configuration.
func env(testKey, fallbackKey string) string {
	if v := os.Getenv(testKey); v != "" {
		return v
	}
	return os.Getenv(fallbackKey)
}

// DB returns a Postgres pool scoped to a schema of its own, with every
// migration applied inside it.
//
// The isolation is per test, not per run, because `go test ./...` runs
// packages in parallel against one server: a shared set of tables means one
// package truncating while another is mid-assertion. A private schema also
// gives each test its own migration bookkeeping, so the migrations themselves
// are exercised on every call rather than once.
func DB(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dsn := env("TEST_DATABASE_URL", "DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL (or DATABASE_URL) to run integration tests")
	}

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer admin.Close()
	if err := admin.PingContext(context.Background()); err != nil {
		t.Skipf("reach database at %s: %v", dsn, err)
	}

	schema := "mydal_test_" + randomSuffix(t)
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}

	db, err := sql.Open("pgx", withSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatalf("open schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		db.Close()
		dropper, err := sql.Open("pgx", dsn)
		if err != nil {
			return
		}
		defer dropper.Close()
		if _, err := dropper.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE"); err != nil {
			fmt.Fprintf(os.Stderr, "testutil: leaked schema %s: %v\n", schema, err)
		}
	})

	if err := migrations.Up(context.Background(), db, Quiet()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}

// withSearchPath points a DSN at one schema. public stays on the path so
// extension functions such as gen_random_uuid remain resolvable.
func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	q := u.Query()
	q.Set("options", "-c search_path="+schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}

// Blobs creates a bucket of its own for the test and removes it afterwards, so
// concurrent packages cannot see each other's objects. It skips when no
// MinIO is configured.
func Blobs(t *testing.T) storage.BlobStore {
	t.Helper()
	store, _, _ := BlobsWithClient(t)
	return store
}

// BlobsWithClient is Blobs, plus the raw client and bucket name for assertions
// that need to look at the bucket directly - "is it empty?" being the point of
// the storage-cleanup tests.
func BlobsWithClient(t *testing.T) (storage.BlobStore, *minio.Client, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	endpoint := env("TEST_MINIO_ENDPOINT", "MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set TEST_MINIO_ENDPOINT (or MINIO_ENDPOINT) to run storage tests")
	}
	access := envOr("MINIO_ACCESS_KEY", "minioadmin")
	secret := envOr("MINIO_SECRET_KEY", "minioadmin")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: os.Getenv("MINIO_USE_SSL") == "true",
	})
	if err != nil {
		t.Fatalf("create MinIO client: %v", err)
	}

	ctx := context.Background()
	bucket := "mydal-test-" + randomSuffix(t)
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Skipf("reach MinIO at %s: %v", endpoint, err)
	}
	t.Cleanup(func() { removeBucket(client, bucket) })

	// Mirrors config's default MAX_UPLOAD_BYTES; nothing here depends on the
	// exact value, just that part sizing has a realistic cap to work from.
	const testMaxUploadBytes = 1 << 30
	return storage.NewMinIOStore(client, bucket, testMaxUploadBytes, Quiet()), client, bucket
}

// Keys lists everything in the bucket, for assertions about what an operation
// left behind.
func Keys(t *testing.T, client *minio.Client, bucket string) []string {
	t.Helper()
	keys := []string{}
	for obj := range client.ListObjects(context.Background(), bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			t.Fatalf("list bucket: %v", obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random bucket name: %v", err)
	}
	return hex.EncodeToString(b)
}

func removeBucket(client *minio.Client, bucket string) {
	ctx := context.Background()
	for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err == nil {
			_ = client.RemoveObject(ctx, bucket, obj.Key, minio.RemoveObjectOptions{})
		}
	}
	if err := client.RemoveBucket(ctx, bucket); err != nil {
		fmt.Fprintf(os.Stderr, "testutil: leaked bucket %s: %v\n", bucket, err)
	}
}
