# Observability Research — Session 3 (tag 0.3.0)

Date: 2026-06-02  
Scope: Prometheus metrics + structured logging + Grafana + Kustomize for a Go 1.22 net/http API on k3d/k3s.

---

## Topic 1 — prometheus/client_golang HTTP Instrumentation

### Package and Version

- **Import path:** `github.com/prometheus/client_golang`
- **Stable version as of 2026-06:** `v1.23.2` (released 2025-09-05)
- **Status:** v1.x is the production track. A v2 branch exists as an experiment but is not recommended for production use.
- **go get:** `go get github.com/prometheus/client_golang@v1.23.2`
- **Sub-packages used:** `prometheus/promhttp` for HTTP middleware and the `/metrics` handler.

### Middleware Approach: InstrumentHandlerCounter + InstrumentHandlerDuration

Use the `promhttp.InstrumentHandler*` family rather than a manual `ResponseWriter` wrapper. These functions return `http.HandlerFunc` and are designed to be chained (nested) together. The standard chain for this project:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    inFlight = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "http_requests_in_flight",
        Help: "Current number of in-flight HTTP requests.",
    })

    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total HTTP requests by method, route, and status code.",
        },
        []string{"code", "method"},
    )

    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request latency by handler and method.",
            Buckets: prometheus.DefBuckets,
        },
        []string{"handler", "method"},
    )
)

func init() {
    prometheus.MustRegister(inFlight, requestsTotal, requestDuration)
}
```

The `code` and `method` labels are the **only labels** natively supported by `InstrumentHandlerCounter` and `InstrumentHandlerDuration` without `WithLabelFromCtx`. The functions inspect the wrapped `ResponseWriter` to capture status code and HTTP method automatically.

### Per-Route Labeling with MustCurryWith

To add a `handler` (route) dimension, curry the metric before wrapping each route. This is the canonical pattern from the official test suite:

```go
func instrumentedHandler(route string, h http.Handler) http.Handler {
    return promhttp.InstrumentHandlerInFlight(inFlight,
        promhttp.InstrumentHandlerDuration(
            requestDuration.MustCurryWith(prometheus.Labels{"handler": route}),
            promhttp.InstrumentHandlerCounter(requestsTotal, h),
        ),
    )
}

// In server.New():
mux.Handle("GET /api/movies",
    instrumentedHandler("api_movies", http.HandlerFunc(s.handleMovies)))
mux.Handle("GET /api/movies/{id}",
    instrumentedHandler("api_movies_id", http.HandlerFunc(s.handleMovieByID)))
// ... etc.
```

Key points:
- `MustCurryWith(prometheus.Labels{"handler": route})` pre-binds the `handler` label on the `HistogramVec` before passing it to `InstrumentHandlerDuration`. This is a zero-allocation compile-time binding.
- `InstrumentHandlerCounter` automatically fills `code` and `method` from the response.
- The chain order is: in-flight (outermost) → duration → counter → actual handler.

### Function Signatures

```go
func InstrumentHandlerCounter(counter *prometheus.CounterVec, next http.Handler, opts ...Option) http.HandlerFunc
func InstrumentHandlerDuration(obs prometheus.ObserverVec, next http.Handler, opts ...Option) http.HandlerFunc
func InstrumentHandlerInFlight(g prometheus.Gauge, next http.Handler) http.Handler
func InstrumentHandlerResponseSize(obs prometheus.ObserverVec, next http.Handler, opts ...Option) http.HandlerFunc
```

Available `Option` constructors:
- `WithExtraMethods(methods ...string) Option` — extend recognized HTTP methods
- `WithLabelFromCtx(name string, valueFn LabelValueFromCtx) Option` — dynamic label from request context
- `WithExemplarFromContext(fn func(ctx context.Context) prometheus.Labels) Option` — inject exemplars

### Exposing /metrics

```go
// In mux registration — do NOT instrument the /metrics endpoint itself
mux.Handle("GET /metrics", promhttp.Handler())
```

`promhttp.Handler()` uses `prometheus.DefaultGatherer` and `prometheus.DefaultRegisterer`. For a custom registry:

```go
reg := prometheus.NewRegistry()
// register metrics on reg...
mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
```

### Default Histogram Buckets

`prometheus.DefBuckets` = `{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}` (seconds).

For an in-memory API like this one, responses should be sub-millisecond. Consider tighter buckets:

```go
Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
```

### High-Cardinality Label Warning

**Never** use raw URL path (`r.URL.Path`) or query strings as a label — every unique movie ID, actor ID, or query parameter combination creates a new time series. The `MustCurryWith` approach above uses a small fixed set of route names (7 routes) — this is safe. The `WithLabelFromCtx` option includes an explicit documentation warning: _"Beware to not have too high cardinality on the values. You always should sanitize external inputs."_

### Recommendation

Use `MustCurryWith` with a fixed set of named routes. Register all metrics at package init. Chain `InstrumentHandlerInFlight` → `InstrumentHandlerDuration` (with curried `handler` label) → `InstrumentHandlerCounter` (fills `code` + `method` automatically). Add the `/metrics` endpoint via `promhttp.Handler()` without instrumentation of the endpoint itself to avoid noise.

---

## Topic 2 — Structured JSON Logging: log/slog vs zerolog vs zap

### log/slog (stdlib)

- **Import path:** `log/slog` (standard library, no external dependency)
- **Available since:** Go 1.21; available in this project's Go 1.22 toolchain
- **JSON handler construction:**

```go
import "log/slog"

// Dynamic level (settable at runtime)
programLevel := new(slog.LevelVar)  // defaults to Info

h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: programLevel,
})
logger := slog.New(h)
slog.SetDefault(logger)
```

- **Parse level from env var:**

```go
levelStr := os.Getenv("MOVIES_LOG_LEVEL")  // "debug", "info", "warn", "error"
if levelStr == "" {
    levelStr = "info"
}
var level slog.Level
if err := level.UnmarshalText([]byte(levelStr)); err != nil {
    level = slog.LevelInfo
}
programLevel.Set(level)
```

- **Accepted strings:** `"DEBUG"`, `"INFO"`, `"WARN"`, `"ERROR"` (case-insensitive via `UnmarshalText`). Also accepts `"WARN+1"` style offsets.
- **Logging API:**

```go
slog.Info("request handled", "method", "GET", "path", "/api/movies", "status", 200, "duration_ms", 5)
// or with a context-attached logger:
logger.InfoContext(ctx, "request handled", slog.String("route", "api_movies"))
```

- **Performance:** ~174 ns/op, 0 allocations in benchmarks. Fast enough for any realistic HTTP API load.
- **External dependencies:** None.

### zerolog

- **Import path:** `github.com/rs/zerolog`
- **Current version:** v1.35.1 (released 2026-04-20)
- **JSON construction:**

```go
import "github.com/rs/zerolog"
import "github.com/rs/zerolog/log"

logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
```

- **Level from env var:**

```go
level, err := zerolog.ParseLevel(os.Getenv("MOVIES_LOG_LEVEL"))
if err != nil {
    level = zerolog.InfoLevel
}
zerolog.SetGlobalLevel(level)
```

- **Accepted strings:** `"trace"`, `"debug"`, `"info"`, `"warn"`, `"error"`, `"fatal"`, `"panic"`, `"disabled"` (lowercase).
- **Logging API (fluent/chained):**

```go
logger.Info().Str("method", "GET").Str("path", "/api/movies").Int("status", 200).Msg("request handled")
```

- **Performance:** ~30 ns/op, 0 allocations — the fastest option.
- **External dependencies:** Minimal; zerolog itself has no heavy transitive deps. One external package.

### zap

- **Import path:** `go.uber.org/zap`
- **Current version:** v1.28.0
- **JSON construction (production preset):**

```go
import "go.uber.org/zap"

cfg := zap.NewProductionConfig()
cfg.OutputPaths = []string{"stdout"}
cfg.ErrorOutputPaths = []string{"stderr"}
logger, err := cfg.Build()
```

- **Level from env var:**

```go
levelStr := os.Getenv("MOVIES_LOG_LEVEL")
atomicLevel, err := zap.ParseAtomicLevel(levelStr)
if err != nil {
    atomicLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
}
cfg.Level = atomicLevel
```

- **Accepted strings (ParseAtomicLevel):** `"debug"`, `"info"`, `"warn"`, `"error"`, `"dpanic"`, `"panic"`, `"fatal"`.
- **Logging API:**

```go
// Typed (fastest)
logger.Info("request handled", zap.String("method", "GET"), zap.Int("status", 200))
// Sugared (printf-style, slightly slower)
logger.Sugar().Infow("request handled", "method", "GET", "status", 200)
```

- **Performance:** ~71 ns/op, 0 allocations. Fast, but zerolog is faster for pure throughput.
- **External dependencies:** `go.uber.org/zap` pulls in `go.uber.org/atomic`, `go.uber.org/multierr`, and `go.uber.org/zap/zapcore`. Small but non-zero dependency footprint.

### Performance Summary

| Library   | ns/op | Allocs/op | External deps |
|-----------|-------|-----------|---------------|
| zerolog   | ~30   | 0         | ~1            |
| zap       | ~71   | 0         | ~3            |
| slog      | ~174  | 0         | 0             |
| logrus    | ~2231 | 23        | several       |

### Recommendation

**Use `log/slog`** for this project. Rationale:
1. Zero external dependencies — consistent with a stdlib-first Go philosophy.
2. Already satisfies the performance envelope; this API will never log at a rate where 174 ns/op vs 30 ns/op matters.
3. Directly integrates with Go 1.22's `net/http` request context via `slog.InfoContext(r.Context(), ...)`.
4. `slog.LevelVar` allows runtime-dynamic level changes (useful for a future `/debug/loglevel` endpoint).
5. The `MOVIES_LOG_LEVEL` env var integration is straightforward with `level.UnmarshalText`.

If this project later needed sub-microsecond logging throughput (e.g. a high-frequency trading feed), zerolog would be the pick. For this use case, stdlib is the correct choice.

---

## Topic 3 — Prometheus Operator on k3s via kube-prometheus-stack

### What the Chart Installs

`prometheus-community/kube-prometheus-stack` (current chart version: **86.1.0**) installs:

| Component | Description |
|---|---|
| Prometheus Operator | Controller (Deployment) that manages Prometheus/Alertmanager CRs |
| Prometheus CRDs | `Prometheus`, `Alertmanager`, `ServiceMonitor`, `PodMonitor`, `PrometheusRule`, `ScrapeConfig` |
| Prometheus instance | StatefulSet, via `Prometheus` CR |
| Alertmanager | StatefulSet, via `Alertmanager` CR |
| Grafana | Deployment, includes sidecar for dashboard/datasource provisioning |
| prometheus-node-exporter | DaemonSet — node-level hardware/OS metrics |
| kube-state-metrics | Deployment — Kubernetes object metrics |
| Default dashboards/rules | Pre-built Grafana dashboards and PrometheusRules for cluster monitoring |

### Minimum Install for k3d

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

kubectl create namespace monitoring

helm install kube-prom prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  -f k8s/monitoring/kube-prom-values.yaml
```

### k3s/k3d-Specific Values to Override

k3s runs the control plane components (scheduler, controller-manager, etcd, kube-proxy) inside the k3s binary, not as separate pods. The default kube-prometheus-stack tries to scrape them as standard pods and fails. Disable the irrelevant monitors:

```yaml
# k8s/monitoring/kube-prom-values.yaml

# k3s embeds etcd differently — no separate etcd pod to scrape
kubeEtcd:
  enabled: false

# k3s runs controller-manager inside the k3s process
kubeControllerManager:
  enabled: false

# k3s runs scheduler inside the k3s process
kubeScheduler:
  enabled: false

# k3s uses Flannel/Traefik, not kube-proxy as a separate pod
kubeProxy:
  enabled: false

# Disable the default alert rules that depend on the above
defaultRules:
  rules:
    etcd: false
    kubeScheduler: false

# Keep kubelet monitoring (works fine on k3s)
kubelet:
  enabled: true

# For k3d development: reduce resource footprint
prometheus:
  prometheusSpec:
    retention: 1d
    resources:
      requests:
        memory: 256Mi
        cpu: 100m

alertmanager:
  enabled: true

grafana:
  enabled: true
  sidecar:
    dashboards:
      enabled: true
      searchNamespace: ALL
    datasources:
      enabled: true
      searchNamespace: ALL
```

For a minimal dev cluster (k3d), you can also set `alertmanager.enabled: false` to reduce footprint further.

### ServiceMonitor CRD

`ServiceMonitor` is the standard resource for scraping a `ClusterIP` service. `PodMonitor` is an alternative when there is no Service in front (e.g., scraping pods directly). For this project's `moviesx` Service, `ServiceMonitor` is correct.

**Minimum ServiceMonitor spec:**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: moviesx
  namespace: default          # same namespace as the Service
  labels:
    app: moviesx
    release: kube-prom        # must match Prometheus CR's serviceMonitorSelector (see below)
spec:
  selector:
    matchLabels:
      app: moviesx            # matches the moviesx Service's labels
  namespaceSelector:
    matchNames:
      - default
  endpoints:
    - port: http              # named port on the Service (add name: http to service.yaml)
      path: /metrics
      interval: 15s
      scrapeTimeout: 10s
```

The Service (`k8s/service.yaml`) needs a named port for the selector to work:

```yaml
ports:
  - name: http
    port: 80
    targetPort: 8080
```

### ServiceMonitor Discovery: serviceMonitorSelector and RBAC

By default, `kube-prometheus-stack` configures the `Prometheus` CR to only discover `ServiceMonitor` resources that carry a label matching the Helm release name. This is controlled by `prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues` (default: `true`).

**Option A — Add the release label to your ServiceMonitor** (recommended for a single-release setup):

```yaml
# In ServiceMonitor metadata.labels:
release: kube-prom    # must match the Helm release name
```

**Option B — Disable the restriction** (picks up all ServiceMonitors cluster-wide):

```yaml
# In kube-prom-values.yaml:
prometheus:
  prometheusSpec:
    serviceMonitorSelectorNilUsesHelmValues: false
    serviceMonitorSelector: {}
    serviceMonitorNamespaceSelector: {}
```

**Cross-namespace RBAC:** The chart creates a `ClusterRole` for the Prometheus ServiceAccount with `get/list/watch` on `services`, `endpoints`, `pods`, and the monitoring CRDs. This covers cross-namespace scraping by default. If you see "forbidden" errors in Prometheus logs, verify the `ClusterRoleBinding` for the `kube-prom-kube-prometheus-stack-prometheus` ServiceAccount exists.

### PodMonitor vs ServiceMonitor

Use `ServiceMonitor` — this project has a `ClusterIP` Service in front of the pod. `PodMonitor` is for scraping pod-level metrics when there is no corresponding Service, which is uncommon for well-structured deployments.

---

## Topic 4 — Grafana Dashboard Auto-Provisioning via Kubernetes ConfigMap

### How the Sidecar Works

`kube-prometheus-stack` deploys a `k8s-sidecar` container alongside Grafana. The sidecar watches the cluster for `ConfigMap` resources that carry a specific label and writes their contents to a directory that Grafana's provisioning system watches. This means new dashboards can be injected at runtime without restarting the Grafana pod.

### Required ConfigMap Label

The default label (as configured in the Helm chart) is:

```yaml
labels:
  grafana_dashboard: "1"
```

This label key/value is set in values.yaml at `grafana.sidecar.dashboards.label` / `grafana.sidecar.dashboards.labelValue`.

### ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: moviesx-dashboard
  namespace: monitoring         # same namespace as Grafana, OR use searchNamespace: ALL
  labels:
    grafana_dashboard: "1"
  annotations:
    grafana_folder: "moviesx"   # optional: places dashboard in a named folder
data:
  moviesx-overview.json: |      # key MUST end in .json
    {
      "title": "moviesx Overview",
      "uid": "moviesx-overview",
      "schemaVersion": 38,
      ...
    }
```

Key constraints:
- The `data` key must end in `.json`.
- The ConfigMap namespace matters: if `searchNamespace: ALL` is set in the Helm values, the sidecar scans all namespaces. Otherwise it only scans the Grafana pod's namespace.
- The sidecar picks up changes dynamically — no Grafana restart needed. Changes appear within ~30 seconds.

### Folder Annotation

To place dashboards into a Grafana folder, add the annotation:

```yaml
annotations:
  grafana_folder: "Application Monitoring"
```

This requires the Grafana sidecar to be configured with `folderAnnotation: grafana_folder` in the Helm values:

```yaml
grafana:
  sidecar:
    dashboards:
      folderAnnotation: grafana_folder
      provider:
        foldersFromFilesStructure: false
```

### Datasource Provisioning via ConfigMap

The chart auto-creates a datasource ConfigMap for Prometheus using the label `grafana_datasource: "1"`. For a custom additional datasource, create a ConfigMap with:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-extra-datasource
  namespace: monitoring
  labels:
    grafana_datasource: "1"   # note: different label from dashboard label
data:
  datasource.yaml: |
    apiVersion: 1
    datasources:
      - name: Prometheus
        type: prometheus
        url: http://kube-prom-kube-prometheus-stack-prometheus.monitoring.svc:9090
        access: proxy
        isDefault: true
        uid: prometheus
```

The sidecar drops this file into Grafana's `provisioning/datasources/` directory.

### values.yaml Sidecar Settings

```yaml
grafana:
  sidecar:
    dashboards:
      enabled: true
      label: grafana_dashboard      # label key to watch
      labelValue: "1"               # label value to match
      searchNamespace: ALL          # scan all namespaces
      folderAnnotation: grafana_folder
    datasources:
      enabled: true
      label: grafana_datasource
      labelValue: "1"
      searchNamespace: ALL
      defaultDatasourceEnabled: true
```

### Grafana 10+ Notes

Grafana 10+ uses `schemaVersion: 38` in dashboard JSON. The `uid` field is required for stable dashboard URLs. The provisioning file format is unchanged from Grafana 7+. The sidecar approach works identically on Grafana 10+; no breaking changes to the ConfigMap label mechanism.

### Recommendation

For this project, create a `k8s/monitoring/` directory and add:
- `grafana-dashboard-configmap.yaml` — the moviesx dashboard ConfigMap with `grafana_dashboard: "1"` label
- A minimal HTTP request rate panel (`http_requests_total`) and a latency quantile panel (`histogram_quantile(0.99, ...)` on `http_request_duration_seconds`)

Put the ConfigMap in the `monitoring` namespace, set `searchNamespace: ALL`, and use the `grafana_folder: moviesx` annotation.

---

## Topic 5 — Kustomize Base/Overlays for This Stack

### Standard Directory Layout

```
k8s/
├── base/
│   ├── kustomization.yaml
│   ├── deployment.yaml          # currently k8s/deployment.yaml
│   ├── service.yaml             # currently k8s/service.yaml
│   └── servicemonitor.yaml      # new: Prometheus scrape target
└── overlays/
    ├── dev/
    │   ├── kustomization.yaml
    │   └── patches/
    │       └── dev-patch.yaml   # replica count, resource limits, image tag
    └── prod/
        ├── kustomization.yaml
        └── patches/
            └── prod-patch.yaml
```

Move the existing `k8s/deployment.yaml` and `k8s/service.yaml` into `k8s/base/`. The Prometheus/Grafana ConfigMaps for dashboards can live in `k8s/monitoring/` as a separate layer (applied independently) or added as resources in `k8s/base/kustomization.yaml`.

### base/kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - deployment.yaml
  - service.yaml
  - servicemonitor.yaml
```

The base deployment uses a placeholder image tag (e.g., `moviesx:dev`) and `imagePullPolicy: Never` (already set — correct for k3d imported images).

### overlays/dev/kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

# Image tag override — matches the image name in deployment.yaml containers
images:
  - name: moviesx
    newTag: "0.3.0"

# Scale and resource override
replicas:
  - name: moviesx
    count: 1

patches:
  - path: patches/dev-patch.yaml
```

### overlays/dev/patches/dev-patch.yaml (strategic merge)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: moviesx
spec:
  template:
    spec:
      containers:
        - name: moviesx
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 200m
              memory: 64Mi
```

### overlays/prod/kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../base

images:
  - name: moviesx
    newName: ghcr.io/mbr/moviesx   # prod registry
    newTag: "0.3.0"

replicas:
  - name: moviesx
    count: 2

patches:
  - path: patches/prod-patch.yaml
```

### How images: Field Works

The `images:` field in kustomization.yaml scans all `containers[*].image` and `initContainers[*].image` fields in every resource in scope. It matches by the `name` field (which is compared against the full image reference before the `:` tag). If `moviesx:dev` is in the Deployment, `name: moviesx` matches it and `newTag: "0.3.0"` rewrites it to `moviesx:0.3.0`. The base YAML is **never modified**; the transformation is applied at render time.

Fields:
- `name` — image name to match (no tag, no digest)
- `newName` — replace the registry/name (optional; leave out to keep original name)
- `newTag` — replace the tag
- `digest` — use a specific SHA256 instead of a tag (immutable pinning for prod)

### Applying an Overlay

```bash
# Preview what will be applied
kubectl kustomize k8s/overlays/dev/

# Apply to cluster
kubectl apply -k k8s/overlays/dev/

# Equivalent with --kustomize flag
kubectl apply --kustomize k8s/overlays/dev/
```

### k3d and imagePullPolicy: Never

k3d imports images from the local Docker daemon into the cluster's internal registry via `k3d image import`. Once imported, the image is available inside the cluster's containerd. `imagePullPolicy: Never` in the Deployment prevents Kubernetes from trying to pull from an external registry — this is **already set correctly** in the current `k8s/deployment.yaml`.

When using Kustomize, keep `imagePullPolicy: Never` in the base `deployment.yaml`. For overlays that target a real registry (prod), patch it to `IfNotPresent` or remove it:

```yaml
# overlays/prod/patches/prod-patch.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: moviesx
spec:
  template:
    spec:
      containers:
        - name: moviesx
          imagePullPolicy: IfNotPresent
```

### Prometheus/Grafana ConfigMaps in the Kustomize Tree

Two reasonable approaches:

**Option A — Include in base (simpler):**  
Add `k8s/base/grafana-dashboard-configmap.yaml` and reference it in `base/kustomization.yaml`. Applied together with the app.

**Option B — Separate monitoring kustomization (cleaner separation):**  
`k8s/monitoring/kustomization.yaml` lists dashboard and datasource ConfigMaps. Apply with `kubectl apply -k k8s/monitoring/` independently of the app.

For 0.3.0, Option A is fine given the small scope. If monitoring configs diverge from the app deploy cycle in a later session, split them.

### Recommendation

1. Migrate `k8s/deployment.yaml` and `k8s/service.yaml` to `k8s/base/`.
2. Add `k8s/base/servicemonitor.yaml` with the `ServiceMonitor` for Prometheus scraping.
3. Create `k8s/overlays/dev/` with the `images:` tag override and a dev resource patch.
4. Keep `imagePullPolicy: Never` in the base; do not patch it in the dev overlay (k3d needs it).
5. Keep the Grafana dashboard ConfigMap in `k8s/base/` for now; move to `k8s/monitoring/` if it grows.

---

## Summary and Implementation Checklist for Session 3 (0.3.0)

| Area | Action | Package/Tool |
|---|---|---|
| Metrics | Add `prometheus/client_golang@v1.23.2` to go.mod | `promhttp.InstrumentHandler*` + `MustCurryWith` |
| Logging | Replace `fmt.Println` log calls with `log/slog` | stdlib, no new dep |
| Config | Parse `MOVIES_LOG_LEVEL` via `slog.Level.UnmarshalText` | stdlib |
| /metrics | Register `promhttp.Handler()` on the mux | `promhttp` |
| Metrics: /metrics route | Do not instrument the `/metrics` route itself | — |
| K8s: Prometheus | Install kube-prometheus-stack with k3s values | Helm, namespace: `monitoring` |
| K8s: ServiceMonitor | Add `k8s/base/servicemonitor.yaml` with `release: kube-prom` label | `monitoring.coreos.com/v1` |
| K8s: Grafana dashboard | Add ConfigMap with `grafana_dashboard: "1"` label + dashboard JSON | namespace: `monitoring` |
| K8s: Kustomize | Migrate to `k8s/base/` + `k8s/overlays/dev/` | `kubectl apply -k` |
| Image tag | Use `images: [{name: moviesx, newTag: 0.3.0}]` in dev overlay | kustomization.yaml |
