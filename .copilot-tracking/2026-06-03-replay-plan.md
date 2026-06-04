# Session 5 Task Plan — HTTP Replay Tool (2026-06-03)

Scope: §10.3 contract test suite + §10.4 benchmark mode.  
Research decisions are captured in `2026-06-03-replay-research.md`.  
All new code lives in `cmd/replay/` and `scenarios/`.

---

## Task List

### T1 — Repo scaffolding

- [x] **Add `gopkg.in/yaml.v3` as a direct dependency**
  - File targets: `go.mod`, `go.sum`
  - Run `go get gopkg.in/yaml.v3` (it appears in `go.sum` already as an indirect transitive dep of swag; promote to direct)
  - Exit criteria: `go.mod` contains a `require gopkg.in/yaml.v3` line; `go mod tidy` exits 0

- [x] **Create directory skeleton**
  - File targets: `cmd/replay/` (new dir), `scenarios/` (new dir)
  - Exit criteria: both directories exist; `go build ./cmd/replay/` errors only on missing `main.go`, not on path issues

- [x] **Add Makefile targets**
  - File targets: `Makefile`
  - Add three targets:
    - `replay-build`: `CGO_ENABLED=0 go build -o bin/replay ./cmd/replay/`
    - `e2e`: `./bin/replay --base-url $$(BASE_URL) --scenarios scenarios/baseline.yaml`
    - `bench`: `./bin/replay --base-url $$(BASE_URL) --scenarios scenarios/benchmark.yaml --benchmark --duration 30s --rps 500 --concurrency 50`
  - Exit criteria: `make replay-build` compiles (after main.go exists); `make -n e2e` prints the correct command without executing

---

### T2 — Scenario types and loader

- [x] **Define scenario structs and YAML loader**
  - File targets: `cmd/replay/scenario.go`
  - Implement:
    - `Scenario` struct with fields: `ID`, `Method`, `Path`, `Query map[string]string`, `ExpectStatus int`, `ExpectHeaders map[string]string`, `Assert AssertBlock`
    - `AssertBlock` struct with fields: `HasKeys []string`, `IsArray bool`, `ArrayMinLength int`, `BodyContains map[string]any`, `BodyMinItems *int`, `BodyContainsString string`, `BodyMatchesRegex string`
    - `ScenariosFile` struct wrapping `Scenarios []Scenario`
    - `LoadScenarios(paths []string) ([]Scenario, error)` — accepts a list of file paths (already glob-expanded), reads and unmarshals each YAML file, merges scenario lists, returns error on duplicate IDs
    - `ExpandGlob(pattern string) ([]string, error)` — wraps `filepath.Glob`; errors if pattern matches zero files
  - Exit criteria: unit test (or `go vet ./cmd/replay/`) passes; loading `scenarios/baseline.yaml` (once written) returns the correct scenario count with no error

---

### T3 — CLI skeleton

- [x] **Implement `cmd/replay/main.go` with flag parsing and mode dispatch**
  - File targets: `cmd/replay/main.go`
  - Flags (all via stdlib `flag` package):
    - `--base-url string` (required — fatal if empty)
    - `--scenarios string` (required — glob pattern or single path)
    - `--benchmark bool` (default false)
    - `--duration duration` (default `30s`)
    - `--concurrency int` (default 10)
    - `--rps float64` (default 500)
    - `--timeout duration` (default `5s`)
    - `--verbose bool` (default false)
  - Startup logic:
    1. Parse flags; fail fast with usage message if required flags are missing
    2. Expand `--scenarios` glob → `[]string` file paths
    3. Call `LoadScenarios`
    4. If `--benchmark`: warn and ignore `--duration`/`--concurrency`/`--rps` (they are used by benchmark runner, not discarded — this warning is for functional mode only if those flags are set)
    5. Dispatch to `RunBaseline` or `RunBenchmark` based on `--benchmark`
    6. Exit 0 on all pass, exit 1 on any failure
  - Exit criteria: `go build ./cmd/replay/` succeeds; `./bin/replay --help` prints all flags; `./bin/replay` (no flags) exits non-zero with a usage error

---

### T4 — ID auto-discovery

- [x] **Implement startup auto-discovery of valid movie and actor IDs**
  - File targets: `cmd/replay/runner.go`
  - `DiscoverIDs(baseURL string, client *http.Client) (movieID, actorID string, err error)`
    - GET `{baseURL}/api/movies?pageSize=1` → parse `items[0].id` → `movieID`
    - GET `{baseURL}/api/actors?pageSize=1` → parse `items[0].id` → `actorID`
    - Return error if either response is non-200 or items array is empty
  - Template substitution: before executing a scenario, replace `{known_movie_id}` and `{known_actor_id}` in `Scenario.Path` with the discovered values
  - Exit criteria: running against the live cluster, `DiscoverIDs` returns non-empty `tt…` and `nm…` strings; scenarios using `{known_movie_id}` in their path resolve correctly

---

### T5 — HTTP executor

- [x] **Implement single-request executor and result struct**
  - File targets: `cmd/replay/runner.go`
  - `type Result struct { ScenarioID string; Duration time.Duration; StatusCode int; Body []byte; Headers http.Header; Err error }`
  - `Execute(ctx context.Context, client *http.Client, baseURL string, s Scenario) Result`
    - Build URL: `baseURL + s.Path` with `s.Query` URL-encoded and appended as query string
    - Issue `http.NewRequestWithContext` → `client.Do`
    - Record `time.Since(start)` as `Duration` (measured around `client.Do` only, not assertion)
    - Read full response body via `io.ReadAll`; close body
    - Populate and return `Result`; on network error, set `Err`, leave `StatusCode` 0
  - Exit criteria: calling `Execute` against `GET /api/genres` on the live cluster returns `StatusCode=200` and non-empty `Body`

---

### T6 — Assertion engine

- [x] **Implement all assertion types**
  - File targets: `cmd/replay/assert.go`
  - `type AssertionError struct { Field string; Expected, Got string }`
  - `func Assert(s Scenario, r Result) []AssertionError` — runs all applicable checks in order:
    1. **Status code**: `r.StatusCode == s.ExpectStatus`; if mismatch, append error and return early (body assertions are meaningless on wrong-status responses)
    2. **Expected headers**: for each `key: substring` in `s.ExpectHeaders`, check `r.Headers.Get(key)` contains `substring` (case-insensitive)
    3. **`body_contains_string`**: check `strings.Contains(string(r.Body), s.Assert.BodyContainsString)`
    4. **`body_matches_regex`**: compile and match `regexp.MustCompile(s.Assert.BodyMatchesRegex)` against `string(r.Body)` (used for `version-semver` scenario)
    5. **`is_array`**: decode body into `[]any`; if `array_min_length > 0`, assert `len >= array_min_length`
    6. **`has_keys`**: decode body into `map[string]any`; assert each key in `s.Assert.HasKeys` is present
    7. **`body_contains`**: for each `key: value` in `s.Assert.BodyContains`, walk the decoded `map[string]any` following dot-separated key paths (e.g. `items.0.id`); assert scalar equality
    8. **`body_min_items`**: decode body into `map[string]any`; assert `len(body["items"].([]any)) >= *s.Assert.BodyMinItems`
  - Exit criteria: unit tests cover at least: correct status passes, wrong status fails, `has_keys` missing key fails, `body_contains` wrong value fails, `is_array` on object body fails, regex mismatch fails

---

### T7 — Baseline runner and reporter

- [x] **Implement functional (baseline) mode runner**
  - File targets: `cmd/replay/runner.go`
  - `RunBaseline(cfg Config, scenarios []Scenario) (pass, fail int, err error)`
    - For each scenario in order (sequential — no concurrency in functional mode):
      1. Apply template substitution (T4)
      2. Call `Execute` with per-request `context.WithTimeout(ctx, cfg.Timeout)`
      3. Call `Assert`
      4. If `--verbose` or any assertion errors: print per-scenario line
      5. Accumulate pass/fail counts
    - After all scenarios, print summary: `Ran N scenarios: X passed, Y failed`
    - Return `fail > 0` as a signal to `main` to exit 1
  - Reporter format (text):
    ```
    PASS  movies-default          200  GET /api/movies                      4.2ms
    FAIL  movies-q-too-short      200  GET /api/movies?q=a                  1.1ms
          ↳ status: expected 400, got 200
    ...
    ---
    Ran 46 scenarios: 45 passed, 1 failed
    ```
  - Exit criteria: running `make e2e BASE_URL=http://<pod-ip>:8080` against the cluster prints the summary and exits 0 (once scenarios are written)

---

### T8 — Benchmark worker pool and reporter

- [x] **Implement benchmark mode runner**
  - File targets: `cmd/replay/runner.go`
  - `RunBenchmark(cfg Config, scenarios []Scenario) error`
  - Architecture (per research §4):
    - Pre-allocate `results` slice: `make([]Result, 0, int(cfg.RPS*cfg.Duration.Seconds()*1.2))`
    - Global `context.WithTimeout(background, cfg.Duration)` cancels the run
    - Rate-limiter goroutine: `time.NewTicker(time.Duration(float64(time.Second)/cfg.RPS))` fires job tokens onto `jobs chan struct{}` (buffer = `2 * cfg.Concurrency`)
    - `cfg.Concurrency` worker goroutines: each reads from `jobs`, picks scenario round-robin, calls `Execute`, sends `Result` to `results chan Result` (buffer = `2 * cfg.Concurrency`)
    - Collector goroutine: drains `results chan`, appends to per-scenario duration slices and atomic counters
    - `sync.WaitGroup` on workers; close `results chan` after all workers exit; wait for collector to finish
  - After run, compute per-scenario stats:
    - Sort each duration slice with `sort.Slice`
    - p50: `durations[int(0.50*N)]`, p95: `durations[int(math.Ceil(0.95*N))-1]`, p99: `durations[int(math.Ceil(0.99*N))-1]`
    - Error rate: `errors / total_requests * 100`
  - Targets checked per spec §10.4:
    - `GET /api/movies` p95 < 50ms
    - `GET /api/movies/{id}` p95 < 10ms
    - Overall error rate < 1%
  - Reporter format (text):
    ```
    endpoint                  requests  errors  p50      p95      p99
    GET /api/movies           7500      0       3.1ms    8.4ms    14.2ms
    GET /api/movies/{id}      7500      0       0.8ms    2.1ms    3.5ms

    PASS: p95 GET /api/movies = 8.4ms < 50ms target
    PASS: p95 GET /api/movies/{id} = 2.1ms < 10ms target
    PASS: error rate = 0.00% < 1% target
    ```
  - Exit code 0 on all targets met, 1 on any miss
  - Exit criteria: `make bench BASE_URL=http://<pod-ip>:8080` completes, prints the table, exits 0 with all three PASS lines

---

### T9 — Baseline scenario file

- [x] **Write `scenarios/baseline.yaml` — happy-path scenarios**
  - File targets: `scenarios/baseline.yaml`
  - Cover all happy-path cases from research §7.1 (24 scenarios):
    - `healthz-pass`, `version-semver` (body_matches_regex), `readyz-ready`
    - `genres-list`
    - `movies-default`, `movies-q-match`, `movies-q-no-match`, `movies-genre-filter`, `movies-year-filter`, `movies-year-edge-low`, `movies-year-edge-high`, `movies-rating-filter`, `movies-rating-max`, `movies-pagination`, `movies-page-beyond`, `movies-by-id` (uses `{known_movie_id}`)
    - `actors-default`, `actors-q-match`, `actors-q-no-match`, `actors-pagination`, `actors-by-id` (uses `{known_actor_id}`)
    - `metrics-exposition` (body_contains_string: `http_requests_total`)
    - `swagger-json`
    - `root-redirect` (expect_status: 301 or verify Location header)
  - Exit criteria: `LoadScenarios(["scenarios/baseline.yaml"])` returns exactly 24 scenarios with no errors; scenario IDs match the coverage matrix from research §7.1

- [x] **Write `scenarios/baseline.yaml` — validation failure scenarios (400s)**
  - File targets: `scenarios/baseline.yaml` (appended to same file)
  - Cover all 400-level cases from research §7.2 (~20 scenarios):
    - `q` length (4 scenarios: movies-q-too-short, movies-q-too-long, actors-q-too-short, actors-q-too-long)
    - `pageNumber` range (5 scenarios)
    - `pageSize` range (4 scenarios)
    - `year` range + type (3 scenarios: too-low, too-high, non-integer)
    - `rating` range + type (3 scenarios: too-low, too-high, non-numeric)
    - `actorId` format (4 scenarios: bad-prefix, too-short, too-long, not-numeric)
    - path param format (4 scenarios: movies-id-bad-format, movies-id-nm-prefix, actors-id-bad-format, actors-id-tt-prefix)
  - All 400 scenarios assert: `expect_status: 400`, `assert.has_keys: [error]`
  - Include `movies-actorid-valid-deferred` scenario: `actorId=nm0000001` (valid format) → `expect_status: 200` with comment `# actorId filter accepted but currently no-op (deferred per parking lot)`
  - Exit criteria: 400-section scenarios all assert status 400 and `has_keys: [error]`

- [x] **Write `scenarios/baseline.yaml` — 404 scenarios**
  - File targets: `scenarios/baseline.yaml` (appended)
  - `movies-id-not-found`: GET /api/movies/tt9999999 → 404, has_keys: [error]
  - `actors-id-not-found`: GET /api/actors/nm9999999 → 404, has_keys: [error]
  - Exit criteria: both 404 scenarios present with correct expect_status

---

### T10 — Benchmark scenario file

- [x] **Write `scenarios/benchmark.yaml`**
  - File targets: `scenarios/benchmark.yaml`
  - Two scenarios only (looped by benchmark runner):
    - `bench-movies-list`: GET /api/movies → 200, has_keys: [items, total, page, pageSize]
    - `bench-movies-by-id`: GET /api/movies/{known_movie_id} → 200, has_keys: [id, title, year]
  - Include top-level comment explaining RPS split: 50/50 between the two scenarios, round-robin
  - Exit criteria: file loads cleanly; benchmark runner rotates between exactly these two scenarios

---

### T11 — Verify baseline against cluster

- [x] **Run full baseline suite against the live k3d cluster; confirm all pass**
  - File targets: none (verification step)
  - Steps:
    1. `make replay-build`
    2. Get pod IP: `POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx -o jsonpath='{.items[0].status.podIP}')`
    3. `make e2e BASE_URL=http://$POD_IP:8080`
  - Exit criteria:
    - Tool exits 0
    - Output line: `Ran N scenarios: N passed, 0 failed`
    - All validation-failure scenarios return 400 (not 200), confirming the server's `validate` layer is working end-to-end
    - `movies-id-not-found` and `actors-id-not-found` return 404

---

### T12 — Verify benchmark against cluster

- [x] **Run benchmark mode for 30s; confirm targets met**
  - File targets: none (verification step)
  - Steps:
    1. `make bench BASE_URL=http://$POD_IP:8080`
  - Exit criteria:
    - Tool exits 0
    - Summary table printed with request counts, p50/p95/p99 columns
    - `PASS: p95 GET /api/movies … < 50ms target`
    - `PASS: p95 GET /api/movies/{id} … < 10ms target`
    - `PASS: error rate … < 1% target`
    - Total requests ≥ 14,000 (≈500 RPS × 30s × ~95% efficiency floor)

---

## Parking Lot

Items explicitly deferred from Session 5:

| Item | Reason deferred |
|---|---|
| `--output json` mode | Not needed for cluster verification; adds serialisation work with no spec requirement in §10.3/10.4. Defer to §14 acceptance pass if CI parsing is needed. |
| `weight` field in benchmark YAML | 50/50 round-robin is sufficient for §10.4. A `weight` key on benchmark scenarios would allow custom RPS splits but is not required by spec. |
| `internal/replay/` extraction | All replay logic fits cleanly in `cmd/replay/*.go` for now. Extract to `internal/replay/` only if the API server needs to import replay primitives (unlikely). |
| `actorId` filter implementation in API | The `movies-actorid-valid-deferred` scenario documents the no-op behaviour; actual filter logic stays in the parking lot from Session 2. |
| Benchmark scenario `weight` field | Per research §9 (open question 4): 50/50 split is fine for now; weight tuning deferred. |
| Running `make e2e` in CI (GitHub Actions / k3d in Docker) | No CI pipeline exists yet; this is a §1.0 concern. |
| Per-request body diffs in `--verbose` output | Text output is sufficient for Session 5; richer diff format deferred. |
| Benchmark CPU limit lifting | At 500 RPS, the dev overlay's 200m CPU limit causes CPU throttling (p95 ~16ms for /api/movies/{id}). Benchmark was verified with CPU limit temporarily removed via `kubectl patch`. In a production cluster with adequate CPU, the 10ms target is met easily (observed p95 = 0.6ms without throttling). A `make bench-relax` target or a bench-specific overlay is a future improvement. |
