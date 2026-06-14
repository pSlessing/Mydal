package api

import (
	"log/slog"
	"mydal/src/internal/api/handlers"
	"net/http"

	_ "mydal/src/cmd/server/docs"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

type Router struct {
	mux           *http.ServeMux
	artistHandler *handlers.ArtistHandler
	trackHandler  *handlers.TrackHandler
	albumHandler  *handlers.AlbumHandler
	streamHandler *handlers.StreamHandler
	logger        *slog.Logger
}

func NewRouter(artistHandler *handlers.ArtistHandler, trackHandler *handlers.TrackHandler, albumHandler *handlers.AlbumHandler, streamHandler *handlers.StreamHandler, logger *slog.Logger) *Router {
	r := &Router{
		mux:           http.NewServeMux(),
		artistHandler: artistHandler,
		trackHandler:  trackHandler,
		albumHandler:  albumHandler,
		streamHandler: streamHandler,
		logger:        logger,
	}
	r.registerRoutes()
	return r
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("POST /artists", r.artistHandler.CreateArtist)
	r.mux.HandleFunc("GET /artists/{id}", r.artistHandler.GetArtist)
	r.mux.HandleFunc("DELETE /artists/{id}", r.artistHandler.DeleteArtist)

	r.mux.HandleFunc("POST /tracks", r.trackHandler.CreateTrack)
	r.mux.HandleFunc("GET /tracks/{id}", r.trackHandler.GetTrack)
	r.mux.HandleFunc("DELETE /tracks/{id}", r.trackHandler.DeleteTrack)
	r.mux.HandleFunc("PUT /tracks/{id}/file", r.trackHandler.UploadTrackFile)

	r.mux.HandleFunc("POST /albums", r.albumHandler.CreateAlbum)
	r.mux.HandleFunc("GET /albums/{id}", r.albumHandler.GetAlbum)
	r.mux.HandleFunc("DELETE /albums/{id}", r.albumHandler.DeleteAlbum)

	r.mux.HandleFunc("GET /tracks/{id}/stream", r.streamHandler.StreamTrack)

	r.mux.Handle("GET /swagger/", httpSwagger.Handler())
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}
