package api

import (
	"log/slog"
	"mydal/src/internal/api/handlers" // Adjust to match your go.mod module path
	"net/http"

	"github.com/gorilla/mux" // Add this import after installing
)

type Router struct {
	Mux           *mux.Router
	artistHandler *handlers.ArtistHandler
	Logger        *slog.Logger
	// Add other handlers: albumHandler, playlistHandler, etc.
}

func NewRouter(artistHandler *handlers.ArtistHandler, logger *slog.Logger /*, other handlers */) *Router {
	r := &Router{
		Mux:           mux.NewRouter(),
		artistHandler: artistHandler,
		Logger:        logger,
		// Initialize other handlers
	}
	//Which port
	r.registerRoutes()
	return r
}

func (r *Router) registerRoutes() {
	// Artist routes
	r.Mux.HandleFunc("/artists", r.artistHandler.CreateArtist).Methods("POST")
	r.Mux.HandleFunc("/artists/{id}", r.artistHandler.GetArtist).Methods("GET")
	r.Mux.HandleFunc("/artists/{id}", r.artistHandler.DeleteArtist).Methods("DELETE")

	// Add similar routes for albums, playlists, tracks, etc.
	// Example: r.Mux.HandleFunc("/albums", r.albumHandler.GetAlbums).Methods("GET")

	// Optional: Add middleware (e.g., logging)
	r.Mux.Use(loggingMiddleware)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.Mux.ServeHTTP(w, req)
}

// Example middleware (implement as needed)
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log request details
		next.ServeHTTP(w, r)
	})
}
