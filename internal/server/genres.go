package server

import "net/http"

func (s *server) handleGenres(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Genres())
}
