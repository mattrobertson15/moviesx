# CLAUDE.md — moviesx

Repo memory for the movies experiment. Updated at the close of each session.
Last updated: Session 3 (2026-06-03, tag 0.3.0).

## Stack

**Language:** Go (local toolchain: 1.26.3 via Homebrew; Docker builder pins `golang:1.23-bookworm`)  
**Module:** `github.com/mbr/moviesx`  
**Runtime image:** `gcr.io/distroless/static-debian12:nonroot` — static binary + data files baked at `/data`  
**Build:** `CGO_ENABLED=0 GOOS=linux go build ./cmd/moviesx`

## Project Layout

```
cmd/moviesx/main.go                       entry point — JSON slog, loads store, starts HTTP server
internal/config/config.go                 env-var config layer (MOVIES_PORT, MOVIES_LOG_LEVEL, MOVIES_DATA_DIR)
internal/server/server.go                 HTTP handler registration; returns http.Handler (wraps mux in logging middleware)
internal/server/metrics.go                Prometheus metric vars + instrumentedHandler wrapper
internal/server/logging.go                loggingMiddleware — per-request JSON log line
internal/server/genres.go                 GET /api/genres
internal/server/movies.go                 GET /api/movies, GET /api/movies/{id}
internal/server/actors.go                 GET /api/actors, GET /api/actors/{id}
internal/server/integration_test.go       end-to-end tests + /metrics smoke tests via httptest
internal/store/types.go                   data types: Movie, Actor, Role, Page[T], MovieListItem, MovieDetail
internal/store/store.go                   Load(dataDir) — parses data files, pre-joins ratings, pre-sorts
internal/validate/validate.go             input validation helpers (no net/http dependency)
k8s/base/deployment.yaml                  Deployment (non-root, liveness/readiness on /healthz)
k8s/base/service.yaml                     ClusterIP, port 80 → targetPort 8080 (named port: http)
k8s/base/servicemonitor.yaml              ServiceMonitor — port http, path /metrics, interval 15s
k8s/base/grafana-dashboard-configmap.yaml ConfigMap (monitoring ns) with moviesx-overview dashboard JSON
k8s/base/kustomization.yaml               Kustomize base (4 resources)
k8s/overlays/dev/kustomization.yaml       Dev overlay: image tag 0.3.0, resource limits, grafana secret
k8s/overlays/dev/patches/dev-patch.yaml   Resource patch: cpu 50m/200m, memory 32Mi/64Mi
k8s/overlays/dev/grafana-admin-secret.yaml Secret: admin / dev-password (plaintext, dev only)
k8s/monitoring/kube-prom-values.yaml      Helm values for kube-prometheus-stack (k3d overrides)
src/data/                                 movies.json, actors.json, ratings.json (baked into image at /data)
Dockerfile                                multi-stage: golang:1.23-bookworm → distroless/static:nonroot
Makefile                                  make test (runs tests + coverage gate), make build
```

## Config

| Env var | Default | Purpose |
|---|---|---|
| `MOVIES_PORT` | `8080` | HTTP listen port |
| `MOVIES_LOG_LEVEL` | `info` | Minimum log level (parsed via slog.Level.UnmarshalText) |
| `MOVIES_DATA_DIR` | `/data` | Data file directory |

CLI flags deferred to a later session. Env-var config only for now.

## Key Implementation Decisions

- **Version string:** `var version = "0.3.0"` in `cmd/moviesx/main.go` — overridable via `-ldflags "-X main.version=X.Y.Z"` in future builds.
- **`runAsUser: 65532`** in Deployment `securityContext` — distroless nonroot UID; required because Kubernetes `runAsNonRoot` validation needs a numeric UID, not just the named user `nonroot`.
- **`go.mod` pins `go 1.23.0`** — bumped from 1.22 by `go get prometheus/client_golang@v1.23.2` which requires go 1.23. Docker builder image updated to match (`golang:1.23-bookworm`).
- **D1 — Response envelope:** All list endpoints return `{ "items": [...], "total": N, "page": N, "pageSize": N }`. Implemented as `Page[T any]` generic in `internal/store/types.go`.
- **D2 — Roles scope:** `GET /api/movies` list items include cast-only roles (`actor`/`actress` categories) in a `cast` field. `GET /api/movies/{id}` includes all roles in a `roles` field.
- **Store immutability:** `store.Store` has no exported mutable fields. `AllMovies()` and `AllActors()` return copies of the internal sorted slices.
- **Pre-sorted at load:** movies sorted (rating desc, title asc), actors sorted (name asc) at `Load` time; handlers iterate and filter without re-sorting.
- **actorId + rating filters on `/api/movies`:** Deferred to Parking Lot — not implemented in 0.2.0.
- **Metrics:** `http_requests_total` (labels: code, method), `http_request_duration_seconds` (labels: handler, method, le), `http_requests_in_flight`. `/metrics` endpoint is NOT wrapped in instrumentedHandler to avoid self-counting.
- **ServiceMonitor discovery:** Prometheus CR patched to `serviceMonitorSelector: {}` — picks up all ServiceMonitors without requiring a release label. Document this if reinstalling via Helm (set `serviceMonitorSelectorNilUsesHelmValues: false` in values).
- **Grafana dashboard provisioning:** The k3d dev cluster uses static ConfigMap volume mounts (`grafana-dashboard-movies`), not the sidecar label approach. Dashboard JSON was patched directly into that ConfigMap. The base `grafana-dashboard-configmap.yaml` is written for the sidecar approach (kube-prometheus-stack install).
- **Data files in image:** Dockerfile copies `src/data/` → `/data` in the final image via `COPY --from=builder /src/src/data /data`. Required because the `moviesx:dev` image predates this — it was a session-1 binary without store.Load.

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

Coverage as of 0.3.0: **90.3%** total (gate: 80%).

## Build & Deploy Inner Loop (Session 3 — Kustomize)

```bash
make test
docker build -t moviesx:0.3.0 .
k3d image import moviesx:0.3.0 -c movies

# Apply via Kustomize dev overlay (pipes through host kubectl because k3d exec doesn't need kubeconfig)
kubectl kustomize k8s/overlays/dev/ | docker exec -i k3d-movies-server-0 kubectl apply -f -

# Restart to pick up new image
docker exec k3d-movies-server-0 kubectl rollout restart deployment/moviesx

# Verify
docker exec k3d-movies-server-0 kubectl get pods
POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx -o jsonpath='{.items[0].status.podIP}')
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/version
docker exec k3d-movies-server-0 wget -qO- http://$POD_IP:8080/healthz
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
| `0.4.0` | OpenAPI/Swagger + /readyz + security hardening + NetworkPolicy | — |
| `0.5.0` | Custom HTTP replay tool (§10.3 baseline + §10.4 benchmark) | — |
| `1.0.0` | §14 gap close + inner-loop README + acceptance pass | — |

## Spec References

- Full spec: [docs/spec.md](docs/spec.md)
- Acceptance criteria: [docs/spec.md §14](docs/spec.md#14-acceptance-criteria)
- Methodology: [docs/METHODOLOGY.md](docs/METHODOLOGY.md)
- Session log: [session-log.md](session-log.md)
- RPI artifacts: [.copilot-tracking/](.copilot-tracking/)
