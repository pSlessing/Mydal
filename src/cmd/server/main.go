package main

import (
	"database/sql"
	"mydal/src/internal/api"
	"mydal/src/internal/api/handlers"
	"mydal/src/internal/pkg"
	"mydal/src/internal/repository"
	"mydal/src/internal/service"
	"net/http"
	"net/url"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	var logger = pkg.New("debug")
	logger.Info("Starting server...")

	// Load configuration
	err := godotenv.Load()
	if err != nil {
		logger.Error("Failed to load .env file", "error", err)
		return
	}
	cfg := pkg.Load()
	logger.Debug("Configuration loaded", "config", cfg)
	// Initialize database connection, etc. here using cfg.DatabaseURL
	var db *sql.DB

	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		logger.Error("Failed to parse database URL", "error", err)
		return
	}
	if cfg.Mode != "production" {
		q := u.Query()
		q.Set("sslmode", "disable")
		u.RawQuery = q.Encode()
	}
	dbURL := u.String()

	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		return
	}
	if err := db.Ping(); err != nil {
		logger.Error("Failed to ping database", "error", err)
		return
	}

	defer db.Close()

	// Start the server on cfg.Addr

	// Initialize MinIO client
	minioClient, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		logger.Error("Failed to initialize MinIO client", "error", err)
		return
	}

	//init repo
	artistRepo := repository.NewArtistRepository(db, logger)
	trackRepo := repository.NewTrackRepository(db, logger)
	minioRepo := repository.NewMiniorepo(minioClient, logger, cfg.BucketName)
	albumRepo := repository.NewAlbumRepository(db, logger)

	//init service
	artistService := service.NewArtistService(artistRepo, logger)
	minioService := service.NewMinioservice(minioRepo, logger)
	trackService := service.NewTrackService(trackRepo, logger)
	albumService := service.NewAlbumService(albumRepo, logger)

	//init handlers
	artistHandler := handlers.NewArtistHandler(artistService, logger)
	trackHandler := handlers.NewTrackHandler(trackService, minioService, logger)
	albumHandler := handlers.NewAlbumHandler(albumService, logger)
	streamHandler := handlers.NewStreamHandler(trackService, minioService, logger)

	//init router
	router := api.NewRouter(artistHandler, trackHandler, albumHandler, streamHandler, logger)

	logger.Info("Server is running on " + cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, router); err != nil {
		logger.Error("Failed to start server", "error", err)
	}
}
