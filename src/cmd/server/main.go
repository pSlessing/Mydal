package server

import (
	"database/sql"
	"mydal/src/internal/pkg"
)

func main() {
	var logger = pkg.New("debug")
	logger.Info("Starting server...")

	// Load configuration
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
	logger.Info("Server is running", "address", cfg.Addr)

}
