# Gap-Close Research — 2026-06-04

Scope: Walk §14 acceptance checklist and related §11/§12/§13 requirements against current codebase.
Cluster state: not verified (k3d stack not exercised during this research pass; cluster-dependent items marked UNKNOWN).

---

## §14 Acceptance Criteria — Pass / Fail / Unknown

### Criterion 1 — Dev-loop steps documented in §12 bring up movies-api + Prometheus + Grafana on fresh local k3s

**Status: FAIL**

§12 (spec.md:285) states: "The implementation README documents every command in this loop verbatim. A fresh clone on a clean machine must reach a successful step 7 by following only that README."

- `README.md:70` says: "For the prompts and the 10-step inner loop, see [METHODOLOGY.md → Your Inner Loop, Step by Step]…" — points elsewhere; no verbatim commands present.
- `CLAUDE.md:130-161` has a detailed "Build & Deploy Inner Loop" section with verbatim kubectl/kustomize/docker commands — but CLAUDE.md is internal session memory, not the implementation README.
- Neither README.md nor any other committed file documents the cluster creation step (`k3d cluster create movies`), the kube-prometheus-stack install, or the one-time setup sequence a fresh clone would need.
- The Makefile (`Makefile:1-33`) has `make test`, `make build`, `make e2e`, `make bench`, `make swagger`, `make replay-build` but no `make dev-up` or equivalent bootstrapping target.

**Gap:** An implementation README (likely README.md, or a new DEVELOPMENT.md) with verbatim §12 loop commands does not exist.

---

### Criterion 2 — All §6 endpoints respond per contract; baseline + benchmark replay suites pass

**Status: PASS (code) / UNKNOWN (cluster execution)**

All §6 endpoints are implemented:

| Endpoint | File | Lines |
|---|---|---|
| `GET /version` | internal/server/server.go | 63–66 |
| `GET /healthz` | internal/server/server.go | 74–77 |
| `GET /readyz` | internal/server/server.go | 86–92 |
| `GET /api/genres` | internal/server/genres.go | 11–13 |
| `GET /api/movies` | internal/server/movies.go | 25–96 |
| `GET /api/movies/{id}` | internal/server/movies.go | 107–119 |
| `GET /api/actors` | internal/server/actors.go | 21–67 |
| `GET /api/actors/{id}` | internal/server/actors.go | 78–90 |
| `GET /metrics` | internal/server/server.go | 38 |
| `GET /swagger/…` | internal/server/server.go | 40–52 |

All query-parameter validation rules are implemented in `internal/validate/validate.go` and exercised by `internal/server/integration_test.go`.

Replay tool files present:
- `cmd/replay/` — main, scenario, runner, assert, assert_test
- `scenarios/baseline.yaml` — 54 scenarios (confirmed by CLAUDE.md:48)
- `scenarios/benchmark.yaml` — 2 scenarios
- `Makefile:28-32` — `make e2e` and `make bench` targets wired

Actual pass/fail against live cluster is UNKNOWN (needs in-cluster run per CLAUDE.md:118-130).

---

### Criterion 3 — `/metrics` exposes all §7.1 metrics with specified names and labels

**Status: PASS**

§7.1 (spec.md:178-180) is intentionally vague: "Exposed at `/metrics` in Prometheus text exposition format. Implementers should use the idiomatic Prometheus client library for their language." §1.1 (spec.md:38-40) explicitly calls metric names/labels/buckets "open design decisions."

Metrics implemented in `internal/server/metrics.go:10-26`:
- `http_requests_in_flight` (Gauge) — line 11
- `http_requests_total` (Counter, labels: `code`, `method`) — line 16
- `http_request_duration_seconds` (Histogram, labels: `handler`, `method`; 9 buckets from 1ms to 1s) — line 21

Integration test `TestMetrics_CounterIncrements` (internal/server/integration_test.go:674-695) confirms `http_requests_total{` appears in `/metrics` output.

---

### Criterion 4 — Logs are valid JSON (one object per line) with all required fields in §7.2

**Status: PASS**

§7.2 (spec.md:182-188): one JSON object per line; levels debug/info/warn/error; configurable via `MOVIES_LOG_LEVEL`; no PII; no request bodies.

Implementation:
- `cmd/moviesx/main.go:31` — `slog.NewJSONHandler(os.Stdout, ...)` — one JSON object per line guaranteed.
- `internal/server/logging.go:19-31` — per-request log with fields: `method`, `path`, `status`, `duration_ms`.
- `internal/config/config.go:14-21` — `MOVIES_LOG_LEVEL` parsed and applied to slog handler opts.

§7.2 says "any library is acceptable as long as the field names match" — it does not enumerate a fixed required-field list, so no specific name gap exists.

---

### Criterion 5 — Grafana dashboard auto-provisions and shows live data from Prometheus

**Status: PASS (config) / UNKNOWN (cluster execution)**

Provisioning path:
- `k8s/base/grafana-dashboard-configmap.yaml` — ConfigMap with dashboard JSON; label `grafana_dashboard: "1"` (line 7).
- `k8s/monitoring/kube-prom-values.yaml:40-46` — Grafana sidecar enabled; watches for `grafana_dashboard: "1"` label.
- `k8s/monitoring/kube-prom-values.yaml:35-38` — prometheus datasource configured automatically.

Metric queries in dashboard:
- Panel 1 (Request Rate): `sum by(handler, code) (rate(http_requests_total[5m]))` — targets implemented metric.
- Panel 2 (Latency p99): `histogram_quantile(0.99, sum by(handler, le) (rate(http_request_duration_seconds_bucket[5m])))` — targets implemented histogram.

ServiceMonitor scrape path: `k8s/base/servicemonitor.yaml` → Prometheus Operator → scrapes `/metrics` every 15s.

Whether auto-provision fires on the live cluster and live data appears is UNKNOWN (requires `kubectl` + Grafana API check).

---

### Criterion 6 — Container image runs as non-root with read-only root FS

**Status: PASS**

- `Dockerfile:11` — `USER nonroot:nonroot` (distroless nonroot user).
- `k8s/base/deployment.yaml:17-21` — Pod-level `securityContext`: `runAsNonRoot: true`, `runAsUser: 65532`, `seccompProfile: RuntimeDefault`.
- `k8s/base/deployment.yaml:28-32` — Container-level: `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop: [ALL]`.

Note: spec §8.1 says "uid 1000" but the distroless nonroot UID is 65532. Both are non-root; the implementation choice is defensible (distroless standard) and documented in CLAUDE.md:65.

---

### Criterion 7 — Inner-loop end-to-end on clean machine: build → deploy → `/version` semver → tests pass → visible on Grafana

**Status: FAIL (cascades from Criterion 1)**

The infrastructure (build, deploy, replay, Grafana) is all implemented. The blocking issue is the same as Criterion 1: §12 requires the implementation README to document every command verbatim, and it does not.

Additional sub-issue: the version string in `cmd/moviesx/main.go:14` is `"0.4.0"` — never bumped to `"0.5.0"` during Session 5. The swagger annotation at `main.go:17` also reads `0.4.0`. A clean end-to-end run would return `0.4.0` from `/version`, not the expected Session-5 semver. The Makefile `make build` target (line 15-16) does not pass `-ldflags` to inject version, so the binary always bakes in the hardcoded string.

---

## Additional Items

### CLI flags (§11: --movies-port, --movies-log-level, --movies-data-dir)

**Status: FAIL**

`internal/config/config.go:11-17` reads only environment variables via `os.Getenv()`. No `flag` package is imported anywhere in the config or main packages.

Spec §11 (spec.md:254-258) requires all three config tiers: defaults < env vars < CLI flags. CLAUDE.md:60 explicitly documents: "CLI flags deferred to a later session. Env-var config only for now."

This is an unimplemented §11 requirement.

---

### Implementation README documenting §12 inner-loop commands

**Status: FAIL**

`README.md` has 104 lines. Line 70 delegates to METHODOLOGY.md for the inner loop. No verbatim commands (k3d cluster create, docker build, kustomize, make e2e, etc.) appear in README.md or any other committed Markdown file outside CLAUDE.md.

CLAUDE.md is internal session memory and is not the "implementation README" §12 refers to.

---

### Dependency scanning (§13: "run locally as part of the inner loop")

**Status: FAIL**

`Makefile` has targets: `test`, `build`, `swagger`, `replay-build`, `e2e`, `bench`. No `go mod audit`, `govulncheck`, `go vet` (beyond what `go test` implies), or equivalent target exists.

Spec §13 (spec.md:294) requires: "Dependency scanning via the language's standard auditor (run locally as part of the inner loop)." Go's standard tooling would be `govulncheck ./...` or at minimum `go list -m all | nancy` / `osv-scanner`. None present.

---

### Resource limits vs §8.1 requirements

**Status: FAIL**

Spec §8.1 (spec.md:203-204):
- `resources.requests`: 100m CPU, 128Mi memory
- `resources.limits`: 500m CPU, 512Mi memory

Actual state:
- `k8s/base/deployment.yaml` — **no `resources:` stanza at all** (lines 22-53)
- `k8s/overlays/dev/patches/dev-patch.yaml:10-16` — dev overlay adds: requests 50m/32Mi, limits 200m/64Mi

Neither the base manifest nor the dev overlay meets the spec's resource requirements. In particular:
- Requests: 50m (not 100m), 32Mi (not 128Mi)
- Limits: 200m (not 500m), 64Mi (not 512Mi)

The 200m CPU limit at 500 RPS is documented in CLAUDE.md:81 as causing "throttle-induced p95 spikes" — this is a direct consequence of not meeting §8.1's 500m limit requirement.

---

### Benchmark CPU limit (parking lot)

**Status: Documented but unresolved**

CLAUDE.md:81 documents the issue and marks it as a future improvement. No bench-specific overlay exists under `k8s/overlays/`. The benchmark scenarios at 500 RPS would run against a pod throttled to 200m CPU in the dev overlay, violating spec §10.4's "500m-CPU pod" premise.

---

### Grafana admin password handling (§13 secret hygiene)

**Status: PASS**

`k8s/overlays/dev/grafana-admin-secret.yaml` delivers credentials as a Kubernetes `Secret`. Helm values reference it via `grafana.adminSecret: grafana-admin-secret` (kube-prom-values.yaml:37) — not baked into ConfigMaps or images. This satisfies §13's secret hygiene requirement.

---

### Version string

**Status: Mismatch — needs bump**

`cmd/moviesx/main.go:14` — `var version = "0.4.0"`
`cmd/moviesx/main.go:17` — `// @version 0.4.0` (Swagger annotation)

CLAUDE.md says Session 5 completed at tag `0.5.0`, but the version string was never bumped. `/version` and `/swagger/v1/swagger.json` would both report `0.4.0`.

---

## Summary

| Item | Status | Primary Evidence |
|---|---|---|
| §14.1 — Dev-loop in README | **FAIL** | README.md:70 delegates; no verbatim commands |
| §14.2 — §6 endpoints + replay | **PASS / UNKNOWN cluster** | All handlers implemented; needs live run |
| §14.3 — `/metrics` per §7.1 | **PASS** | §7.1 is intentionally open; impl provides 3 metrics |
| §14.4 — JSON logs per §7.2 | **PASS** | slog JSON handler; configurable level |
| §14.5 — Grafana auto-provisions | **PASS / UNKNOWN cluster** | ConfigMap + ServiceMonitor + sidecar configured |
| §14.6 — Non-root + read-only FS | **PASS** | Dockerfile:11, deployment.yaml:17-32 |
| §14.7 — Inner-loop end-to-end | **FAIL** | Cascades from §14.1; version string also stale |
| CLI flags (§11) | **FAIL** | config.go:11-17 env-only; flag package absent |
| Implementation README (§12) | **FAIL** | Same as §14.1 |
| Dependency scanning (§13) | **FAIL** | No govulncheck/audit target in Makefile |
| Resource limits (§8.1) | **FAIL** | Base deployment has no resources stanza; dev overlay: 200m/64Mi |
| Bench overlay (§10.4) | **FAIL** | No k8s/overlays/bench/; parking lot per CLAUDE.md:81 |
| Version string (0.5.0) | **FAIL** | main.go:14 still reads "0.4.0" |
| Grafana secret hygiene (§13) | **PASS** | Kubernetes Secret via existingSecret ref |

### Hard blockers for §14 acceptance (must fix)

1. **Implementation README** — Write verbatim §12 inner-loop commands (cluster create through Grafana check).
2. **CLI flags** — Implement `--movies-port`, `--movies-log-level`, `--movies-data-dir` in `internal/config/config.go` + `cmd/moviesx/main.go`.
3. **Resource limits** — Add `resources:` stanza to `k8s/base/deployment.yaml` (100m/128Mi requests, 500m/512Mi limits); adjust dev overlay to patch down as needed.
4. **Version string** — Bump `cmd/moviesx/main.go:14` and `main.go:17` to `"0.5.0"` (or `"1.0.0"` if this session is the acceptance pass).
5. **Dependency scanning** — Add `govulncheck ./...` (or `go vet ./...` + `go mod verify`) as a Makefile target; reference it in the inner-loop README.
6. **Bench overlay** — Create `k8s/overlays/bench/` with 500m CPU limit to match §10.4 target conditions.

### Items that are cluster-UNKNOWN (verify, don't rewrite)

- Actual replay baseline/benchmark pass/fail rates (Criterion 2)
- Grafana dashboard live data appearance (Criterion 5)
