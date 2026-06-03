# Session 3 Task Plan — Observability (tag 0.3.0)

Date: 2026-06-02  
Scope: Prometheus metrics · structured JSON logging · Prometheus Operator + Grafana on k3d · Kustomize base/overlays restructure  
Research source: [2026-06-02-observability-research.md](2026-06-02-observability-research.md)

---

## Phase 1 — Go: Prometheus Metrics

### T1 — Add prometheus/client_golang dependency

- [x] **Files:** `go.mod`, `go.sum`
- **Action:** `go get github.com/prometheus/client_golang@v1.23.2`
- **Exit criteria:** `go.mod` lists `github.com/prometheus/client_golang v1.23.2`; `go build ./...` succeeds with no errors.

---

### T2 — Define metric vars in a new metrics file

- [x] **Files:** `internal/server/metrics.go` (new)
- **Action:** Declare package-level vars in the `server` package:
  - `inFlight` — `prometheus.NewGauge` with name `http_requests_in_flight`
  - `requestsTotal` — `prometheus.NewCounterVec` with name `http_requests_total`, labels `["code", "method"]`
  - `requestDuration` — `prometheus.NewHistogramVec` with name `http_request_duration_seconds`, label `["handler", "method"]`, buckets `[]float64{.001, .005, .01, .025, .05, .1, .25, .5, 1}` (tighter than DefBuckets — responses are sub-ms in memory)
  - `init()` calling `prometheus.MustRegister(inFlight, requestsTotal, requestDuration)`
  - `instrumentedHandler(route string, h http.Handler) http.Handler` — wraps with `InstrumentHandlerInFlight` → `InstrumentHandlerDuration` (with `requestDuration.MustCurryWith`) → `InstrumentHandlerCounter`
- **Exit criteria:** `go build ./internal/server/...` succeeds; `go vet ./...` clean.

---

### T3 — Wire metrics middleware and /metrics endpoint

- [x] **Files:** `internal/server/server.go`
- **Action:**
  - Wrap all seven API routes with `instrumentedHandler("route_name", ...)` using these route name constants: `version`, `healthz`, `api_genres`, `api_movies`, `api_movies_id`, `api_actors`, `api_actors_id`
  - Register `/metrics` via `mux.Handle("GET /metrics", promhttp.Handler())` — do **not** wrap in `instrumentedHandler`
  - Add `"github.com/prometheus/client_golang/prometheus/promhttp"` import
- **Exit criteria:** `curl localhost:8080/metrics` returns `http_requests_total` and `http_request_duration_seconds` families in text/plain Prometheus format after at least one request to any API route.

---

## Phase 2 — Go: Structured JSON Logging

### T4 — Wire slog JSON handler with level from config

- [x] **Files:** `cmd/moviesx/main.go`
- **Action:**
  - After `cfg := config.Load()`, parse `cfg.LogLevel` via `slog.Level.UnmarshalText` (fall back to `slog.LevelInfo` on parse error)
  - Construct `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel})` using a `*slog.LevelVar`
  - Call `slog.SetDefault(slog.New(handler))`
  - Emit a startup log line after handler init: `slog.Info("startup", "port", cfg.Port, "log_level", cfg.LogLevel, "data_dir", cfg.DataDir)`
  - Update `version` string to `"0.3.0"`
- **Exit criteria:** `MOVIES_LOG_LEVEL=debug go run ./cmd/moviesx` outputs newline-delimited JSON objects to stdout with a `"level"` field; `MOVIES_LOG_LEVEL=error` suppresses the startup Info line.

---

### T5 — Add per-request logging middleware

- [x] **Files:** `internal/server/server.go` (or new `internal/server/logging.go`)
- **Action:**
  - Add a `loggingMiddleware(next http.Handler) http.Handler` that wraps the entire mux; it captures method, path, response status code (via a minimal `responseWriter` recorder), and wall-clock duration, then calls `slog.Info("request", "method", ..., "path", ..., "status", ..., "duration_ms", ...)`.
  - Wrap the returned mux in `loggingMiddleware` inside `server.New()` — return `loggingMiddleware(mux)` so the signature stays `*http.ServeMux` → `http.Handler`.
  - Adjust `cmd/moviesx/main.go` to accept `http.Handler` from `server.New()` (or keep mux if it already satisfies the interface — `*http.ServeMux` implements `http.Handler`).
- **Exit criteria:** Each HTTP request produces exactly one JSON log line on stdout with fields `method`, `path`, `status`, `duration_ms`; `/metrics` requests also logged.

---

## Phase 3 — Go: Tests

### T6 — Update integration tests to cover /metrics and log output

- [x] **Files:** `internal/server/integration_test.go`, `internal/server/server_test.go`
- **Action:**
  - Add a test that GETs `/metrics` and asserts HTTP 200 and `Content-Type: text/plain`.
  - Add a test that makes one request to `/api/genres`, then GETs `/metrics` and asserts the response body contains `http_requests_total{` (basic smoke-check that counters are registered and incrementing).
  - Verify no existing tests are broken by the `loggingMiddleware` wrapper (the server now returns `http.Handler`, so adjust `httptest.NewServer` calls if needed).
- **Exit criteria:** `make test` passes with coverage ≥ 80%.

---

## Phase 4 — Kustomize: Restructure k8s/

### T7 — Move flat manifests into k8s/base/

- [x] **Files:** `k8s/base/deployment.yaml` (moved from `k8s/deployment.yaml`), `k8s/base/service.yaml` (moved from `k8s/service.yaml`); delete originals at `k8s/deployment.yaml` and `k8s/service.yaml`
- **Action:** `mkdir -p k8s/base && git mv k8s/deployment.yaml k8s/base/ && git mv k8s/service.yaml k8s/base/`
- **Exit criteria:** `k8s/deployment.yaml` and `k8s/service.yaml` no longer exist at root of `k8s/`; files present at `k8s/base/`.

---

### T8 — Add named port to Service

- [x] **Files:** `k8s/base/service.yaml`
- **Action:** Add `name: http` to the port entry so the `ServiceMonitor` can reference it by name:
  ```yaml
  ports:
    - name: http
      port: 80
      targetPort: 8080
  ```
- **Exit criteria:** `kubectl kustomize k8s/base/` (after T9) renders the Service with a named port `http`.

---

### T9 — Create k8s/base/kustomization.yaml

- [x] **Files:** `k8s/base/kustomization.yaml` (new)
- **Action:**
  ```yaml
  apiVersion: kustomize.config.k8s.io/v1beta1
  kind: Kustomization
  resources:
    - deployment.yaml
    - service.yaml
    - servicemonitor.yaml
    - grafana-dashboard-configmap.yaml
  ```
- **Exit criteria:** `kubectl kustomize k8s/base/` renders four resources without error.

---

### T10 — Create ServiceMonitor

- [x] **Files:** `k8s/base/servicemonitor.yaml` (new)
- **Action:** Minimal `ServiceMonitor` targeting the `moviesx` Service on the named port `http`, path `/metrics`, interval `15s`; metadata labels include `app: moviesx` and `release: kube-prom` (the Helm release name, required for ServiceMonitor discovery).
- **Exit criteria:** `kubectl kustomize k8s/base/` includes a `ServiceMonitor` resource with `apiVersion: monitoring.coreos.com/v1`.

---

### T11 — Create Grafana dashboard ConfigMap

- [x] **Files:** `k8s/base/grafana-dashboard-configmap.yaml` (new)
- **Action:** ConfigMap in namespace `monitoring`, label `grafana_dashboard: "1"`, annotation `grafana_folder: moviesx`. Data key `moviesx-overview.json` with a minimal dashboard JSON (`schemaVersion: 38`, `uid: moviesx-overview`) containing two panels:
  - **Request rate** — `rate(http_requests_total[5m])` summed by `handler` and `code`
  - **Latency p99** — `histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))` by `handler`
- **Exit criteria:** File is valid JSON under the `data` key (validate with `jq` or `kubectl apply --dry-run=client`); ConfigMap applies without error to a cluster with the CRDs present.

---

### T12 — Create dev overlay

- [x] **Files:** `k8s/overlays/dev/kustomization.yaml` (new), `k8s/overlays/dev/patches/dev-patch.yaml` (new), `k8s/overlays/dev/grafana-admin-secret.yaml` (new)
- **Action:**
  - `kustomization.yaml`: resources point to `../../base`; `images: [{name: moviesx, newTag: "0.3.0"}]`; include `grafana-admin-secret.yaml` as a resource; reference `patches/dev-patch.yaml`
  - `patches/dev-patch.yaml` (strategic merge): sets resource requests/limits on the `moviesx` container (`cpu: 50m/200m`, `memory: 32Mi/64Mi`)
  - `grafana-admin-secret.yaml`: a `Secret` in namespace `monitoring` named `grafana-admin-secret` with key `admin-password` — value `dev-password` (plaintext is fine for a local k3d cluster; not for prod)
- **Exit criteria:** `kubectl kustomize k8s/overlays/dev/` renders the Deployment with image tag `moviesx:0.3.0` and the resource patch applied; Secret resource is present in the output.

---

## Phase 5 — Monitoring Stack: Helm Install

### T13 — Create kube-prometheus-stack Helm values file

- [x] **Files:** `k8s/monitoring/kube-prom-values.yaml` (new)
- **Action:** Helm values file with k3s-specific overrides (disable `kubeEtcd`, `kubeControllerManager`, `kubeScheduler`, `kubeProxy`; disable corresponding defaultRules); Prometheus retention 1d, resources `256Mi/100m`; Grafana enabled with sidecar config (`searchNamespace: ALL`, `folderAnnotation: grafana_folder`); `grafana.admin.existingSecret: grafana-admin-secret` + `grafana.admin.userKey: admin-user` / `grafana.admin.passwordKey: admin-password` referencing the Secret from T12; `alertmanager.enabled: false` to reduce dev footprint; `serviceMonitorSelectorNilUsesHelmValues: false` + `serviceMonitorSelector: {}` so the Operator picks up all ServiceMonitors without requiring the release label (alternative to putting `release: kube-prom` on every ServiceMonitor — pick one approach and document it).
- **Exit criteria:** File exists and `helm template kube-prom prometheus-community/kube-prometheus-stack -f k8s/monitoring/kube-prom-values.yaml --namespace monitoring` renders without error.

---

### T14 — Install kube-prometheus-stack on k3d

- [x] **Files:** no code changes; cluster state
- **Action:**
  ```bash
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo update
  kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f k8s/base/grafana-admin-secret.yaml   # or via kustomize
  helm install kube-prom prometheus-community/kube-prometheus-stack \
    --namespace monitoring \
    -f k8s/monitoring/kube-prom-values.yaml
  ```
- **Exit criteria:** All pods in `monitoring` namespace reach `Running`/`Completed` within ~3 minutes (`kubectl get pods -n monitoring`); `prometheus-operator` Deployment is `1/1 Ready`.

---

## Phase 6 — Build, Deploy, Verify

### T15 — Build and tag image 0.3.0

- [x] **Files:** no code changes; Docker build
- **Action:**
  ```bash
  make test                          # must pass
  docker build -t moviesx:0.3.0 .
  k3d image import moviesx:0.3.0 -c movies
  ```
- **Exit criteria:** `make test` exits 0 with coverage ≥ 80%; `docker images moviesx` shows `0.3.0` tag; `k3d image import` succeeds.

---

### T16 — Apply dev overlay to cluster

- [x] **Files:** no code changes; cluster state
- **Action:**
  ```bash
  kubectl apply -k k8s/overlays/dev/
  kubectl rollout status deployment/moviesx
  kubectl port-forward svc/moviesx 8080:80
  ```
- **Exit criteria:** `curl localhost:8080/version` returns `0.3.0`; `curl localhost:8080/metrics` returns Prometheus text with `http_requests_total` and `http_request_duration_seconds`; startup log line is a JSON object on stdout.

---

### T17 — Verify Prometheus is scraping /metrics

- [x] **Files:** no code changes; cluster state (verified: moviesx target UP, rate(http_requests_total[1m]) non-empty)
- **Action:**
  - Port-forward Prometheus UI: `kubectl port-forward -n monitoring svc/kube-prom-kube-prometheus-stack-prometheus 9090:9090`
  - Open `localhost:9090/targets` — confirm `moviesx` target appears and is `UP`
  - Run `rate(http_requests_total[1m])` in the query UI — confirm metric is present
- **Exit criteria:** `moviesx` target listed as `UP` in Prometheus Targets UI; `http_requests_total` query returns a non-empty result set after hitting the API a few times.

---

### T18 — Verify Grafana dashboard renders

- [x] **Files:** no code changes; cluster state (verified via Grafana API: title=moviesx Overview, 2 panels, Prometheus datasource present)
- **Action:**
  - Port-forward Grafana: `kubectl port-forward -n monitoring svc/kube-prom-grafana 3001:80` (use 3001 to avoid conflict with any existing port 3000 process)
  - Login at `localhost:3001` with `admin` / `dev-password`
  - Navigate to `moviesx` folder — confirm `moviesx Overview` dashboard exists
  - Both panels (request rate + p99 latency) render without "No data" errors after sending a few test requests
- **Exit criteria:** Both panels display data; no "datasource not found" errors.

---

## Phase 7 — Wrap-up

### T19 — Update CLAUDE.md and tag 0.3.0

- [x] **Files:** `CLAUDE.md`, `cmd/moviesx/main.go` (version already updated in T4), `session-log.md`
- **Action:**
  - Update `CLAUDE.md` session map to mark `0.3.0` done; update Coverage line; add new deploy inner-loop commands for Kustomize; note Helm release name `kube-prom` in monitoring namespace
  - Write session retro in `session-log.md`
  - `git tag 0.3.0`
- **Exit criteria:** `git tag` shows `0.3.0`; `CLAUDE.md` Last updated reflects session 3.

---

## Parking Lot (deferred from Session 3 scope)

| Item | Reason deferred |
|---|---|
| `actorId` + `rating` filter on `GET /api/movies` | Carried from 0.2.0; not in 0.3.0 scope |
| `/readyz` (deep readiness — checks store is loaded) | Spec §10 item; scheduled for 0.4.0 |
| OpenAPI/Swagger doc generation | Scheduled for 0.4.0 |
| NetworkPolicy + securityContext hardening | Scheduled for 0.4.0 |
| `overlays/prod/` complete wiring (ghcr.io image name, `imagePullPolicy: IfNotPresent`, replica 2) | Stub created in T12 notes but prod k8s target does not exist yet |
| Grafana admin Secret rotation / external secrets operator | Not needed for local k3d dev |
| Alertmanager rules for SLO breach | Alertmanager disabled in dev overlay (footprint); revisit at 0.4.0 |
| Move Grafana dashboard ConfigMap to `k8s/monitoring/` kustomization | Option A (in base) is fine for 0.3.0; split if deploy cycles diverge |
| `/debug/loglevel` runtime level change endpoint | `slog.LevelVar` is in place to support this; endpoint deferred |
| Custom HTTP replay / benchmark tool (§10.4) | Scheduled for 0.5.0 |
