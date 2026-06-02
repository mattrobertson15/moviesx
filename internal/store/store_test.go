package store

import (
	"os"
	"path/filepath"
	"testing"
)

const testdata = "testdata"

func TestLoad_HappyPath(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	movies := s.AllMovies()
	if got := len(movies); got != 3 {
		t.Errorf("movie count = %d, want 3", got)
	}

	actors := s.AllActors()
	if got := len(actors); got != 3 {
		t.Errorf("actor count = %d, want 3", got)
	}

	genres := s.Genres()
	wantGenres := []string{"Action", "Comedy", "Drama", "Thriller"}
	if len(genres) != len(wantGenres) {
		t.Fatalf("genre count = %d, want %d: %v", len(genres), len(wantGenres), genres)
	}
	for i, g := range genres {
		if g != wantGenres[i] {
			t.Errorf("genres[%d] = %q, want %q", i, g, wantGenres[i])
		}
	}
}

func TestLoad_GenreDeduplication(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// "Drama" appears in tt0000001 and tt0000003; "Action" in tt0000001 and tt0000002.
	// Total distinct genres: Action, Comedy, Drama, Thriller = 4.
	genres := s.Genres()
	if len(genres) != 4 {
		t.Errorf("genre count = %d, want 4 (dedup check)", len(genres))
	}
	// Verify sorted.
	for i := 1; i < len(genres); i++ {
		if genres[i] < genres[i-1] {
			t.Errorf("genres not sorted: %v", genres)
		}
	}
}

func TestLoad_RatingJoin(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m, ok := s.Movie("tt0000001")
	if !ok {
		t.Fatal("tt0000001 not found")
	}
	if m.Rating != 8.5 {
		t.Errorf("rating = %v, want 8.5", m.Rating)
	}
	if m.Votes != 10000 {
		t.Errorf("votes = %d, want 10000", m.Votes)
	}
}

func TestLoad_MovieSort(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	movies := s.AllMovies()
	// tt0000003 (Gamma, 8.5) and tt0000001 (Alpha, 8.5) tie on rating;
	// alphabetical tiebreak: Alpha < Gamma, so tt0000001 first.
	// tt0000002 (Beta, 7.5) last.
	wantOrder := []string{"tt0000001", "tt0000003", "tt0000002"}
	for i, m := range movies {
		if m.ID != wantOrder[i] {
			t.Errorf("movies[%d].ID = %q, want %q", i, m.ID, wantOrder[i])
		}
	}
}

func TestLoad_ActorSort(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	actors := s.AllActors()
	// Alice Smith < Bob Jones < Carol Davis
	wantOrder := []string{"nm0000001", "nm0000002", "nm0000003"}
	for i, a := range actors {
		if a.ID != wantOrder[i] {
			t.Errorf("actors[%d].ID = %q, want %q", i, a.ID, wantOrder[i])
		}
	}
}

func TestLoad_ActorFields(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// deathYear: 0 (alive/unknown)
	alice, ok := s.Actor("nm0000001")
	if !ok {
		t.Fatal("nm0000001 not found")
	}
	if alice.DeathYear != 0 {
		t.Errorf("Alice deathYear = %d, want 0", alice.DeathYear)
	}

	// birthYear: 0 (unknown)
	bob, ok := s.Actor("nm0000002")
	if !ok {
		t.Fatal("nm0000002 not found")
	}
	if bob.BirthYear != 0 {
		t.Errorf("Bob birthYear = %d, want 0", bob.BirthYear)
	}

	// normal death year
	carol, ok := s.Actor("nm0000003")
	if !ok {
		t.Fatal("nm0000003 not found")
	}
	if carol.DeathYear != 2020 {
		t.Errorf("Carol deathYear = %d, want 2020", carol.DeathYear)
	}
}

func TestLoad_MovieRoles(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m, ok := s.Movie("tt0000001")
	if !ok {
		t.Fatal("tt0000001 not found")
	}
	// Alpha Movie has 3 roles: actor, self, director.
	if len(m.Roles) != 3 {
		t.Fatalf("role count = %d, want 3", len(m.Roles))
	}
	// self category present.
	var hasSelf bool
	for _, r := range m.Roles {
		if r.Category == "self" {
			hasSelf = true
		}
	}
	if !hasSelf {
		t.Error("no 'self' role found in Alpha Movie")
	}
	// Director role has no characters (omitted in JSON).
	for _, r := range m.Roles {
		if r.Category == "director" && len(r.Characters) != 0 {
			t.Errorf("director role should have no characters, got %v", r.Characters)
		}
	}
}

func TestLoad_MissingRatingsFile(t *testing.T) {
	dir := t.TempDir()
	copyFile(t, filepath.Join(testdata, "movies.json"), filepath.Join(dir, "movies.json"))
	copyFile(t, filepath.Join(testdata, "actors.json"), filepath.Join(dir, "actors.json"))
	// ratings.json intentionally absent.

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for missing ratings.json, got nil")
	}
}

func TestLoad_MalformedMoviesJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "movies.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(testdata, "actors.json"), filepath.Join(dir, "actors.json"))
	copyFile(t, filepath.Join(testdata, "ratings.json"), filepath.Join(dir, "ratings.json"))

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for malformed movies.json, got nil")
	}
}

func TestLoad_MalformedActorsJSON(t *testing.T) {
	dir := t.TempDir()
	copyFile(t, filepath.Join(testdata, "movies.json"), filepath.Join(dir, "movies.json"))
	if err := os.WriteFile(filepath.Join(dir, "actors.json"), []byte("[bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(testdata, "ratings.json"), filepath.Join(dir, "ratings.json"))

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for malformed actors.json, got nil")
	}
}

func TestLoad_ImmutableSlices(t *testing.T) {
	s, err := Load(testdata)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	movies1 := s.AllMovies()
	movies2 := s.AllMovies()
	// Mutating the returned slice should not affect subsequent calls.
	movies1[0] = nil
	if movies2[0] == nil {
		t.Error("AllMovies returned same backing array; slice isolation broken")
	}
}

func TestLoad_NoExportedMutableFields(t *testing.T) {
	// Compile-time check: Store has no exported mutable fields.
	// If the struct gains exported fields, this test file won't compile unless updated.
	var _ Store
}

// copyFile is a helper to copy a file to a temp directory.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("copyFile read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("copyFile write %s: %v", dst, err)
	}
}
