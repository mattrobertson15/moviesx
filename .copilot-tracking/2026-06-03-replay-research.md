# Replay Tool Research — Session 5 pre-work (2026-06-03)

Covering §10.3 end-to-end contract tests and §10.4 benchmark mode.
Research only — no code written.

---

## 1. Language

**Decision: Go.**

Rationale:
- The project is already Go; adding `cmd/replay/` costs zero new toolchain or runtime dependency. A fresh contributor with `go` installed gets the tool for free from `go build ./cmd/replay/`.
- Goroutines are the best-fit concurrency primitive for high-RPS load generation and are already in use in `internal/server/` (via `sync/atomic`).
- A single `CGO_ENABLED=0 go build` produces a static Linux binary that can run inside the cluster (e.g. in a debug container or as a Job) without an interpreter.
- Scenario file parsing needs only one external library (`gopkg.in/yaml.v3`) rather than a full language runtime.
- Spec §10.3 explicitly says the tool language is the implementer's call; Go is defensible and eliminates the cross-language cognitive load.

Tradeoffs vs alternatives:
- Python would be faster to prototype; rejects because it adds a runtime dependency and is slower for tight rate-loop goroutine scheduling.
- TypeScript/Deno: same objection. Also would introduce `package.json` to a Go repo.
- Shell + curl: can hit endpoints but cannot calculate p95 latency or sustain 500 RPS accurately.

---

## 2. Scenario File Format

**Decision: YAML (parsed with `gopkg.in/yaml.v3`).**

Rationale:
- YAML supports inline comments, which are essential for documenting *why* a negative case exists (e.g., `# q too short — must be ≥2 chars`). JSON does not.
- YAML is already the ambient format of the repo (all K8s manifests, Kustomize overlays are YAML). Reviewers are familiar.
- `gopkg.in/yaml.v3` is the canonical pure-Go YAML library; it unmarshals directly into Go structs, is well-maintained, and adds ~250KB to the binary.
- TOML is a valid alternative but is less common for test definition files; the scenario shape is nested (assertions on sub-fields), which YAML handles more naturally.

JSON was rejected because the scenario files will include both test intent comments and assertion sub-objects; JSON's verbosity and comment-absence make it awkward for a test suite humans will maintain.

Proposed scenario schema (YAML):

```yaml
# scenarios/baseline.yaml
scenarios:
  - id: movies-list-default
    method: GET
    path: /api/movies
    # no query string — exercises defaults
    expect_status: 200
    expect_headers:
      Content-Type: application/json
    assert:
      has_keys: [items, total, page, pageSize]

  - id: movies-q-too-short
    method: GET
    path: /api/movies
    query: {q: a}
    expect_status: 400
    assert:
      has_keys: [error]

  - id: movies-by-id-found
    method: GET
    path: /api/movies/tt0000001   # substituted via seed data ref at runtime
    expect_status: 200
    assert:
      body_contains:
        id: tt0000001

  - id: genres-array
    method: GET
    path: /api/genres
    expect_status: 200
    assert:
      is_array: true
      array_min_length: 1
```

Top-level keys: `scenarios` (list). Each scenario:
| Key | Required | Purpose |
|---|---|---|
| `id` | yes | unique label for output/reporting |
| `method` | yes | HTTP method (GET for all current endpoints) |
| `path` | yes | path including any path parameters |
| `query` | no | map of query param key→value (URL-encoded by tool) |
| `expect_status` | yes | expected HTTP status code |
| `expect_headers` | no | map of header→substring match |
| `assert` | no | body assertion block (see §3) |

---

## 3. Assertion Model

**Decision: tiered structural assertions — no JSONPath, no exact-match.**

Full exact-body match is rejected: it hard-codes data content and breaks whenever the dataset changes. JSONPath (`github.com/PaesslerAG/jsonpath`) is expressive but adds a heavy dependency and is overkill for this contract suite, which only needs shape and key-value verification.

The tiered model, from cheapest to most specific:

### 3.1 `has_keys`
Asserts that the decoded JSON object contains all listed keys at the top level. Does not check values. Used for envelope structure.

```yaml
assert:
  has_keys: [items, total, page, pageSize]
```

Implementation: unmarshal response body into `map[string]any`, check key presence. O(k).

### 3.2 `is_array` + `array_min_length`
For endpoints that return a bare array (`/api/genres`). Asserts the body decodes as `[]any` with at least N elements.

```yaml
assert:
  is_array: true
  array_min_length: 1
```

### 3.3 `body_contains`
Subset key-value assertion: every listed key in the decoded object must equal the given scalar value. String comparison is exact; numbers compared as float64. Nested paths use dot notation: `"items.0.id"` (one level of nesting sufficient for current API shape).

```yaml
assert:
  body_contains:
    page: 1
    pageSize: 20
```

Implementation: walk the `map[string]any` tree following dot-separated key path. Return failure with diff on first mismatch.

### 3.4 `body_min_items`
Shorthand for asserting the `items` array within a Page envelope has at least N elements (avoids boilerplate for pagination tests).

```yaml
assert:
  body_min_items: 1
```

### 3.5 Rejected: JSONPath predicates, regex body match
Not needed. The contract tests verify shape and status codes; precise data values are the integration test's job (they already run in-process with fixture data).

---

## 4. Concurrency Model for Benchmark Mode

**Decision: fixed goroutine worker pool + ticker-based rate limiter + in-memory duration slice for p95.**

### 4.1 Worker Pool

```
main goroutine
  → ticker goroutine  (fires at 1/RPS interval, sends job tokens on chan)
  → N worker goroutines (each: dequeue token, pick scenario, fire request, write result)
  → collector goroutine  (aggregates result structs from buffered results chan)
```

`N = --concurrency` workers. Channel buffer = `2 * concurrency` to avoid back-pressure stalls.

Relevant stdlib:
- `sync.WaitGroup` — wait for all workers to drain after duration expires
- `sync/atomic` — `int64` counters for requests sent, successes, errors (no mutex needed for counters)
- `context.WithTimeout` — per-request timeout; also used for the global benchmark duration
- `time.NewTicker` — rate control. At 500 RPS: tick every 2ms. Ticker fires on a `for range` in the rate-limiter goroutine; each tick enqueues one job token.

Ticker math for target RPS:
```go
tickInterval := time.Duration(float64(time.Second) / float64(rps))
```

### 4.2 Rate Limiting

Pure ticker is sufficient for 500 RPS on a single binary (a 2ms ticker has ~1µs jitter on Linux, well within the 50ms p95 target). No need for `golang.org/x/time/rate` token bucket unless burst handling is required. If bursting is needed in a future iteration, `golang.org/x/time/rate.Limiter` is a one-line drop-in.

### 4.3 Latency Collection and p95 Calculation

Collect all response durations in a `[]time.Duration` (pre-allocated at `rps * duration_seconds * 1.2` capacity to avoid GC pressure during the run). After the run completes, sort and index:

```go
sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
p95idx := int(math.Ceil(0.95*float64(len(durations)))) - 1
p95 := durations[p95idx]
```

Stdlib only: `sort.Slice`, `math.Ceil`. No HDR histogram library needed.
Accuracy: for 500 RPS × 30s = 15,000 samples; percentile error at this sample count is negligible for 50ms targets.

### 4.4 Result Struct

```go
type result struct {
    scenario string
    duration time.Duration
    status   int
    err      error
}
```

Collector goroutine receives on `chan result`, appends to slice per-scenario for p95 breakdown.

### 4.5 Benchmark Output

After benchmark completes, print a summary table:
```
endpoint                       requests  errors  p50     p95     p99
GET /api/movies                15000     0       3.1ms   8.4ms   14.2ms
GET /api/movies/{id}           15000     0       0.8ms   2.1ms   3.5ms

PASS: p95 /api/movies = 8.4ms < 50ms target
PASS: p95 /api/movies/{id} = 2.1ms < 10ms target
PASS: error rate = 0.00% < 1% target
```

Exit code 0 on pass, 1 on target miss.

---

## 5. CLI Interface

**Decision: `flag` stdlib only — no cobra, no urfave/cli.**

The tool has one top-level binary with two modes (functional vs benchmark), distinguished by `--benchmark`. This does not need subcommands. `flag` is sufficient and adds zero dependencies.

Flags:

| Flag | Type | Default | Purpose |
|---|---|---|---|
| `--base-url` | string | (required) | Base URL of the target service, e.g. `http://10.42.0.5:8080` |
| `--scenarios` | string | (required) | Path to scenario YAML file, or glob (`scenarios/*.yaml`) |
| `--benchmark` | bool | false | Enable sustained-load benchmark mode instead of functional suite |
| `--duration` | duration | `30s` | Benchmark run duration (parsed via `time.ParseDuration`) |
| `--concurrency` | int | 10 | Number of concurrent goroutines in benchmark mode |
| `--rps` | float | 500 | Target requests per second in benchmark mode |
| `--timeout` | duration | `5s` | Per-request HTTP client timeout |
| `--verbose` | bool | false | Print per-request pass/fail detail in functional mode |
| `--output` | string | `text` | Output format: `text` or `json` (for CI parsing) |

In functional mode `--duration`, `--concurrency`, and `--rps` are ignored (logged as warning if set).
In benchmark mode `--scenarios` selects which requests to loop over (the benchmark scenarios YAML should list only the endpoints targeted by §10.4: `/api/movies` and `/api/movies/{id}`).

---

## 6. Repo Location

**Decision: `cmd/replay/main.go`**

Rationale:
- Follows the existing convention: the API server lives at `cmd/moviesx/main.go`. A second binary at `cmd/replay/main.go` is idiomatic Go.
- `go build ./cmd/replay/` or `go run ./cmd/replay/ --help` requires no additional explanation.
- `tools/replay/` is an alternative; in Go, `tools/` conventionally holds non-runnable build-time helpers (like the existing `tools.go` with build-tag-isolated imports for swag). `cmd/` is for runnable binaries.
- The binary can be added to `Makefile` targets alongside `make test` and `make build`:
  - `make replay-build` → `go build -o bin/replay ./cmd/replay/`
  - `make e2e` → `./bin/replay --base-url $$BASE_URL --scenarios scenarios/baseline.yaml`
  - `make bench` → `./bin/replay --base-url $$BASE_URL --scenarios scenarios/benchmark.yaml --benchmark --duration 30s --rps 500 --concurrency 50`

Internal packages (if any shared logic is needed) would live at `internal/replay/`. For now, all logic should fit in `cmd/replay/main.go` + a few focused files (runner.go, assert.go, scenario.go).

---

## 7. Baseline Scenario Suite — Full Coverage Matrix

Every §6 endpoint plus all validation rules from `internal/validate/validate.go`.

### 7.1 Happy-path (200s)

| Scenario ID | Endpoint | Query / Path | Assert |
|---|---|---|---|
| healthz-pass | GET /healthz | — | status=200, body="pass" (text) |
| version-semver | GET /version | — | status=200, body matches `^\d+\.\d+\.\d+$` |
| readyz-ready | GET /readyz | — | status=200, body_contains: {status: ok} |
| genres-list | GET /api/genres | — | status=200, is_array=true, array_min_length=1 |
| movies-default | GET /api/movies | — | status=200, has_keys: [items,total,page,pageSize] |
| movies-q-match | GET /api/movies | q=tt (or any 2-char min substring that exists) | status=200, body_min_items=1 |
| movies-q-no-match | GET /api/movies | q=zzzzzzzz | status=200, body_contains: {total: 0} |
| movies-genre-filter | GET /api/movies | genre=Action | status=200 |
| movies-year-filter | GET /api/movies | year=2001 | status=200 |
| movies-year-edge-low | GET /api/movies | year=1888 | status=200 (boundary, not a data test) |
| movies-year-edge-high | GET /api/movies | year=2100 | status=200 |
| movies-rating-filter | GET /api/movies | rating=1.0 | status=200 (min boundary) |
| movies-rating-max | GET /api/movies | rating=10.0 | status=200 (max boundary) |
| movies-pagination | GET /api/movies | pageSize=5&pageNumber=1 | status=200, body_contains: {page:1, pageSize:5} |
| movies-page-beyond | GET /api/movies | pageSize=1&pageNumber=9999 | status=200, body_contains: {total: <any>}, body_min_items=0 |
| movies-by-id | GET /api/movies/{known_id} | — | status=200, has_keys: [id,title,year,runtime,genres,roles,rating,votes] |
| actors-default | GET /api/actors | — | status=200, has_keys: [items,total,page,pageSize] |
| actors-q-match | GET /api/actors | q=<2+ char name substring> | status=200 |
| actors-q-no-match | GET /api/actors | q=zzzzzzzzz | status=200, body_contains: {total: 0} |
| actors-pagination | GET /api/actors | pageSize=3&pageNumber=1 | status=200, body_contains: {pageSize: 3} |
| actors-by-id | GET /api/actors/{known_id} | — | status=200, has_keys: [id,name,birthYear,profession,movies] |
| metrics-exposition | GET /metrics | — | status=200, body_contains_string: "http_requests_total" |
| swagger-json | GET /swagger/v1/swagger.json | — | status=200, has_keys: [info,paths] |
| root-redirect | GET / | — | status=301 (or 3xx), Location header present |

### 7.2 Validation Failures (400s)

All must assert: status=400, has_keys: [error]

**`q` validation (length [2, 20]):**
| Scenario ID | Endpoint | Trigger |
|---|---|---|
| movies-q-too-short | GET /api/movies | q=a (1 char) |
| movies-q-too-long | GET /api/movies | q=<21-char string> |
| actors-q-too-short | GET /api/actors | q=x |
| actors-q-too-long | GET /api/actors | q=<21-char string> |

**`pageNumber` validation ([1, 10000]):**
| Scenario ID | Endpoint | Trigger |
|---|---|---|
| movies-pageno-zero | GET /api/movies | pageNumber=0 |
| movies-pageno-negative | GET /api/movies | pageNumber=-1 |
| movies-pageno-too-high | GET /api/movies | pageNumber=10001 |
| actors-pageno-zero | GET /api/actors | pageNumber=0 |
| actors-pageno-too-high | GET /api/actors | pageNumber=10001 |

**`pageSize` validation ([1, 1000]):**
| Scenario ID | Endpoint | Trigger |
|---|---|---|
| movies-pagesize-zero | GET /api/movies | pageSize=0 |
| movies-pagesize-too-high | GET /api/movies | pageSize=1001 |
| actors-pagesize-zero | GET /api/actors | pageSize=0 |
| actors-pagesize-too-high | GET /api/actors | pageSize=1001 |

**`year` validation ([1888, 2100]):**
| Scenario ID | Trigger |
|---|---|
| movies-year-too-low | year=1887 |
| movies-year-too-high | year=2101 |
| movies-year-non-integer | year=abc |

**`rating` validation ([1.0, 10.0]):**
| Scenario ID | Trigger |
|---|---|
| movies-rating-too-low | rating=0.9 |
| movies-rating-too-high | rating=10.1 |
| movies-rating-non-numeric | rating=great |

**`actorId` format (`^nm\d{5,9}$`):**
| Scenario ID | Trigger |
|---|---|
| movies-actorid-bad-prefix | actorId=tt0000001 |
| movies-actorid-too-short | actorId=nm1234 (4 digits) |
| movies-actorid-too-long | actorId=nm1234567890 (10 digits) |
| movies-actorid-not-numeric | actorId=nmABCDE |

**Path parameter ID format:**
| Scenario ID | Endpoint | Trigger |
|---|---|---|
| movies-id-bad-format | GET /api/movies/not-an-id | invalid path param |
| movies-id-nm-prefix | GET /api/movies/nm0000001 | actor ID used as movie ID |
| actors-id-bad-format | GET /api/actors/not-an-id | invalid path param |
| actors-id-tt-prefix | GET /api/actors/tt0000001 | movie ID used as actor ID |

### 7.3 Not-Found (404s)

| Scenario ID | Endpoint | Trigger |
|---|---|---|
| movies-id-not-found | GET /api/movies/tt9999999 | valid format, no record |
| actors-id-not-found | GET /api/actors/nm9999999 | valid format, no record |

### 7.4 Benchmark Scenarios (§10.4 targets)

Separate YAML file `scenarios/benchmark.yaml` with exactly two scenarios, looped at the configured RPS split:
- GET /api/movies (50% of load): p95 target < 50ms
- GET /api/movies/{id} (50% of load): p95 target < 10ms

The benchmark runner resolves `{id}` by fetching `/api/movies?pageSize=1` at startup and extracting the first movie ID.

---

## 8. Go Stdlib Packages Used

| Package | Use |
|---|---|
| `net/http` | HTTP client (`http.Client` with `Timeout`); response body reading |
| `encoding/json` | Decode response bodies into `map[string]any` for assertion |
| `flag` | CLI flag parsing |
| `sync` | `WaitGroup` for worker coordination |
| `sync/atomic` | `int64` counters (requests, errors, successes) — lock-free |
| `time` | `Duration`, `Ticker`, `Now()`/`Since()` for latency measurement |
| `sort` | `sort.Slice` for p95 percentile calculation |
| `math` | `math.Ceil` for percentile index |
| `context` | Per-request timeout (`context.WithTimeout`) and global benchmark duration |
| `os` | Exit codes (`os.Exit`), file reads for scenario YAML |
| `io` | `io.ReadAll` for response body capture |
| `fmt` | Output formatting |
| `strings` | String matching for header assertions, body_contains_string |
| `regexp` | Version semver assertion (`^\d+\.\d+\.\d+$`) |
| `path/filepath` | Glob expansion for `--scenarios` |

External library (one addition to `go.mod`):
- `gopkg.in/yaml.v3` — scenario file parsing (already a transitive dep of swag; check if it's already in go.sum)

---

## 9. Open Questions for Session 5

1. **Known IDs for positive path-param tests**: The scenario YAML can't hard-code a tt-ID from production data because the data set isn't a testing fixture. Options: (a) have the tool auto-discover a valid ID by calling `/api/movies?pageSize=1` at startup and storing the result as a template variable; (b) keep a small `scenarios/seed.yaml` that records known valid IDs from the `src/data/` files. Option (a) is self-contained; option (b) is fragile across data updates. **Recommend option (a).**

2. **actorId filter on `/api/movies`**: This filter is listed in §6 but explicitly deferred in the parking lot (`internal/server/movies.go:48`). Should the baseline scenario suite include it as a "currently returns 200 but filter is no-op" case, or skip it until implemented? Recommend: include a scenario that sends `actorId=<valid>`, asserts 200 (not 400), and documents the deferred behavior.

3. **`/readyz` pre-ready state**: The tool runs against a live in-cluster service that has already completed `store.Load`, so `/readyz` will be 200. No action needed; the 503-before-ready case is covered by the integration tests in `internal/server/integration_test.go` (which don't use SetReady).

4. **RPS split in benchmark**: The spec only names two endpoints for p95 targets. A 50/50 split between them is a reasonable default. If one endpoint's latency is more sensitive, the ratio can be tuned via a `weight` field in the benchmark scenario YAML.
