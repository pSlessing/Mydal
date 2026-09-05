package api

import (
	"log/slog"
	"mydal/src/internal/api/handlers"
	"net/http"

	_ "mydal/src/cmd/server/docs"

	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// APIPrefix versions the API from the start; adding it later means breaking
// every client at once.
const APIPrefix = "/api/v1"

type Router struct {
	mux             *http.ServeMux
	handler         http.Handler
	artistHandler   *handlers.ArtistHandler
	trackHandler    *handlers.TrackHandler
	albumHandler    *handlers.AlbumHandler
	streamHandler   *handlers.StreamHandler
	playlistHandler *handlers.PlaylistHandler
	logger          *slog.Logger
}

func NewRouter(artistHandler *handlers.ArtistHandler, trackHandler *handlers.TrackHandler, albumHandler *handlers.AlbumHandler, streamHandler *handlers.StreamHandler, playlistHandler *handlers.PlaylistHandler, logger *slog.Logger) *Router {
	r := &Router{
		mux:             http.NewServeMux(),
		artistHandler:   artistHandler,
		trackHandler:    trackHandler,
		albumHandler:    albumHandler,
		streamHandler:   streamHandler,
		playlistHandler: playlistHandler,
		logger:          logger,
	}
	r.registerRoutes()

	// The middleware wraps the whole router rather than being attached per
	// route: 404s and 405s are answered by the mux itself and would otherwise
	// carry no request id and never be logged. Outermost first: an id for
	// every request, one log line, and a panic guard that still lands inside
	// the logged request.
	r.handler = RequestID(Logging(logger)(Recover(logger)(r.mux)))
	return r
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("POST "+APIPrefix+"/artists", r.artistHandler.CreateArtist)
	r.mux.HandleFunc("GET "+APIPrefix+"/artists/{id}", r.artistHandler.GetArtist)
	r.mux.HandleFunc("DELETE "+APIPrefix+"/artists/{id}", r.artistHandler.DeleteArtist)

	r.mux.HandleFunc("POST "+APIPrefix+"/tracks", r.trackHandler.CreateTrack)
	r.mux.HandleFunc("GET "+APIPrefix+"/tracks/{id}", r.trackHandler.GetTrack)
	r.mux.HandleFunc("DELETE "+APIPrefix+"/tracks/{id}", r.trackHandler.DeleteTrack)
	r.mux.HandleFunc("PUT "+APIPrefix+"/tracks/{id}/file", r.trackHandler.UploadTrackFile)

	r.mux.HandleFunc("POST "+APIPrefix+"/albums", r.albumHandler.CreateAlbum)
	r.mux.HandleFunc("GET "+APIPrefix+"/albums/{id}", r.albumHandler.GetAlbum)
	r.mux.HandleFunc("DELETE "+APIPrefix+"/albums/{id}", r.albumHandler.DeleteAlbum)

	r.mux.HandleFunc("GET "+APIPrefix+"/tracks/{id}/stream", r.streamHandler.StreamTrack)

	r.mux.HandleFunc("POST "+APIPrefix+"/playlists", r.playlistHandler.CreatePlaylist)
	r.mux.HandleFunc("GET "+APIPrefix+"/playlists/{id}", r.playlistHandler.GetPlaylist)
	r.mux.HandleFunc("DELETE "+APIPrefix+"/playlists/{id}", r.playlistHandler.DeletePlaylist)
	r.mux.HandleFunc("PUT "+APIPrefix+"/playlists/{id}/tracks/{trackId}", r.playlistHandler.AddTrackToPlaylist)
	r.mux.HandleFunc("DELETE "+APIPrefix+"/playlists/{id}/tracks/{trackId}", r.playlistHandler.RemoveTrackFromPlaylist)

	// The docs UI is not part of the versioned API surface.
	r.mux.Handle("GET /swagger/", httpSwagger.Handler())
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}
