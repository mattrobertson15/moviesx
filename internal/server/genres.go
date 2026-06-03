package server

import "net/http"

// handleGenres godoc
// @Summary     List all genres
// @Tags        genres
// @Produce     json
// @Success     200 {array} string
// @Router      /api/genres [get]
func (s *server) handleGenres(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Genres())
}
