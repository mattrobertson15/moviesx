package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mbr/moviesx/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type server struct {
	version string
	store   *store.Store
}

// New registers all routes and returns an HTTP handler.
func New(version string, st *store.Store) http.Handler {
	s := &server{version: version, store: st}
	mux := http.NewServeMux()

	mux.Handle("GET /version", instrumentedHandler("version", http.HandlerFunc(s.handleVersion)))
	mux.Handle("GET /healthz", instrumentedHandler("healthz", http.HandlerFunc(s.handleHealthz)))
	mux.Handle("GET /api/genres", instrumentedHandler("api_genres", http.HandlerFunc(s.handleGenres)))
	mux.Handle("GET /api/movies", instrumentedHandler("api_movies", http.HandlerFunc(s.handleMovies)))
	mux.Handle("GET /api/movies/{id}", instrumentedHandler("api_movies_id", http.HandlerFunc(s.handleMovieByID)))
	mux.Handle("GET /api/actors", instrumentedHandler("api_actors", http.HandlerFunc(s.handleActors)))
	mux.Handle("GET /api/actors/{id}", instrumentedHandler("api_actors_id", http.HandlerFunc(s.handleActorByID)))
	mux.Handle("GET /metrics", promhttp.Handler())

	return loggingMiddleware(mux)
}

func (s *server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, s.version)
}

func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "pass")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
