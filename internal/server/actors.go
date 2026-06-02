package server

import (
	"net/http"
	"strings"

	"github.com/mbr/moviesx/internal/store"
	"github.com/mbr/moviesx/internal/validate"
)

func (s *server) handleActors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if _, err := validate.Q(q); err != nil {
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

	actors := s.store.AllActors()
	filtered := make([]*store.Actor, 0, len(actors))
	ql := strings.ToLower(q)
	for _, a := range actors {
		if q != "" && !strings.Contains(strings.ToLower(a.Name), ql) {
			continue
		}
		filtered = append(filtered, a)
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

	items := make([]*store.Actor, end-start)
	copy(items, filtered[start:end])

	writeJSON(w, http.StatusOK, store.Page[*store.Actor]{
		Items:    items,
		Total:    total,
		Page:     pageNum,
		PageSize: pgSize,
	})
}

func (s *server) handleActorByID(w http.ResponseWriter, r *http.Request) {
	id, err := validate.ActorPathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, ok := s.store.Actor(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}
