# Gap-Close Plan — Session 6 (2026-06-04)
# Target: 1.0.0

Source: 2026-06-04-gap-close-research.md
Items tagged PASS in that research are omitted here.
Ordered by dependency (code changes → infra changes → docs → live verification → acceptance).

---

## Task 1 — Bump version string to 1.0.0

**Files:** `cmd/moviesx/main.go` (lines 14, 17)

**Changes:**
- `var version = "0.4.0"` → `"1.0.0"`
- Swagger annotation `// @version 0.4.0` → `// @version 1.0.0`
- Run `make swagger` to regenerate `docs/` artifacts

**Exit criteria:**
- `GET /version` returns exactly `1.0.0`
- `docs/swagger.json` `.info.version` field reads `1.0.0`
- `make test` still passes

---

## Task 2 — Implement CLI flags (§11)

**Files:** `internal/config/config.go`, `cmd/moviesx/main.go`

**Changes:**
- Refactor `internal/config/config.go` to accept a parsed-flags struct (or accept `flag.FlagSet` values) so that CLI flags override env vars, which override built-in defaults. Precedence: defaults < env vars < CLI flags.
- In `cmd/moviesx/main.go`, register three flags before `config.Load()`:
  - `--movies-port` (default `8080`, maps to `MOVIES_PORT`)
  - `--movies-log-level` (default `info`, maps to `MOVIES_LOG_LEVEL`)
  - `--movies-data-dir` (default `/data`, maps to `MOVIES_DATA_DIR`)
- Implement `--version` / `-v`: print bare semver to stdout (no newline, no prefix), exit 0. Output must equal `GET /version` body byte-for-byte. Process before config loading.
- Implement `--help` / `-h`: print all flags with env-var names, defaults, and effective values, exit 0.
- Unknown flags or invalid values must exit non-zero before the HTTP listener starts.
- After `store.Load()` and before `server.SetReady()`, log the effective configuration (with no secret values; none exist here) once at `info` level.

**Exit criteria:**
- `./moviesx --version` prints `1.0.0` with no surrounding whitespace or newline
- `./moviesx --help` lists all three flags with their env-var equivalents and defaults
- `./moviesx --movies-port 9090` starts and listens on 9090
- `./moviesx --movies-port 9090 --movies-log-level debug` shows both overrides in startup log
- `./moviesx --unknown-flag` exits non-zero before binding any port
- `make test` still passes (integration tests use `httptest` — flag parsing must not break them)

---

## Task 3 — Fix resource limits in base deployment (§8.1)

**Files:** `k8s/base/deployment.yaml`, `k8s/overlays/dev/patches/dev-patch.yaml`

**Changes:**
- Add a `resources:` stanza to the `moviesx` container in `k8s/base/deployment.yaml`:
  ```yaml
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
  ```
- The dev overlay patch (`k8s/overlays/dev/patches/dev-patch.yaml`) already patches resources down to 50m/32Mi req, 200m/64Mi limit. That patch remains correct for low-resource local dev — no change needed there.

**Exit criteria:**
- `kubectl kustomize k8s/base/` output contains `cpu: 500m` and `memory: 512Mi` under `limits`
- `kubectl kustomize k8s/overlays/dev/` output still shows dev-overlay values (200m/64Mi limits)

---

## Task 4 — Create bench Kustomize overlay (§10.4)

**Files:** `k8s/overlays/bench/kustomization.yaml`

**Changes:**
- Create `k8s/overlays/bench/kustomization.yaml` that extends `k8s/base/` with:
  - Image tag pointing to the current release (e.g. `moviesx:1.0.0`)
  - No resource patch — inherits base values (100m/128Mi req, 500m/512Mi limit), matching the §10.4 "500m-CPU pod" condition
  - Optional: a distinct namespace or name suffix if isolation is needed

**Exit criteria:**
- `kubectl kustomize k8s/overlays/bench/` renders without error and shows `cpu: 500m` limit (from base, not patched down)
- CLAUDE.md benchmark usage section updated to reference `k8s/overlays/bench/` for accurate benchmark runs

---

## Task 5 — Add dependency scanning to Makefile (§13)

**Files:** `Makefile`

**Changes:**
- Add an `audit` target that runs `govulncheck ./...`
- `govulncheck` must be imported in `tools.go` (build-tag-isolated) so it can be invoked via `go run` without requiring a separate global install, OR documented as a required toolchain prerequisite in the dev-loop README
- Wire `audit` into the inner-loop README's "Run validation tests" step (step 6 in §12)

**Exit criteria:**
- `make audit` runs to completion (exit 0 on a clean module; any found vulnerabilities are logged as warnings and do not fail the build unless critical — follow `govulncheck` default exit-code behavior)
- The audit step appears in the implementation README (Task 6)

---

## Task 6 — Write implementation README with §12 inner-loop commands

**Files:** `docs/dev-loop.md` (new file), `README.md` (add link)

**Changes:**
Write `docs/dev-loop.md` covering every command from fresh clone to a successful Grafana dashboard view. The file must be self-contained: no step may depend on knowledge outside the file. Structure:

### Prerequisites section
- Required toolchain (versions): Go 1.23+, Docker, k3d, kubectl, kustomize, make
- `k3d` install one-liner

### One-time cluster setup
1. `k3d cluster create movies --port "8080:80@loadbalancer"` (or the port-forward equivalent given LB issue)
2. Install kube-prometheus-stack (verbatim `helm` or `kubectl apply` commands, including the values file)
3. Confirm Prometheus and Grafana pods are Running

### Inner loop (verbatim, §12 steps 1–7)
1. Make a change / bump version in `cmd/moviesx/main.go`
2. `make swagger` (if annotations changed)
3. `docker build -t moviesx:<version> .` — build image
4. `k3d image import moviesx:<version> -c movies` — load into cluster
5. Update image tag in `k8s/overlays/dev/kustomization.yaml`
6. `kubectl kustomize k8s/overlays/dev/ | docker exec -i k3d-movies-server-0 kubectl apply -f -`
7. `docker exec k3d-movies-server-0 kubectl rollout restart deployment/moviesx`
8. Verify: `GET /version`, `/healthz`, `/readyz` — verbatim wget commands
9. Run validation: `make replay-build && make e2e BASE_URL=http://<POD_IP>:8080` (or in-cluster equivalent)
10. Run audit: `make audit`
11. Inspect Grafana: verbatim URL + credential commands to confirm dashboard is visible

### Benchmark section
- When to use `k8s/overlays/bench/` instead of dev overlay
- Verbatim `make bench BASE_URL=...` command
- Expected output / pass criteria

**Exit criteria:**
- A fresh reader can follow `docs/dev-loop.md` from step 1 to a visible Grafana dashboard without consulting any other file
- `README.md` contains a link to `docs/dev-loop.md` in the "Read these in order" table or a "Development" section
- Every command in the file has been manually verified to work (confirmed during Task 7 live run)

---

## Task 7 — Live cluster verification (§14.2 and §14.5 UNKNOWN items)

**Scope:** Verify the two criteria that were PASS in code but UNKNOWN in cluster execution.

**Steps:**
1. Build and import `moviesx:1.0.0` image into k3d
2. Apply `k8s/overlays/dev/` and confirm pod reaches `Running`
3. Confirm `GET /version` returns `1.0.0`
4. Run in-cluster baseline replay suite:
   ```
   make replay-build
   docker cp bin/replay-linux k3d-movies-server-0:/tmp/replay
   docker cp scenarios/ k3d-movies-server-0:/tmp/scenarios
   docker exec k3d-movies-server-0 /tmp/replay \
     --base-url http://<POD_IP>:8080 \
     --scenarios /tmp/scenarios/baseline.yaml
   ```
   All 54 scenarios must pass.
5. Apply `k8s/overlays/bench/` and run benchmark:
   ```
   docker exec k3d-movies-server-0 /tmp/replay \
     --base-url http://<POD_IP>:8080 \
     --scenarios /tmp/scenarios/benchmark.yaml \
     --benchmark --duration 30s --rps 500 --concurrency 50
   ```
   p95 latency must be within acceptable range (not throttled — verify via bench overlay's 500m CPU limit).
6. Verify Grafana dashboard:
   - `GET /api/dashboards/uid/moviesx-overview` returns HTTP 200
   - At least one panel shows non-zero rate data after the replay load

**Exit criteria:**
- Baseline replay: 54/54 pass
- Benchmark replay: runs without error; p95 is not dominated by CPU throttle
- Grafana `moviesx-overview` dashboard loads and displays live request-rate data
- Any discovered bugs are fixed and re-verified before moving to Task 8

---

## Task 8 — Final §14 checklist verification pass

**Files:** `CLAUDE.md`, `session-log.md`, tag `1.0.0`

**Steps:**
Walk each §14 criterion and confirm pass with a specific command or artifact:

- [ ] **§14.1** — `docs/dev-loop.md` exists and covers fresh-clone → Grafana; linked from `README.md`
- [ ] **§14.2** — Baseline replay: 54/54 pass (from Task 7); benchmark: runs without throttle errors
- [ ] **§14.3** — `GET /metrics` contains `http_requests_total`, `http_request_duration_seconds_bucket`, `http_requests_in_flight`
- [ ] **§14.4** — Server startup log and request logs are valid JSON (spot-check with `| python3 -c "import sys,json; json.load(sys.stdin)"`)
- [ ] **§14.5** — Grafana `moviesx-overview` dashboard visible with live data (from Task 7)
- [ ] **§14.6** — `kubectl exec ... id` returns uid=65532 (nonroot); write to `/tmp` inside container fails (read-only FS)
- [ ] **§14.7** — Full inner-loop run per `docs/dev-loop.md` completes; `/version` returns semver matching git tag; tests pass; Grafana shows the run

**Exit criteria:**
- All 7 §14 checkboxes confirmed green with evidence
- `CLAUDE.md` updated for Session 6 / 1.0.0
- `session-log.md` close fields filled
- `git tag 1.0.0` applied

---

## Ordered Summary

| # | Task | Primary files | Unblocks |
|---|---|---|---|
| 1 | Bump version to 1.0.0 | `cmd/moviesx/main.go` | 2, 6, 7 |
| 2 | CLI flags (§11) | `internal/config/config.go`, `cmd/moviesx/main.go` | 6, 7 |
| 3 | Resource limits in base (§8.1) | `k8s/base/deployment.yaml` | 4, 7 |
| 4 | Bench Kustomize overlay (§10.4) | `k8s/overlays/bench/kustomization.yaml` | 7 |
| 5 | Dependency scanning (§13) | `Makefile` | 6, 7 |
| 6 | Implementation README (§12) | `docs/dev-loop.md`, `README.md` | 8 |
| 7 | Live cluster verification (§14.2, §14.5) | — (runtime check) | 8 |
| 8 | §14 final checklist + tag 1.0.0 | `CLAUDE.md`, `session-log.md` | — |
