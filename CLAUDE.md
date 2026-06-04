# CLAUDE.md — moviesx

Repo memory for the movies experiment. Updated at the close of each session.
Last updated: Session 6 (2026-06-04, tag 1.0.0).

## Stack

**Language:** Go (local toolchain: 1.26.3 via Homebrew; Docker builder pins `golang:1.25-bookworm`)  
**Module:** `github.com/mbr/moviesx`  
**Runtime image:** `gcr.io/distroless/static-debian12:nonroot` — static binary + data files baked at `/data`  
**Build:** `CGO_ENABLED=0 GOOS=linux go build ./cmd/moviesx`

## Project Layout

```
cmd/moviesx/main.go                       entry point — JSON slog, loads store, starts HTTP server
internal/config/config.go                 env-var config layer (MOVIES_PORT, MOVIES_LOG_LEVEL, MOVIES_DATA_DIR)
internal/server/server.go                 HTTP handler registration; returns (*server, http.Handler); server.SetReady() marks readiness
internal/server/metrics.go                Prometheus metric vars + instrumentedHandler wrapper
internal/server/logging.go                loggingMiddleware — per-request JSON log line
internal/server/genres.go                 GET /api/genres
internal/server/movies.go                 GET /api/movies, GET /api/movies/{id}
internal/server/actors.go                 GET /api/actors, GET /api/actors/{id}
internal/server/integration_test.go       end-to-end tests + /metrics smoke tests via httptest
internal/store/types.go                   data types: Movie, Actor, Role, Page[T], MovieListItem, MovieDetail
internal/store/store.go                   Load(dataDir) — parses data files, pre-joins ratings, pre-sorts
internal/validate/validate.go             input validation helpers (no net/http dependency)
docs/                                     Generated Swagger 2.0 spec (docs.go, swagger.json, swagger.yaml) — committed artifact
tools.go                                  Build-tag-isolated tool imports (swag CLI + http-swagger)
k8s/base/deployment.yaml                  Deployment (non-root; liveness on /healthz, readiness on /readyz; full securityContext)
k8s/base/service.yaml                     ClusterIP, port 80 → targetPort 8080 (named port: http)
k8s/base/servicemonitor.yaml              ServiceMonitor — port http, path /metrics, interval 15s
k8s/base/grafana-dashboard-configmap.yaml ConfigMap (monitoring ns) with moviesx-overview dashboard JSON
k8s/base/networkpolicy.yaml               NetworkPolicy: ingress from Traefik+Prometheus, egress to CoreDNS
k8s/base/kustomization.yaml               Kustomize base (5 resources)
k8s/overlays/dev/kustomization.yaml       Dev overlay: image tag 0.3.0, resource limits, grafana secret
k8s/overlays/dev/patches/dev-patch.yaml   Resource patch: cpu 50m/200m, memory 32Mi/64Mi
k8s/overlays/dev/grafana-admin-secret.yaml Secret: admin / dev-password (plaintext, dev only)
k8s/monitoring/kube-prom-values.yaml      Helm values for kube-prometheus-stack (k3d overrides)
src/data/                                 movies.json, actors.json, ratings.json (baked into image at /data)
Dockerfile                                multi-stage: golang:1.23-bookworm → distroless/static:nonroot
Makefile                                  make test, make build, make e2e, make bench, make replay-build
cmd/replay/main.go                        Replay tool CLI — flag parsing, mode dispatch
cmd/replay/scenario.go                    Scenario/AssertBlock structs + YAML loader + DiscoverIDs
cmd/replay/runner.go                      HTTP executor, RunBaseline, RunBenchmark
cmd/replay/assert.go                      8 assertion types (status, has_keys, body_contains, regex, etc.)
cmd/replay/assert_test.go                 16 unit tests for all assertion paths
scenarios/baseline.yaml                   54-scenario contract suite (happy-path + 400s + 404s)
scenarios/benchmark.yaml                  2-scenario sustained-load suite
```

## Config

| Env var | Default | Purpose |
|---|---|---|
| `MOVIES_PORT` | `8080` | HTTP listen port |
| `MOVIES_LOG_LEVEL` | `info` | Minimum log level (parsed via slog.Level.UnmarshalText) |
| `MOVIES_DATA_DIR` | `/data` | Data file directory |

CLI flags implemented in 1.0.0: `--movies-port`, `--movies-log-level`, `--movies-data-dir`. Precedence: defaults < env vars < CLI flags.

## Key Implementation Decisions

- **Version string:** `var version = "1.0.0"` in `cmd/moviesx/main.go` — overridable via `-ldflags "-X main.version=X.Y.Z"` in future builds.
- **`runAsUser: 65532`** in Deployment `securityContext` — distroless nonroot UID; required because Kubernetes `runAsNonRoot` validation needs a numeric UID, not just the named user `nonroot`.
- **`go.mod` pins `go 1.25.0`** — bumped from 1.23 by `go get golang.org/x/vuln` (govulncheck dep) which requires go 1.25. Docker builder image updated to match (`golang:1.25-bookworm`).
- **CLI flags (1.0.0):** `--movies-port`, `--movies-log-level`, `--movies-data-dir` override env vars which override defaults. `--version`/`-v` prints bare semver to stdout, exits 0. `--help`/`-h` shows all flags with env-var names, exits 0. Unknown flags exit 2. `config.Load()` now accepts a `config.Flags` struct.
- **govulncheck (1.0.0):** `make audit` installs govulncheck to GOPATH/bin and runs `./...`. Exit code 3 (vulnerabilities found) is treated as warning — make succeeds. Exit 1/2 (internal errors) fail the build. Two stdlib vulnerabilities in go1.26.3 vs go1.26.4 are expected on the local toolchain.
- **Bench overlay (1.0.0):** `k8s/overlays/bench/kustomization.yaml` inherits base resource limits (500m CPU / 512Mi) without dev-overlay reduction. Use this for accurate §10.4 benchmarks.
- **D1 — Response envelope:** All list endpoints return `{ "items": [...], "total": N, "page": N, "pageSize": N }`. Implemented as `Page[T any]` generic in `internal/store/types.go`.
- **D2 — Roles scope:** `GET /api/movies` list items include cast-only roles (`actor`/`actress` categories) in a `cast` field. `GET /api/movies/{id}` includes all roles in a `roles` field.
- **Store immutability:** `store.Store` has no exported mutable fields. `AllMovies()` and `AllActors()` return copies of the internal sorted slices.
- **Pre-sorted at load:** movies sorted (rating desc, title asc), actors sorted (name asc) at `Load` time; handlers iterate and filter without re-sorting.
- **actorId + rating filters on `/api/movies`:** Deferred to Parking Lot — not implemented in 0.2.0.
- **Metrics:** `http_requests_total` (labels: code, method), `http_request_duration_seconds` (labels: handler, method, le), `http_requests_in_flight`. `/metrics` endpoint is NOT wrapped in instrumentedHandler to avoid self-counting.
- **Readiness:** `GET /readyz` returns 503 until `server.SetReady()` is called (after `store.Load()`). `server.New()` returns `(*server, http.Handler)` — callers capture the `*server` to call `SetReady()`.
- **Swagger:** `docs/` generated by `make swagger` (swag v1 / Swagger 2.0). `_ "github.com/mbr/moviesx/docs"` import in main.go registers the spec. UI served by http-swagger at `/swagger/`; raw JSON at `/swagger/v1/swagger.json`. `GET /` redirects to `/swagger`.
- **NetworkPolicy:** `app.kubernetes.io/name: prometheus` is the correct label for the Prometheus pod in this cluster's kube-prometheus-stack install (NOT `app: prometheus`).
- **ServiceMonitor discovery:** Prometheus CR patched to `serviceMonitorSelector: {}` — picks up all ServiceMonitors without requiring a release label. Document this if reinstalling via Helm (set `serviceMonitorSelectorNilUsesHelmValues: false` in values).
- **Grafana dashboard provisioning:** The k3d dev cluster uses static ConfigMap volume mounts (`grafana-dashboard-movies`), not the sidecar label approach. Dashboard JSON was patched directly into that ConfigMap. The base `grafana-dashboard-configmap.yaml` is written for the sidecar approach (kube-prometheus-stack install).
- **Data files in image:** Dockerfile copies `src/data/` → `/data` in the final image via `COPY --from=builder /src/src/data /data`. Required because the `moviesx:dev` image predates this — it was a session-1 binary without store.Load.
- **securityContext (0.4.0):** Container-level adds `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop: [ALL]`. Pod-level adds `seccompProfile: RuntimeDefault`. No emptyDir needed — binary makes no filesystem writes.
- **Replay tool (0.5.0):** Lives in `cmd/replay/`. Scenario format: YAML with `scenarios:` list. Template vars `{known_movie_id}` and `{known_actor_id}` discovered at startup via `/api/movies` and `/api/actors`. 8 assertion types. Benchmark: 50-worker goroutine pool, `time.NewTicker` rate control, p95 via sort+index. 54-scenario baseline suite (all §6 endpoints + all validation 400s + 404s), 2-scenario benchmark suite.
- **Benchmark CPU limit:** Dev overlay caps moviesx at 200m CPU; at 500 RPS this causes throttle-induced p95 spikes. Use `k8s/overlays/bench/` (inherits base 500m limit) for accurate benchmark runs. Verified in 1.0.0: p95=0.6ms at 500 RPS under bench overlay.
- **Replay macOS reachability:** k3d pod IPs (10.42.x.x) are not directly reachable from macOS host. Run the replay binary inside the k3d container: `GOOS=linux go build -o bin/replay-linux ./cmd/replay && docker cp bin/replay-linux k3d-movies-server-0:/tmp/replay && docker exec k3d-movies-server-0 /tmp/replay --base-url http://<POD_IP>:8080 --scenarios /tmp/scenarios/baseline.yaml`

## Local Cluster

- **Cluster:** k3d cluster named `movies`
- **Known issue:** k3d load-balancer failed to bind host port 3000. The LB pod is in `Pending`/`Error` state. This does not affect the API — use `kubectl port-forward` or `docker exec` for access.
- **kubectl access:** `docker exec k3d-movies-server-0 kubectl <args>` (avoids needing the LB); host `kubectl` is configured as context `k3d-movies` but `port-forward` doesn't bind host ports in this environment.
- **Direct pod access:** `docker exec k3d-movies-server-0 wget -qO- http://<POD_IP>:8080/...`
- **Image import:** `k3d image import moviesx:<tag> -c movies`
- **Monitoring:** Prometheus Operator in `default` ns; Prometheus + Grafana in `monitoring` ns; Helm not required — stack was installed manually during research session.

## Testing

```
make test                          # all tests + coverage gate (≥80%)
go test ./...                      # all tests without coverage gate
go test ./internal/config/...      # config unit tests
go test ./internal/store/...       # store unit tests
go test ./internal/validate/...    # validation unit tests
go test ./internal/server/...      # server unit + integration tests (includes /metrics smoke tests)
```

Coverage as of 1.0.0: **95.3%** total (gate: 80%; replay tool covered by `cmd/replay/assert_test.go`).

## Replay Tool Usage

```bash
# Build Linux binary for in-cluster execution
make replay-build                          # produces bin/replay-linux (GOOS=linux)

# Copy scenarios + binary into k3d container
docker cp bin/replay-linux k3d-movies-server-0:/tmp/replay
docker cp scenarios/ k3d-movies-server-0:/tmp/scenarios

POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx -o jsonpath='{.items[0].status.podIP}')

# Baseline (functional contract suite — 54 scenarios)
docker exec k3d-movies-server-0 /tmp/replay \
  --base-url http://$POD_IP:8080 \
  --scenarios /tmp/scenarios/baseline.yaml

# Benchmark (500 RPS, 30s — remove CPU limit first for accurate results)
docker exec k3d-movies-server-0 /tmp/replay \
  --base-url http://$POD_IP:8080 \
  --scenarios /tmp/scenarios/benchmark.yaml \
  --benchmark --duration 30s --concurrency 50 --rps 500
```

## Build & Deploy Inner Loop (Session 6 — 1.0.0)

See full step-by-step guide: [docs/dev-loop.md](docs/dev-loop.md)

```bash
make test
make swagger                              # regenerate docs/ if annotations changed
docker build -t moviesx:1.0.0 .
k3d image import moviesx:1.0.0 -c movies

# Apply via Kustomize dev overlay (pipes through host kubectl because k3d exec doesn't need kubeconfig)
kubectl kustomize k8s/overlays/dev/ | docker exec -i k3d-movies-server-0 kubectl apply -f -

# Restart to pick up new image
docker exec k3d-movies-server-0 kubectl rollout restart deployment/moviesx

# Verify
docker exec k3d-movies-server-0 kubectl get pods
POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx -o jsonpath='{.items[0].status.podIP}')
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/version
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/healthz
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/readyz
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/swagger/v1/swagger.json | python3 -c "import sys,json; print(json.load(sys.stdin)['info']['title'])"
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/metrics | grep http_requests_total | head -5
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/api/genres

# Prometheus target check
PROM_IP=$(docker exec k3d-movies-server-0 kubectl get pod -n monitoring -l app=prometheus -o jsonpath='{.items[0].status.podIP}')
docker exec k3d-movies-server-0 wget -qO- "http://$PROM_IP:9090/api/v1/targets" | python3 -c "import sys,json; [print(t['labels'].get('job'), t['health']) for t in json.load(sys.stdin)['data']['activeTargets'] if 'moviesx' in str(t['labels'])]"

# Grafana dashboard check
GRAF_IP=$(docker exec k3d-movies-server-0 kubectl get pod -n monitoring -l app.kubernetes.io/name=grafana -o jsonpath='{.items[0].status.podIP}' 2>/dev/null || docker exec k3d-movies-server-0 kubectl get pod -n monitoring -l app=grafana -o jsonpath='{.items[0].status.podIP}')
docker exec k3d-movies-server-0 wget -qO- "http://$GRAF_IP:3000/api/dashboards/uid/moviesx-overview"
```

## Session Map

| Tag | Goal | Status |
|---|---|---|
| `0.1.0` | Stack + /version + /healthz on k3s | ✅ done |
| `0.2.0` | Data layer + /api/* endpoints + validation + tests ≥80% | ✅ done |
| `0.3.0` | Prometheus metrics + structured logging + Grafana + Kustomize | ✅ done |
| `0.4.0` | OpenAPI/Swagger + /readyz + security hardening + NetworkPolicy | ✅ done |
| `0.5.0` | Custom HTTP replay tool (§10.3 baseline + §10.4 benchmark) | ✅ done |
| `1.0.0` | §14 gap close + inner-loop README + acceptance pass | ✅ done |

## Spec References

- Full spec: [docs/spec.md](docs/spec.md)
- Acceptance criteria: [docs/spec.md §14](docs/spec.md#14-acceptance-criteria)
- Methodology: [docs/METHODOLOGY.md](docs/METHODOLOGY.md)
- Session log: [session-log.md](session-log.md)
- RPI artifacts: [.copilot-tracking/](.copilot-tracking/)
