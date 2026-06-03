package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/mbr/moviesx/docs"
	"github.com/mbr/moviesx/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type server struct {
	version string
	store   *store.Store
	ready   atomic.Bool
}

// SetReady marks the server as ready to serve traffic.
func (s *server) SetReady() { s.ready.Store(true) }

// New registers all routes and returns the server and an HTTP handler.
func New(version string, st *store.Store) (*server, http.Handler) {
	s := &server{version: version, store: st}
	mux := http.NewServeMux()

	mux.Handle("GET /version", instrumentedHandler("version", http.HandlerFunc(s.handleVersion)))
	mux.Handle("GET /healthz", instrumentedHandler("healthz", http.HandlerFunc(s.handleHealthz)))
	mux.Handle("GET /readyz", instrumentedHandler("readyz", http.HandlerFunc(s.handleReadyz)))
	mux.Handle("GET /api/genres", instrumentedHandler("api_genres", http.HandlerFunc(s.handleGenres)))
	mux.Handle("GET /api/movies", instrumentedHandler("api_movies", http.HandlerFunc(s.handleMovies)))
	mux.Handle("GET /api/movies/{id}", instrumentedHandler("api_movies_id", http.HandlerFunc(s.handleMovieByID)))
	mux.Handle("GET /api/actors", instrumentedHandler("api_actors", http.HandlerFunc(s.handleActors)))
	mux.Handle("GET /api/actors/{id}", instrumentedHandler("api_actors_id", http.HandlerFunc(s.handleActorByID)))
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("GET /swagger/v1/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
	})
	mux.Handle("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/v1/swagger.json"),
	))
	mux.HandleFunc("GET /swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger", http.StatusMovedPermanently)
	})

	return s, loggingMiddleware(mux)
}

// handleVersion godoc
// @Summary     Get server version
// @Tags        system
// @Produce     plain
// @Success     200 {string} string "version string"
// @Router      /version [get]
func (s *server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, s.version)
}

// handleHealthz godoc
// @Summary     Liveness check
// @Tags        system
// @Produce     plain
// @Success     200 {string} string "pass"
// @Router      /healthz [get]
func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "pass")
}

// handleReadyz godoc
// @Summary     Readiness check
// @Tags        system
// @Produce     json
// @Success     200 {object} map[string]string
// @Failure     503 {object} map[string]string
// @Router      /readyz [get]
func (s *server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready", "reason": "store loading"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
