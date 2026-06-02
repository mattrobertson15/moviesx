# API Research: Data Schema and Query Semantics

**Date:** 2026-06-02  
**Purpose:** Establish defensible decisions on /api/movies and /api/actors query semantics before implementation begins (Session 2, tag 0.2.0).

---

## 1. Dataset Size

| File | Records |
|---|---|
| `src/data/movies.json` | 100 movies |
| `src/data/actors.json` | 553 actor/crew records |
| `src/data/ratings.json` | 100 ratings (one per movie) |

---

## 2. ID Formats

Both formats use exactly 7 digits in the data (the spec regex `^tt\d{5,9}$` / `^nm\d{5,9}$` is intentionally wider to allow for IDs that differ in length).

| Type | Format | Examples |
|---|---|---|
| Movie | `tt` + 7 digits | `tt0120737` (LOTR: Fellowship), `tt0133093` (The Matrix) |
| Actor/Crew | `nm` + 7 digits | `nm0000002` (Lauren Bacall), `nm0000704` (Elijah Wood) |

Every record in all three files has redundant identity fields (`id`, `key`, `_id`, plus `movieId`/`actorId`) all holding the same value. Use `id` (or `movieId`/`actorId`) canonically; discard `key` and `_id` from API responses.

---

## 3. Inferred Schemas

### 3.1 movies.json

```json
{
  "id": "tt0120737",
  "title": "The Lord of the Rings: The Fellowship of the Ring",
  "year": 2001,
  "runtime": 178,
  "genres": ["Adventure", "Drama", "Fantasy"],
  "roles": [
    {
      "order": 1,
      "actorId": "nm0000704",
      "name": "Elijah Wood",
      "category": "actor",
      "characters": ["Frodo"]
    },
    {
      "order": 5,
      "actorId": "nm0001392",
      "name": "Peter Jackson",
      "category": "director"
    },
    {
      "order": 9,
      "actorId": "nm0651614",
      "name": "Barrie M. Osborne",
      "category": "producer",
      "job": "producer"
    }
  ]
}
```

**Notes:**
- `year` is an integer, not a string.
- `runtime` is minutes (integer).
- `genres` is a non-empty array of capitalized strings (e.g., `"Action"`, `"Sci-Fi"`).
- `roles[].category` values observed: `"actor"`, `"actress"`, `"director"`, `"producer"`, `"self"`.
- `roles[].characters` is absent for directors and some producers.
- `roles[].job` is present only when `category` is `"director"` or `"producer"` and a sub-role exists (e.g., `"co-director"`, `"producer"`).
- No `description`, `plot`, or `language` field exists.

**Example — movie with `self` category (documentary):** `src/data/movies.json` index for `tt0264476` ("Children Underground"), roles include `"category": "self"`.

### 3.2 actors.json

```json
{
  "id": "nm0000704",
  "type": "actor",
  "actorId": "nm0000704",
  "name": "Elijah Wood",
  "birthYear": 1981,
  "deathYear": 0,
  "profession": ["actor", "producer", "soundtrack"],
  "movies": [
    { "movieId": "tt0120737", "title": "The Lord of the Rings: The Fellowship of the Ring" },
    { "movieId": "tt0167260", "title": "The Lord of the Rings: The Return of the King" },
    { "movieId": "tt0167261", "title": "The Lord of the Rings: The Two Towers" }
  ]
}
```

**Notes:**
- `deathYear: 0` means alive (or unknown death year) — it is **not** null. Example: `nm0001657` (Oliver Reed, d. 1999) has `"deathYear": 1999`; Brad Pitt has `"deathYear": 0`.
- `birthYear: 0` means unknown birth year. Example: `nm0004423` (Gerry Robert Byrne).
- `profession` array is not restricted to "actor"/"actress" — includes "director", "producer", "writer", etc. The actors.json file is a crew+cast registry, not actors-only.
- `movies` is the list of films this person appears in from the dataset (denormalized for read convenience).

**Cross-check:** `nm0000704` (Elijah Wood) appears in 3 movies in actors.json, matching the 3 LOTR films in movies.json where his `actorId` appears in `roles`.

### 3.3 ratings.json

```json
{
  "id": "tt0120737",
  "movieId": "tt0120737",
  "type": "Rating",
  "rating": 8.8,
  "votes": 1446962
}
```

**Notes:**
- `rating` is a float with one decimal place in the source data.
- `votes` is an integer.
- Exactly 100 entries — one per movie — no missing or extra records.

---

## 4. Ratings Join

**Join key:** `ratings.json[n].movieId` == `movies.json[m].id`  
**Cardinality:** 1:1 (100 movies, 100 ratings, fully matched)  
**Decision:** Pre-join at startup into a single in-memory map `movieId → Rating`. No lazy join needed. The `rating` field and `votes` field should be included in `/api/movies` responses (both list and detail).

**Verification:** `tt0167260` ("LOTR: Return of the King") has `"rating": 8.9` in ratings.json and `"year": 2003` in movies.json — consistent.

---

## 5. Valid Ranges for `year` and `rating` Filters

### 5.1 `year`

**Data range:** 1999 (`tt0133093`, The Matrix) through 2009 (one film at the tail end).  
**Decision:** Validate `year` ∈ [1888, 2100].

Rationale: 1888 is the year of the first recorded film. Using the exact data range [1999, 2009] would make the API unusable if more seed data is added. Using [1888, 2100] is defensible, semantically meaningful, and rejects nonsense values like 0 or 99999.

**HTTP 400** if `year` is outside [1888, 2100] or non-integer.

### 5.2 `rating`

**Data range:** 8.0 (many films) through 8.9 (`tt0167260`, LOTR: Return of the King).  
**Decision:** Validate `rating` ∈ [1.0, 10.0] (inclusive, float).

Rationale: The source is IMDb-style ratings on a 1–10 scale. The dataset is a curated top-films subset so actual values cluster at 8.0–8.9, but the filter should accept the full IMDb scale. A caller asking for `?rating=7.5` should get an empty result set, not a 400. Only values outside the IMDb domain (e.g., 0, 11, negative) are invalid.

**HTTP 400** if `rating` < 1.0 or > 10.0 or non-numeric. Accept one decimal place minimum; do not require it.

---

## 6. `q` Parameter Match Scope

**Decision for `/api/movies`:** Match against `title` only (case-insensitive substring).

Rationale: The only natural-language text fields in a movie record are `title` and `roles[].name`. Matching `q` against actor names inside the movie record would make `q` redundant with `actorId`. A user typing "Keanu" in a movie search would be surprised to get The Matrix back (it works, but the expectation is title search). If actor-name movie discovery is needed, the client should use `/api/actors?q=Keanu` to get `nm0000206`, then call `/api/movies?actorId=nm0000206`. This is compositional and clear.

**Decision for `/api/actors`:** Match against `name` only (case-insensitive substring).

No other natural-language field on an actor record is appropriate for free-text search.

**Minimum length:** 2 characters (per spec §6 validation rule).  
**Maximum length:** 20 characters (per spec §6 validation rule).  
**HTTP 400** if `q` is present but length < 2 or > 20.

---

## 7. Filter Combination Logic

**Decision:** All filters combine as **AND** (intersection/narrowing).

**Rationale:** AND is the universal REST convention for multi-parameter filtering. A query like `?genre=Action&year=2003&rating=8.0` should return Action films from 2003 with rating ≥ 8.0 — not the union of all three sets. OR semantics would require an explicit query-language syntax (e.g., `filter=...`).

**Filters on `/api/movies`:**

| Parameter | Filter logic |
|---|---|
| `q` | `title` contains `q` (case-insensitive) |
| `genre` | `genres` array contains exact string (case-insensitive match) |
| `year` | `year == value` (exact match, not range) |
| `rating` | joined `rating >= value` (minimum rating threshold) |
| `actorId` | any entry in `roles[].actorId` equals value (all categories: actor, director, producer, self) |

**`genre` exact vs. substring:** The genres in the data are capitalized single words or hyphenated (e.g., `"Sci-Fi"`, `"Drama"`). Exact case-insensitive match (not substring) is correct — `?genre=Fi` should not return Sci-Fi films.

**`actorId` scope:** Includes all roles regardless of `category` — a director (e.g., `nm0634240` Christopher Nolan on `tt0209144` Memento) should be findable via `actorId=nm0634240`. The parameter name is a slight misnomer, but matching the spec.

**Filters on `/api/actors`:** Only `q` (name substring search). No other filters specified.

---

## 8. Default Sort Orders

### 8.1 `/api/movies`

**Decision:** Primary sort by `rating` descending, secondary (tie-break) by `title` ascending.

Rationale: The dataset is a curated collection of high-quality films. Showing the highest-rated films first is the natural default for a movie catalog. Tie-breaking by title alphabetically gives stable, reproducible ordering (many films share the same rating floor of 8.0).

**Example:** With rating desc + title asc, the list would open with LOTR: Return of the King (8.9), followed by the 8.8 films, then 8.7, and so on, each group alphabetized.

### 8.2 `/api/actors`

**Decision:** Sort by `name` ascending (alphabetical).

Rationale: Actor/crew lists have no inherent quality ranking without a `popularity` or `rating` field. Alphabetical by name is the only stable, unambiguous default. No secondary sort needed (names are unique or nearly so in the dataset).

---

## 9. Observed Genres (from `src/data/movies.json`)

For `/api/genres` implementation reference:

```
Action, Adventure, Animation, Biography, Comedy, Crime, Documentary,
Drama, Fantasy, History, Music, Musical, Mystery, Romance, Sci-Fi,
Thriller, War
```

(Derive at runtime from all distinct values in `genres` arrays, sorted alphabetically.)

---

## 10. Observations for Response Shape

- API responses for `/api/movies` should embed the joined `rating` and `votes` — callers should not need to hit a separate endpoint.
- The `roles` array in movies.json mixes cast and crew. For list responses (`/api/movies`), consider returning a filtered cast-only subset (category == "actor" | "actress") to keep payloads small. Full `roles` array appropriate for the detail endpoint `/api/movies/{id}`.
- `actors.json` `profession` field (e.g., `["actress", "soundtrack", "producer"]`) is informational and should be passed through in `/api/actors` responses.
- The denormalized `movies` array in actors.json is a pre-computed list — use it directly in `/api/actors/{id}` responses rather than re-deriving it from movies.json at query time. However, if rating data is wanted on actor film lists, a join is necessary since actors.json contains only `movieId` and `title`.

---

## 11. Open Questions (Deferred to Implementation)

1. **`rating` filter semantics — exact or minimum?** Decision above is minimum threshold (`rating >= value`). Confirm before finalizing request validation.
2. **`year` filter semantics — exact or range?** Decision above is exact match. A range filter (`yearFrom` / `yearTo`) would require additional params not in the spec — defer.
3. **Response envelope shape** — does the list response use `{ "items": [...], "total": N, "page": N, "pageSize": N }` or a flat array? Spec §6 does not define this; needs a decision in Session 2.
4. **Cast vs. crew in `/api/movies` list response** — whether to return all roles or cast-only is deferred to Session 2.
