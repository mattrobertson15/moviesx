package server

import (
	"net/http"
	"strings"

	"github.com/mbr/moviesx/internal/store"
	"github.com/mbr/moviesx/internal/validate"
)

// handleMovies godoc
// @Summary     List movies
// @Tags        movies
// @Produce     json
// @Param       q          query  string  false  "Title search substring (min 2 chars)"
// @Param       genre      query  string  false  "Genre filter"
// @Param       year       query  int     false  "Release year"
// @Param       rating     query  number  false  "Minimum rating"
// @Param       actorId    query  string  false  "Filter by actor ID"
// @Param       pageNumber query  int     false  "Page number (default 1)"
// @Param       pageSize   query  int     false  "Page size (default 20, max 100)"
// @Success     200 {object} store.Page[store.MovieListItem]
// @Failure     400 {object} map[string]string
// @Router      /api/movies [get]
func (s *server) handleMovies(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if _, err := validate.Q(q); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	year, err := validate.Year(r.URL.Query().Get("year"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pageNum, err := validate.PageNumber(r.URL.Query().Get("pageNumber"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pgSize, err := validate.PageSize(r.URL.Query().Get("pageSize"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := validate.Rating(r.URL.Query().Get("rating")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := validate.ActorID(r.URL.Query().Get("actorId")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	genre := r.URL.Query().Get("genre")
	// rating and actorId filters are in the Parking Lot (not implemented in 0.2.0).

	movies := s.store.AllMovies()
	filtered := make([]*store.Movie, 0, len(movies))
	ql := strings.ToLower(q)
	for _, m := range movies {
		if q != "" && !strings.Contains(strings.ToLower(m.Title), ql) {
			continue
		}
		if genre != "" && !containsGenre(m.Genres, genre) {
			continue
		}
		if year != 0 && m.Year != year {
			continue
		}
		filtered = append(filtered, m)
	}

	total := len(filtered)
	start := (pageNum - 1) * pgSize
	if start > total {
		start = total
	}
	end := start + pgSize
	if end > total {
		end = total
	}

	items := make([]store.MovieListItem, end-start)
	for i, m := range filtered[start:end] {
		items[i] = toListItem(m)
	}

	writeJSON(w, http.StatusOK, store.Page[store.MovieListItem]{
		Items:    items,
		Total:    total,
		Page:     pageNum,
		PageSize: pgSize,
	})
}

// handleMovieByID godoc
// @Summary     Get movie by ID
// @Tags        movies
// @Produce     json
// @Param       id  path  string  true  "Movie ID (e.g. tt0000001)"
// @Success     200 {object} store.MovieDetail
// @Failure     400 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Router      /api/movies/{id} [get]
func (s *server) handleMovieByID(w http.ResponseWriter, r *http.Request) {
	id, err := validate.MovieID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	m, ok := s.store.Movie(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, toDetail(m))
}

func containsGenre(genres []string, filter string) bool {
	for _, g := range genres {
		if strings.EqualFold(g, filter) {
			return true
		}
	}
	return false
}

func toListItem(m *store.Movie) store.MovieListItem {
	cast := make([]store.Role, 0)
	for _, r := range m.Roles {
		if r.Category == "actor" || r.Category == "actress" {
			cast = append(cast, r)
		}
	}
	genres := m.Genres
	if genres == nil {
		genres = []string{}
	}
	return store.MovieListItem{
		ID:      m.ID,
		Title:   m.Title,
		Year:    m.Year,
		Runtime: m.Runtime,
		Genres:  genres,
		Cast:    cast,
		Rating:  m.Rating,
		Votes:   m.Votes,
	}
}

func toDetail(m *store.Movie) store.MovieDetail {
	roles := m.Roles
	if roles == nil {
		roles = []store.Role{}
	}
	genres := m.Genres
	if genres == nil {
		genres = []string{}
	}
	return store.MovieDetail{
		ID:      m.ID,
		Title:   m.Title,
		Year:    m.Year,
		Runtime: m.Runtime,
		Genres:  genres,
		Roles:   roles,
		Rating:  m.Rating,
		Votes:   m.Votes,
	}
}
