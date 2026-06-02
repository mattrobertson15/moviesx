package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// rawMovie is parsed from movies.json.
type rawMovie struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Year    int      `json:"year"`
	Runtime int      `json:"runtime"`
	Genres  []string `json:"genres"`
	Roles   []Role   `json:"roles"`
}

// rawRating is parsed from ratings.json.
type rawRating struct {
	MovieID string  `json:"movieId"`
	Rating  float64 `json:"rating"`
	Votes   int     `json:"votes"`
}

// Store holds all in-memory data loaded from the data directory.
// It is immutable after Load returns.
type Store struct {
	moviesByID map[string]*Movie
	actorsByID map[string]*Actor
	sortedMovies []*Movie
	sortedActors []*Actor
	genres       []string
}

// Load reads movies.json, actors.json, and ratings.json from dataDir.
// Returns a fully-populated, immutable Store ready for query.
func Load(dataDir string) (*Store, error) {
	rawMovies, err := loadJSON[[]rawMovie](filepath.Join(dataDir, "movies.json"))
	if err != nil {
		return nil, fmt.Errorf("movies.json: %w", err)
	}
	actors, err := loadJSON[[]Actor](filepath.Join(dataDir, "actors.json"))
	if err != nil {
		return nil, fmt.Errorf("actors.json: %w", err)
	}
	rawRatings, err := loadJSON[[]rawRating](filepath.Join(dataDir, "ratings.json"))
	if err != nil {
		return nil, fmt.Errorf("ratings.json: %w", err)
	}

	// Build rating lookup by movieId.
	ratingByMovieID := make(map[string]rawRating, len(rawRatings))
	for _, r := range rawRatings {
		ratingByMovieID[r.MovieID] = r
	}

	s := &Store{
		moviesByID: make(map[string]*Movie, len(rawMovies)),
		actorsByID: make(map[string]*Actor, len(actors)),
	}

	// Enrich movies with pre-joined ratings and build movie index.
	for _, rm := range rawMovies {
		genres := rm.Genres
		if genres == nil {
			genres = []string{}
		}
		roles := rm.Roles
		if roles == nil {
			roles = []Role{}
		}
		m := &Movie{
			ID:      rm.ID,
			Title:   rm.Title,
			Year:    rm.Year,
			Runtime: rm.Runtime,
			Genres:  genres,
			Roles:   roles,
		}
		if r, ok := ratingByMovieID[rm.ID]; ok {
			m.Rating = r.Rating
			m.Votes = r.Votes
		} else {
			slog.Warn("no rating for movie", "id", rm.ID)
		}
		s.moviesByID[rm.ID] = m
		s.sortedMovies = append(s.sortedMovies, m)
	}

	// Sort movies: rating desc, title asc.
	sort.Slice(s.sortedMovies, func(i, j int) bool {
		a, b := s.sortedMovies[i], s.sortedMovies[j]
		if a.Rating != b.Rating {
			return a.Rating > b.Rating
		}
		return a.Title < b.Title
	})

	// Build actor index and sorted list.
	for i := range actors {
		a := &actors[i]
		if a.Profession == nil {
			a.Profession = []string{}
		}
		if a.Movies == nil {
			a.Movies = []ActorMovie{}
		}
		s.actorsByID[a.ID] = a
		s.sortedActors = append(s.sortedActors, a)
	}

	// Sort actors: name asc.
	sort.Slice(s.sortedActors, func(i, j int) bool {
		return s.sortedActors[i].Name < s.sortedActors[j].Name
	})

	// Compute sorted genre list (deduplicated).
	genreSet := make(map[string]struct{})
	for _, rm := range rawMovies {
		for _, g := range rm.Genres {
			genreSet[g] = struct{}{}
		}
	}
	s.genres = make([]string, 0, len(genreSet))
	for g := range genreSet {
		s.genres = append(s.genres, g)
	}
	sort.Strings(s.genres)

	return s, nil
}

// Genres returns the pre-sorted list of distinct genres across all movies.
func (s *Store) Genres() []string {
	out := make([]string, len(s.genres))
	copy(out, s.genres)
	return out
}

// AllMovies returns all movies in stable sort order (rating desc, title asc).
// The returned slice is a copy; mutating it does not affect the store.
func (s *Store) AllMovies() []*Movie {
	out := make([]*Movie, len(s.sortedMovies))
	copy(out, s.sortedMovies)
	return out
}

// Movie returns the movie with the given ID, or (nil, false) if not found.
func (s *Store) Movie(id string) (*Movie, bool) {
	m, ok := s.moviesByID[id]
	return m, ok
}

// AllActors returns all actors in stable sort order (name asc).
// The returned slice is a copy; mutating it does not affect the store.
func (s *Store) AllActors() []*Actor {
	out := make([]*Actor, len(s.sortedActors))
	copy(out, s.sortedActors)
	return out
}

// Actor returns the actor with the given ID, or (nil, false) if not found.
func (s *Store) Actor(id string) (*Actor, bool) {
	a, ok := s.actorsByID[id]
	return a, ok
}

// loadJSON reads a file and unmarshals its JSON into T.
func loadJSON[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, err
	}
	return result, nil
}
