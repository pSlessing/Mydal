package main

import (
	"database/sql"
	"mydal/src/internal/api"
	"mydal/src/internal/api/handlers"
	"mydal/src/internal/pkg"
	"mydal/src/internal/repository"
	"mydal/src/internal/service"
	"net/http"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
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
	// For example:
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		return
	}
	defer db.Close()

	// Start the server on cfg.Addr

	//init repo
	artistRepo := repository.NewArtistRepository(db)

	//init service
	artistService := service.NewArtistService(artistRepo)

	//init handlers
	artistHandler := handlers.NewArtistHandler(artistService)

	//init router
	router := api.NewRouter(artistHandler)

	logger.Info("Server is running on " + cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, router); err != nil {
		logger.Error("Failed to start server", "error", err)
	}
}
