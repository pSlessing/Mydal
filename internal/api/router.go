package api

import (
	"encoding/json"
	"log/slog"
	"mydal/internal/api/handlers"
	"net/http"
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
	healthHandler   *handlers.HealthHandler
	logger          *slog.Logger
}

func NewRouter(artistHandler *handlers.ArtistHandler, trackHandler *handlers.TrackHandler, albumHandler *handlers.AlbumHandler, streamHandler *handlers.StreamHandler, playlistHandler *handlers.PlaylistHandler, healthHandler *handlers.HealthHandler, logger *slog.Logger) *Router {
	r := &Router{
		mux:             http.NewServeMux(),
		artistHandler:   artistHandler,
		trackHandler:    trackHandler,
		albumHandler:    albumHandler,
		streamHandler:   streamHandler,
		playlistHandler: playlistHandler,
		healthHandler:   healthHandler,
		logger:          logger,
	}
	r.registerRoutes()

	// The middleware wraps the whole router rather than being attached per
	// route: 404s and 405s are answered by the mux itself and would otherwise
	// carry no request id and never be logged. Outermost first: an id for
	// every request, one log line, and a panic guard that still lands inside
	// the logged request. jsonMuxErrors sits innermost, right against the mux,
	// since it exists only to rewrite what the mux itself writes.
	r.handler = RequestID(Logging(logger)(Recover(logger)(jsonMuxErrors(r.mux))))
	return r
}

func (r *Router) registerRoutes() {
	// Go 1.22+ ServeMux patterns carry the method, so a known path with an
	// unknown method is answered 405 with an Allow header, and GET also
	// answers HEAD.
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

	// Probes sit outside the versioned API: an orchestrator's healthcheck
	// should not have to be rewritten when the API version moves.
	r.mux.HandleFunc("GET /healthz", r.healthHandler.Healthz)
	r.mux.HandleFunc("GET /readyz", r.healthHandler.Readyz)
}

// Handle registers an extra route directly on the underlying mux, for
// handlers that do not belong to this package - the generated Swagger UI
// being the reason this exists. Keeping that registration out of router.go
// means this library package never imports cmd/server/docs, the entry
// point's own generated code; main.go, which already sits above both, does
// the wiring instead.
func (r *Router) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
}

// muxError is the JSON body for a status the stdlib mux answers on its own,
// without ever reaching a registered handler: 404 for a path matching no
// pattern, 405 for a registered path called with a method no pattern on it
// accepts. code follows the same stable-code contract httpx.WriteError gives
// every other error response.
type muxError struct {
	code, message string
}

var muxErrors = map[int]muxError{
	http.StatusNotFound:         {"not_found", "not found"},
	http.StatusMethodNotAllowed: {"method_not_allowed", "method not allowed"},
}

// jsonMuxErrors rewrites those two plain-text bodies into the same JSON error
// contract every handler uses. A 405 keeps the Allow header the mux already
// set on it.
//
// A registered handler can also answer 404 or 405 (httpx.WriteError does, for
// example), and those must pass through untouched. The two are told apart by
// Content-Type: net/http.Error - the only thing that produces the mux's own
// plain-text errors - always sets it to exactly "text/plain; charset=utf-8"
// before WriteHeader, which no JSON response ever does.
func jsonMuxErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&muxErrorRewriter{ResponseWriter: w}, r)
	})
}

type muxErrorRewriter struct {
	http.ResponseWriter
	rewriting bool
}

func (w *muxErrorRewriter) WriteHeader(status int) {
	muxErr, ok := muxErrors[status]
	if !ok || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Del("X-Content-Type-Options")
	w.ResponseWriter.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{"code": muxErr.code, "error": muxErr.message})
	w.ResponseWriter.Write(body)
	w.rewriting = true
}

func (w *muxErrorRewriter) Write(b []byte) (int, error) {
	if w.rewriting {
		// Discard the mux's own plain-text body; the JSON body already went
		// out in WriteHeader above.
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets a caller further down the chain (http.ServeContent's range and
// flush support, a future http.ResponseController deadline) reach the
// underlying writer instead of stopping at this wrapper.
func (w *muxErrorRewriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}
