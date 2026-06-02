# Session 2 API Review — tag 0.2.0

**Date:** 2026-06-02  
**Reviewer:** Claude (code review, no modifications)  
**Sources checked:** api-plan.md, api-changes.md, all implementation files, all test files  
**Coverage run:** `make test` → **92.4%** (gate ≥80%) — all tests green

---

## Verdict: SHIP ✅

All in-scope tasks pass their exit criteria. The two gaps below are planning artifacts, not code defects, and are consistent with the parking-lot scope cut.

---

## Checklist Results

### Upfront Decisions

| Decision | Status | Notes |
|---|---|---|
| D1 — Response envelope `{ "items": [...], "total": N, "page": N, "pageSize": N }` | ✅ PASS | Implemented as `Page[T any]` generic in `internal/store/types.go:67`. Used by all three list handlers. |
| D2 — `cast` (actor/actress) in list; `roles` (all) in detail | ✅ PASS | `toListItem()` in `movies.go:98` filters `category == "actor" \| "actress"` into `Cast`; `toDetail()` passes full `Roles`. `TestMovies_CastOnlyInList` verifies no "roles" key appears in list items. |

---

### Phase 1 — Data Types (T1)

✅ PASS. `internal/store/types.go` defines `Movie`, `Role`, `Actor`, `ActorMovie`, `MovieListItem`, `MovieDetail`, `Page[T]`. No response type has `json` tags for `key` or `_id`. The `rawMovie` struct in `store.go` omits both fields, so they are silently dropped during JSON parsing of fixture data that contains them.

---

### Phase 2 — Store (T2, T3)

✅ PASS.

**T2 — Loader:**
- Reads all three JSON files; returns error on missing or malformed file.
- Ratings pre-joined by `movieId`; missing rating emits `slog.Warn` and continues (no panic).
- `Store` struct: all fields unexported; `AllMovies()` and `AllActors()` return `copy()` of internal slices (immutability verified by `TestLoad_ImmutableSlices`).
- Genres: collected from `rawMovies` (not `enrichedMovies` map iteration — avoids map-ordering non-determinism), deduplicated via `map[string]struct{}`, sorted with `sort.Strings`.
- Sort: movies by rating desc, title asc (tested by `TestLoad_MovieSort` with a rating-tie case); actors by name asc (`TestLoad_ActorSort`).

**T3 — Store tests (12 total):**
- Happy path: movie count, actor count, genre list ✅
- `TestLoad_GenreDeduplication`: Action + Drama each appear in two movies; total distinct = 4, sorted ✅
- `TestLoad_RatingJoin`: tt0000001 gets rating=8.5, votes=10000 ✅
- `TestLoad_MovieSort`: tie-break on rating → alphabetical (Alpha before Gamma) ✅
- `TestLoad_ActorSort`: Alice < Bob < Carol ✅
- `TestLoad_ActorFields`: deathYear=0 (alive/unknown), birthYear=0 (unknown), normal death year ✅
- `TestLoad_MovieRoles`: "self" category present; director role has no characters ✅
- Error paths (3): missing ratings.json, malformed movies.json, malformed actors.json ✅
- `TestLoad_ImmutableSlices`: mutating returned slice doesn't affect next call ✅
- `TestLoad_NoExportedMutableFields`: compile-time check ✅

---

### Phase 3 — Server Wiring (T4)

✅ PASS. `server.New(version, *store.Store)` registers exactly 7 routes:
`GET /version`, `GET /healthz`, `GET /api/genres`, `GET /api/movies`, `GET /api/movies/{id}`, `GET /api/actors`, `GET /api/actors/{id}`. `cmd/moviesx/main.go` calls `store.Load()` before `server.New()`; fatal on error.

---

### Phase 4 — Handlers (T5–T9)

**T5 — GET /api/genres** ✅ PASS  
`handleGenres` calls `s.store.Genres()` and writes it directly. Returns a JSON array (not an object). `TestGenres_SortedList` confirms `["Action","Comedy","Drama","Thriller"]`.

**T6 — GET /api/movies** ✅ PASS  
- **q filter:** case-insensitive substring on `title` (`strings.ToLower` on both sides). Tested: match, no-match.
- **genre filter:** `containsGenre` uses `strings.EqualFold` — case-insensitive exact match. Tested: match, no-match.
- **year filter:** exact integer match. Tested: match, no-match.
- **AND combination:** `TestMovies_TwoFiltersCombined` — year=2001 AND genre=Comedy → 1 result (Beta Movie only).
- **Empty result:** `TestMovies_QNoMatch` (q=zzzz) → 200 `{"items":[],"total":0,...}`, never 404.
- **Pagination math:** `start = (pageNum-1)*pgSize`, clamped to `[0, total]`; `end` clamped to `total`. `TestMovies_Pagination` (pageSize=1 → 1 item, total=3) ✅. `TestMovies_PageBeyondLast` (pageNumber=99 → 0 items, total=3) ✅.

**T7 — GET /api/movies/{id}** ✅ PASS  
- `validate.MovieID()` enforces `^tt\d{5,9}$` → 400 on mismatch.
- `s.store.Movie(id)` returns `(nil, false)` on miss → 404 `{"error":"not found"}`.
- Detail response: `toDetail()` includes full `roles` array (all categories). `TestMovieByID_Hit` verifies 3 roles and `rating` field present. `TestMovieByID_UnknownValidID` (tt9999999) → 404. `TestMovieByID_MalformedID` (badid) → 400.

**T8 — GET /api/actors** ✅ PASS  
- q filter: case-insensitive substring on `name`. `TestActors_QMatch` (q=alice) → 1 result (nm0000001). `TestActors_QNoMatch` (q=zzzzz) → 0 results.
- `TestActors_NoFilter` → total=3.
- `TestActors_Pagination` (pageSize=2) → 2 items, total=3.

**T9 — GET /api/actors/{id}** ✅ PASS  
- `validate.ActorPathID()` enforces `^nm\d{5,9}$` → 400 on mismatch.
- 404 on miss. `TestActorByID_Hit` (nm0000001) → 200 with `movies` array of length 2. `TestActorByID_UnknownValidID` → 404. `TestActorByID_MalformedID` → 400.

---

### Phase 5 — Validation (T10, T11)

✅ PASS.

**T10 — `internal/validate/validate.go`**  
8 exported functions (`PageNumber`, `PageSize`, `Q`, `Year`, `Rating`, `ActorID`, `MovieID`, `ActorPathID`). Zero imports of `net/http`. All functions return `(value, error)` with human-readable error strings. No panics on empty input.

| Param | Rule | Implemented |
|---|---|---|
| pageNumber | [1, 10000], default 1 | ✅ |
| pageSize | [1, 1000], default 20 | ✅ |
| q | length [2, 20] when present | ✅ (byte length — correct for ASCII dataset) |
| year | [1888, 2100] when present | ✅ |
| rating | [1.0, 10.0] when present | ✅ |
| actorId (query param) | `^nm\d{5,9}$` | ✅ (implemented; not called from any handler — see Gap 2) |
| movieId (path) | `^tt\d{5,9}$` | ✅ |
| actorId (path) | `^nm\d{5,9}$` | ✅ |

**T11 — Boundary coverage:**

| Validator | Empty | Lower | Lower−1 | Upper | Upper+1 | Non-numeric/bad |
|---|---|---|---|---|---|---|
| PageNumber | ✅ | ✅ (1) | ✅ (0) | ✅ (10000) | ✅ (10001) | ✅ (abc, −1) |
| PageSize | ✅ | ✅ (1) | ✅ (0) | ✅ (1000) | ✅ (1001) | ✅ (abc) |
| Q | ✅ | ✅ (2 chars) | ✅ (1 char) | ✅ (20 chars) | ✅ (21 chars) | n/a |
| Year | ✅ | ✅ (1888) | ✅ (1887) | ✅ (2100) | ✅ (2101) | ✅ (abc) |
| Rating | ✅ | ✅ (1.0) | ✅ (0.9) | ✅ (10.0) | ✅ (10.1) | ✅ (abc, 0, 11) |
| ActorID | ✅ | ✅ (5 digits) | ✅ (4 digits) | ✅ (9 digits) | ✅ (10 digits) | ✅ (wrong prefix, no prefix) |
| MovieID | ✅ (→ error) | ✅ (5 digits) | ✅ (4 digits) | ✅ (9 digits) | ✅ (10 digits) | ✅ (wrong prefix, no prefix) |
| ActorPathID | ✅ (→ error) | ✅ | ✅ | ✅ | — | ✅ (wrong prefix) |

All boundaries covered. All tests pass.

---

### Phase 6 — Integration Tests (T12)

✅ PASS for all implemented features. 29 integration tests against a 3-movie / 3-actor / 3-rating fixture via `httptest.NewServer`.

| Required case | Test | Status |
|---|---|---|
| GET /api/genres: sorted list | TestGenres_SortedList | ✅ |
| GET /api/movies: no filter (3 items) | TestMovies_NoFilter | ✅ |
| GET /api/movies: q match, q no-match | TestMovies_QMatch, TestMovies_QNoMatch | ✅ |
| GET /api/movies: genre match, genre no-match | TestMovies_GenreMatch, TestMovies_GenreNoMatch | ✅ |
| GET /api/movies: year match, year no-match | TestMovies_YearMatch, TestMovies_YearNoMatch | ✅ |
| GET /api/movies: two filters combined (AND) | TestMovies_TwoFiltersCombined | ✅ |
| GET /api/movies: pageSize=1, total=3 | TestMovies_Pagination | ✅ |
| GET /api/movies: pageNumber=99 → 200, empty items | TestMovies_PageBeyondLast | ✅ |
| GET /api/movies: D2 cast-only in list | TestMovies_CastOnlyInList | ✅ |
| GET /api/movies/{id}: hit → 200 | TestMovieByID_Hit | ✅ |
| GET /api/movies/{id}: unknown valid ID → 404 | TestMovieByID_UnknownValidID | ✅ |
| GET /api/movies/{id}: malformed ID → 400 | TestMovieByID_MalformedID | ✅ |
| GET /api/actors: no filter, q match, q no-match | TestActors_NoFilter, _QMatch, _QNoMatch | ✅ |
| GET /api/actors: pagination | TestActors_Pagination | ✅ |
| GET /api/actors/{id}: hit, miss, malformed | TestActorByID_Hit, _UnknownValidID, _MalformedID | ✅ |
| Invalid pageNumber → 400 | TestMovies_InvalidPageNumber, TestActors_InvalidPageNumber | ✅ |
| Invalid pageSize → 400 | TestMovies_InvalidPageSize, TestActors_InvalidPageSize | ✅ |
| Invalid q length → 400 | TestMovies_InvalidQ, TestActors_InvalidQ | ✅ |
| Invalid year range → 400 | TestMovies_InvalidYear | ✅ |
| Invalid actorId format → 400 (path param) | TestActorByID_MalformedID | ✅ |
| GET /api/movies: rating match, no-match | — | ⚠️ GAP 1 (see below) |
| GET /api/movies: actorId match | — | ⚠️ GAP 1 (see below) |
| Invalid rating → 400 (list endpoint) | — | ⚠️ GAP 1 (see below) |
| Invalid actorId format → 400 (query param) | — | ⚠️ GAP 1 (see below) |

---

### Phase 7 — Coverage Gate (T13)

✅ PASS. `make test` output:

```
internal/config:   100.0%
internal/server:    96.8%
internal/store:     93.1%
internal/validate: 100.0%
total:              92.4%
PASS: coverage 92.4% meets 80% threshold
```

---

### Phase 8 — Wrap-up (T14)

✅ PASS. `CLAUDE.md` reflects 0.2.0 done, D1/D2 decisions, updated layout, coverage, and session map. `0.3.0` marked "next".

---

### Parking Lot Confirmation

✅ CONFIRMED ABSENT. Neither `actorId` nor `rating` query params are parsed, validated, or filtered in any handler. The only references in `movies.go:34` is an explanatory comment. Both validators exist in `validate.go` but are not called from any handler.

---

## Gaps (Non-Blocking)

### Gap 1 — T12 plan contains 4 test-matrix rows for parking-lot features

The T12 test matrix in `api-plan.md` was written before the scope cut and lists:

- `GET /api/movies: rating threshold match, no-match` — not testable (filter not implemented)
- `GET /api/movies: actorId match (role in any category)` — not testable (filter not implemented)
- `All list endpoints: invalid rating → 400` — not testable (param not parsed)
- `All list endpoints: invalid actorId format → 400` — not testable as query param (param not parsed); covered via path param `TestActorByID_MalformedID`

These are planning artifacts. The implementation is internally consistent with the parking-lot scope cut. No action needed; track as future T12 additions when 0.3.x implements actorId/rating filters.

### Gap 2 — `validate.ActorID()` and `validate.Rating()` are implemented but uncalled

Both functions are fully implemented and unit-tested. Neither is called from any handler because the corresponding filters are parking lot. They are dead code today but clearly stubs for future sessions. Not a defect.

---

## Summary

| Area | Result |
|---|---|
| In-scope tasks D1, D2, T1–T14 | All exit criteria met ✅ |
| q/genre/year filters combine as AND | ✅ |
| GET /api/movies/{id} — 404 on miss, 400 on bad format | ✅ |
| GET /api/actors and GET /api/actors/{id} | ✅ |
| GET /api/genres — sorted, unique, JSON array | ✅ |
| Negative tests: pageNumber, pageSize, q, year, actorId format | ✅ |
| Coverage ≥ 80% | 92.4% ✅ |
| actorId and rating filters absent + in parking lot | ✅ |
| T12 test rows for parking-lot features | ⚠️ 4 rows not satisfiable — planning artifact |

**SHIP** — no blocking issues.
