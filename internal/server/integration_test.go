package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbr/moviesx/internal/server"
	"github.com/mbr/moviesx/internal/store"
)

const fixtureDir = "testdata"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Load(fixtureDir)
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	return httptest.NewServer(server.New("test", st))
}

func getJSON(t *testing.T, srv *httptest.Server, path string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response from %s: %v", path, err)
	}
	return resp.StatusCode, body
}

func getJSONArray(t *testing.T, srv *httptest.Server, path string) (int, []any) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var body []any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response from %s: %v", path, err)
	}
	return resp.StatusCode, body
}

// --- GET /api/genres ---

func TestGenres_SortedList(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, genres := getJSONArray(t, srv, "/api/genres")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	want := []string{"Action", "Comedy", "Drama", "Thriller"}
	if len(genres) != len(want) {
		t.Fatalf("genres count = %d, want %d: %v", len(genres), len(want), genres)
	}
	for i, g := range genres {
		if g.(string) != want[i] {
			t.Errorf("genres[%d] = %q, want %q", i, g, want[i])
		}
	}
}

// --- GET /api/movies ---

func itemsFrom(t *testing.T, body map[string]any) []any {
	t.Helper()
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("items field missing or wrong type: %v", body)
	}
	return items
}

func totalFrom(t *testing.T, body map[string]any) int {
	t.Helper()
	v, ok := body["total"].(float64)
	if !ok {
		t.Fatalf("total field missing or wrong type: %v", body)
	}
	return int(v)
}

func TestMovies_NoFilter(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	items := itemsFrom(t, body)
	if len(items) != 3 {
		t.Errorf("items count = %d, want 3", len(items))
	}
	if totalFrom(t, body) != 3 {
		t.Errorf("total = %d, want 3", totalFrom(t, body))
	}
}

func TestMovies_QMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?q=alpha")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 1 {
		t.Errorf("total = %d, want 1", totalFrom(t, body))
	}
}

func TestMovies_QNoMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?q=zzzz")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 0 {
		t.Errorf("total = %d, want 0", totalFrom(t, body))
	}
	items := itemsFrom(t, body)
	if len(items) != 0 {
		t.Errorf("items count = %d, want 0", len(items))
	}
}

func TestMovies_GenreMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// "Comedy" only in Beta Movie.
	status, body := getJSON(t, srv, "/api/movies?genre=Comedy")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 1 {
		t.Errorf("total = %d, want 1", totalFrom(t, body))
	}
}

func TestMovies_GenreNoMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?genre=Horror")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 0 {
		t.Errorf("total = %d, want 0", totalFrom(t, body))
	}
}

func TestMovies_YearMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Alpha and Beta both year=2001.
	status, body := getJSON(t, srv, "/api/movies?year=2001")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 2 {
		t.Errorf("total = %d, want 2", totalFrom(t, body))
	}
}

func TestMovies_YearNoMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?year=1999")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 0 {
		t.Errorf("total = %d, want 0", totalFrom(t, body))
	}
}

func TestMovies_TwoFiltersCombined(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// year=2001 AND genre=Comedy → only Beta Movie.
	status, body := getJSON(t, srv, "/api/movies?year=2001&genre=Comedy")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 1 {
		t.Errorf("total = %d, want 1", totalFrom(t, body))
	}
}

func TestMovies_Pagination(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?pageSize=1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 3 {
		t.Errorf("total = %d, want 3", totalFrom(t, body))
	}
	items := itemsFrom(t, body)
	if len(items) != 1 {
		t.Errorf("items on page 1 = %d, want 1", len(items))
	}
}

func TestMovies_PageBeyondLast(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?pageNumber=99")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never 404 on empty page)", status)
	}
	items := itemsFrom(t, body)
	if len(items) != 0 {
		t.Errorf("items on page 99 = %d, want 0", len(items))
	}
	if totalFrom(t, body) != 3 {
		t.Errorf("total = %d, want 3 (total is full count, not page count)", totalFrom(t, body))
	}
}

func TestMovies_CastOnlyInList(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	items := itemsFrom(t, body)
	for _, item := range items {
		m := item.(map[string]any)
		cast, ok := m["cast"].([]any)
		if !ok {
			t.Errorf("movie %v: cast field missing or wrong type", m["id"])
			continue
		}
		for _, roleAny := range cast {
			role := roleAny.(map[string]any)
			cat := role["category"].(string)
			if cat != "actor" && cat != "actress" {
				t.Errorf("movie %v: list contains non-cast role category %q (D2 violation)", m["id"], cat)
			}
		}
		// No "roles" field should appear in list items.
		if _, hasRoles := m["roles"]; hasRoles {
			t.Errorf("movie %v: list item must not expose 'roles' field (D2)", m["id"])
		}
	}
}

// --- GET /api/movies/{id} ---

func TestMovieByID_Hit(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies/tt0000001")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["id"] != "tt0000001" {
		t.Errorf("id = %v, want tt0000001", body["id"])
	}
	// Detail must expose full roles (including director).
	roles, ok := body["roles"].([]any)
	if !ok {
		t.Fatal("roles field missing in detail response")
	}
	if len(roles) != 3 {
		t.Errorf("roles count = %d, want 3 (all roles)", len(roles))
	}
	// Must have rating and votes.
	if body["rating"] == nil {
		t.Error("rating field missing in detail response")
	}
}

func TestMovieByID_UnknownValidID(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies/tt9999999")
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
	if body["error"] == nil {
		t.Error("error field missing in 404 response")
	}
}

func TestMovieByID_MalformedID(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies/badid")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing in 400 response")
	}
}

// --- GET /api/actors ---

func TestActors_NoFilter(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 3 {
		t.Errorf("total = %d, want 3", totalFrom(t, body))
	}
}

func TestActors_QMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors?q=alice")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 1 {
		t.Errorf("total = %d, want 1", totalFrom(t, body))
	}
	items := itemsFrom(t, body)
	if len(items) != 1 {
		t.Fatalf("items count = %d, want 1", len(items))
	}
	a := items[0].(map[string]any)
	if a["id"] != "nm0000001" {
		t.Errorf("id = %v, want nm0000001", a["id"])
	}
}

func TestActors_QNoMatch(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors?q=zzzzz")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 0 {
		t.Errorf("total = %d, want 0", totalFrom(t, body))
	}
}

func TestActors_Pagination(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors?pageSize=2")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if totalFrom(t, body) != 3 {
		t.Errorf("total = %d, want 3", totalFrom(t, body))
	}
	items := itemsFrom(t, body)
	if len(items) != 2 {
		t.Errorf("items on page 1 = %d, want 2", len(items))
	}
}

// --- GET /api/actors/{id} ---

func TestActorByID_Hit(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors/nm0000001")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["id"] != "nm0000001" {
		t.Errorf("id = %v, want nm0000001", body["id"])
	}
	movies, ok := body["movies"].([]any)
	if !ok {
		t.Fatal("movies field missing in actor detail")
	}
	if len(movies) != 2 {
		t.Errorf("movies count = %d, want 2", len(movies))
	}
}

func TestActorByID_UnknownValidID(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors/nm9999999")
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
	if body["error"] == nil {
		t.Error("error field missing in 404 response")
	}
}

func TestActorByID_MalformedID(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors/badid")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing in 400 response")
	}
}

// --- Invalid param validation ---

func TestMovies_InvalidPageNumber(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?pageNumber=0")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing")
	}
}

func TestMovies_InvalidPageSize(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?pageSize=9999")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing")
	}
}

func TestMovies_InvalidQ(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?q=x")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing")
	}
}

func TestMovies_InvalidYear(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/movies?year=1800")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing")
	}
}

func TestActors_InvalidPageNumber(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors?pageNumber=abc")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing")
	}
}

func TestActors_InvalidPageSize(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors?pageSize=0")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing")
	}
}

func TestActors_InvalidQ(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := getJSON(t, srv, "/api/actors?q=z")
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if body["error"] == nil {
		t.Error("error field missing")
	}
}

// --- GET /metrics ---

func TestMetrics_OK(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
}

func TestMetrics_CounterIncrements(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Trigger at least one instrumented request.
	if _, err := http.Get(srv.URL + "/api/genres"); err != nil {
		t.Fatalf("GET /api/genres: %v", err)
	}

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	if !strings.Contains(string(body), "http_requests_total{") {
		t.Error("/metrics body does not contain http_requests_total{")
	}
}
