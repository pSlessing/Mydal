// @title           Mydal API
// @version         1.0
// @description     Music library management and streaming API
// @host            localhost:8080
// @BasePath        /

package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"mydal/src/internal/api"
	"mydal/src/internal/api/handlers"
	"mydal/src/internal/pkg"
	"mydal/src/internal/repository"
	"mydal/src/internal/service"
	"mydal/src/migrations"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	// Startup work must not hang forever on an unreachable dependency.
	startupTimeout = 10 * time.Second
	// Long enough for in-flight requests, short enough for an orchestrator.
	shutdownTimeout = 30 * time.Second

	maxOpenConns    = 25
	maxIdleConns    = 25
	connMaxLifetime = 5 * time.Minute
)

func main() {
	// Bootstrap logger for anything that fails before the configured one exists.
	bootstrap := pkg.New("info")
	if err := run(bootstrap); err != nil {
		bootstrap.Error("Startup failed", "error", err)
		os.Exit(1)
	}
}

func run(bootstrap *slog.Logger) error {
	migrateOnly := flag.Bool("migrate-only", false, "apply database migrations and exit")
	flag.Parse()

	// In production the variables come from the environment and there is no
	// .env file, so a missing one is not an error.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	cfg, err := pkg.Load()
	if err != nil {
		return err
	}
	logger := pkg.New(cfg.LogLevel)
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

	//init repo
	artistRepo := repository.NewArtistRepository(db, logger)
	trackRepo := repository.NewTrackRepository(db, logger)
	minioRepo := repository.NewMiniorepo(minioClient, logger, cfg.BucketName)
	albumRepo := repository.NewAlbumRepository(db, logger)
	playlistRepo := repository.NewPlaylistRepository(db, logger)

	//init service
	artistService := service.NewArtistService(artistRepo, logger)
	minioService := service.NewMinioservice(minioRepo, logger)
	trackService := service.NewTrackService(trackRepo, logger)
	albumService := service.NewAlbumService(albumRepo, logger)
	playlistService := service.NewPlaylistService(playlistRepo, logger)

	//init handlers
	artistHandler := handlers.NewArtistHandler(artistService, logger)
	trackHandler := handlers.NewTrackHandler(trackService, minioService, logger)
	albumHandler := handlers.NewAlbumHandler(albumService, logger)
	streamHandler := handlers.NewStreamHandler(trackService, minioService, logger)
	playlistHandler := handlers.NewPlaylistHandler(playlistService, logger)

	//init router
	router := api.NewRouter(artistHandler, trackHandler, albumHandler, streamHandler, playlistHandler, logger)

	return serve(ctx, cfg.Addr, router, logger)
}

func openDB(ctx context.Context, cfg pkg.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DatabaseURL)
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
