// @title           Mydal API
// @version         1.0
// @description     Music library management and streaming API
// @host            localhost:8080
// @BasePath        /api/v1

package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"mydal/internal/api"
	"mydal/internal/api/handlers"
	"mydal/internal/config"
	"mydal/internal/gc"
	"mydal/internal/logging"
	"mydal/internal/repository"
	"mydal/internal/service"
	"mydal/internal/storage"
	"mydal/migrations"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "mydal/cmd/server/docs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

const (
	// Startup work must not hang forever on an unreachable dependency.
	startupTimeout = 10 * time.Second
	// Long enough for in-flight requests, short enough for an orchestrator.
	shutdownTimeout = 30 * time.Second

	maxOpenConns    = 25
	maxIdleConns    = 25
	connMaxLifetime = 5 * time.Minute

	healthcheckTimeout = 3 * time.Second
)

func main() {
	// Bootstrap logger for anything that fails before the configured one exists.
	bootstrap := logging.New("info")
	if err := run(bootstrap); err != nil {
		bootstrap.Error("Startup failed", "error", err)
		os.Exit(1)
	}
}

// probeReady asks the locally running server whether it is ready. It is the
// container healthcheck, so it opens no database or bucket connections of its
// own - it only reports what the running process says about itself.
func probeReady(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse ADDR %q: %w", addr, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: healthcheckTimeout}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/readyz")
	if err != nil {
		return fmt.Errorf("probe /readyz: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not ready: /readyz answered %d", resp.StatusCode)
	}
	return nil
}

func run(bootstrap *slog.Logger) error {
	migrateOnly := flag.Bool("migrate-only", false, "apply database migrations and exit")
	healthcheck := flag.Bool("healthcheck", false, "probe the local server's /readyz and exit 0 if ready")
	runGC := flag.Bool("gc", false, "sweep the bucket for objects the catalogue no longer references, and exit")
	dryRun := flag.Bool("dry-run", false, "with -gc, report what would be deleted without deleting it")
	flag.Parse()

	// In production the variables come from the environment and there is no
	// .env file, so a missing one is not an error.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	// The container image is distroless: no shell, no curl. The binary probes
	// itself so compose and any orchestrator have a healthcheck to run. This
	// needs only ADDR, so it is kept off the full config.Load below, which
	// also requires DATABASE_URL, MINIO_ENDPOINT and BUCKET_NAME - none of
	// which bear on whether the local server is answering.
	if *healthcheck {
		addr, err := config.Addr()
		if err != nil {
			return err
		}
		return probeReady(addr)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)
	logger.Info("Starting server...")
	logger.Debug("Configuration loaded", "config", cfg)

	// Signals cancel this context, which unblocks the wait below.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := openDB(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	// Bring the schema up before anything reads from it.
	if err := migrations.Up(ctx, db, logger); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if *migrateOnly {
		return nil
	}

	minioClient, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		return fmt.Errorf("initialise MinIO client: %w", err)
	}
	if err := ensureBucket(ctx, minioClient, cfg.BucketName, logger); err != nil {
		return err
	}

	//init storage
	blobs := storage.NewMinIOStore(minioClient, cfg.BucketName, cfg.MaxUploadBytes, logger)

	if *runGC {
		return runOrphanSweep(ctx, db, blobs, *dryRun, logger)
	}

	//init repo
	artistRepo := repository.NewArtistRepository(db)
	trackRepo := repository.NewTrackRepository(db)
	albumRepo := repository.NewAlbumRepository(db)
	playlistRepo := repository.NewPlaylistRepository(db)

	//init service
	artistService := service.NewArtistService(artistRepo, blobs, logger)
	trackService := service.NewTrackService(trackRepo, blobs, logger)
	albumService := service.NewAlbumService(albumRepo, logger)
	playlistService := service.NewPlaylistService(playlistRepo, logger)

	//init handlers
	artistHandler := handlers.NewArtistHandler(artistService, logger)
	trackHandler := handlers.NewTrackHandler(trackService, blobs, cfg.MaxUploadBytes, logger)
	albumHandler := handlers.NewAlbumHandler(albumService, logger)
	streamHandler := handlers.NewStreamHandler(trackService, blobs, logger)
	playlistHandler := handlers.NewPlaylistHandler(playlistService, logger)
	healthHandler := handlers.NewHealthHandler(db, blobs, logger)

	//init router
	router := api.NewRouter(artistHandler, trackHandler, albumHandler, streamHandler, playlistHandler, healthHandler, logger)

	// The docs UI is not part of the versioned API surface, and wiring it
	// here - rather than inside internal/api - keeps that library package
	// from depending on cmd/server/docs, the entry point's own generated code.
	router.Handle("GET /swagger/", httpSwagger.Handler())

	return serve(ctx, cfg.Addr, router, logger)
}

// runOrphanSweep lists the bucket, lists every track's storage_key, deletes
// objects with no row unless dryRun, and reports rows whose object is
// missing. Every "Orphaned object" log line elsewhere in the codebase is a
// promise that this exists.
func runOrphanSweep(ctx context.Context, db *sql.DB, blobs storage.BlobStore, dryRun bool, logger *slog.Logger) error {
	storageKeys, err := repository.NewTrackRepository(db).AllStorageKeys(ctx)
	if err != nil {
		return fmt.Errorf("list track storage keys: %w", err)
	}

	report, sweepErr := gc.Sweep(ctx, blobs, storageKeys, dryRun)
	if sweepErr != nil {
		logger.Error("Orphan sweep failed to delete some objects", "error", sweepErr)
	}
	action := "Deleted orphaned object"
	if dryRun {
		action = "Would delete orphaned object (dry run)"
	}
	for _, key := range report.Orphaned {
		logger.Info(action, "storage_key", key)
	}
	for _, key := range report.Missing {
		logger.Warn("Track references a missing object", "storage_key", key)
	}
	logger.Info("Orphan sweep complete",
		"orphaned", len(report.Orphaned), "missing", len(report.Missing), "dry_run", dryRun)
	return sweepErr
}

func openDB(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// ensureBucket creates the configured bucket if it is missing, so a fresh
// `docker compose up` needs no manual MinIO setup.
func ensureBucket(ctx context.Context, client *minio.Client, bucket string, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check bucket %q: %w", bucket, err)
	}
	if exists {
		logger.Debug("Bucket already exists", "bucket", bucket)
		return nil
	}
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket %q: %w", bucket, err)
	}
	logger.Info("Created bucket", "bucket", bucket)
	return nil
}

// serve runs the HTTP server until ctx is cancelled, then drains it.
func serve(ctx context.Context, addr string, handler http.Handler, logger *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout is deliberately unset: streaming a large audio file is
		// a long-lived response and must not be cut off mid-flight.
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("Server is running on " + addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		logger.Info("Shutdown signal received, draining connections")
	}

	// A fresh context: the signal has already cancelled ctx.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("Server stopped")
	return nil
}
