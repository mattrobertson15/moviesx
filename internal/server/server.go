package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mbr/moviesx/internal/store"
)

type server struct {
	version string
	store   *store.Store
}

// New registers all routes and returns the HTTP mux.
func New(version string, st *store.Store) *http.ServeMux {
	s := &server{version: version, store: st}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /version", s.handleVersion)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/genres", s.handleGenres)
	mux.HandleFunc("GET /api/movies", s.handleMovies)
	mux.HandleFunc("GET /api/movies/{id}", s.handleMovieByID)
	mux.HandleFunc("GET /api/actors", s.handleActors)
	mux.HandleFunc("GET /api/actors/{id}", s.handleActorByID)

	return mux
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
