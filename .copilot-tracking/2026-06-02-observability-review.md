# Session 3 — Observability Review (tag 0.3.0)

Review date: 2026-06-03  
Reviewer: Claude Code  
Scope: Validate implementation against [2026-06-02-observability-plan.md](2026-06-02-observability-plan.md) and [2026-06-02-observability-changes.md](2026-06-02-observability-changes.md)

---

## Verdict: SHIP ✓

All seven checklist items passed. One documented plan deviation (ServiceMonitor label strategy) is correct and verified working. One cosmetic redundancy in Helm values (noted below, no action required before ship).

---

## Checklist Results

### 1. GET /metrics — valid Prometheus text exposition format

**PASS**

Live pod at `10.42.0.62:8080/metrics` returns well-formed `# HELP` / `# TYPE` / metric-value lines. Content-Type is `text/plain`. Test `TestMetrics_OK` asserts this in CI.

Sample (first five non-Go-runtime lines):
```
# HELP http_requests_in_flight Current number of HTTP requests being served.
# TYPE http_requests_in_flight gauge
http_requests_in_flight 0
# HELP http_request_duration_seconds HTTP request latency distributions.
# TYPE http_request_duration_seconds histogram
```

---

### 2. Request count counter and latency histogram — correct names and labels

**PASS**

All three custom metrics are present with the correct names, types, and label sets:

| Metric | Type | Labels | Sample line |
|---|---|---|---|
| `http_requests_total` | Counter | `code`, `method` | `http_requests_total{code="200",method="get"} 6508` |
| `http_request_duration_seconds` | Histogram | `handler`, `method`, `le` | `http_request_duration_seconds_bucket{handler="api_genres",method="get",le="0.001"} 2` |
| `http_requests_in_flight` | Gauge | (none) | `http_requests_in_flight 0` |

Histogram buckets (9 finite + `+Inf`): `.001 .005 .01 .025 .05 .1 .25 .5 1` — matches plan T2.

Handler label values observed: `api_genres`, `api_movies` (full set: `version`, `healthz`, `api_genres`, `api_movies`, `api_movies_id`, `api_actors`, `api_actors_id`).

`/metrics` endpoint is NOT wrapped in `instrumentedHandler` — confirmed in `server.go:29`.

---

### 3. Every HTTP request produces a valid JSON log line on stdout

**PASS**

Sample lines from live pod (`kubectl logs -l app=moviesx --tail=5`):
```json
{"time":"2026-06-03T22:01:59.485858049Z","level":"INFO","msg":"request","method":"GET","path":"/healthz","status":200,"duration_ms":0}
{"time":"2026-06-03T22:01:59.802091632Z","level":"INFO","msg":"request","method":"GET","path":"/metrics","status":200,"duration_ms":0}
{"time":"2026-06-03T22:02:07.426844011Z","level":"INFO","msg":"request","method":"GET","path":"/healthz","status":200,"duration_ms":0}
```

Fields present: `time`, `level`, `msg`, `method`, `path`, `status`, `duration_ms` — matches plan T5.

`loggingMiddleware` wraps the entire mux including `/metrics` and `/healthz` — intentional per changes doc.

Startup log (plan T4):
```json
{"time":"2026-06-03T17:16:13.063569-05:00","level":"INFO","msg":"startup","port":"18082","log_level":"info","data_dir":"/Users/mbr/moviesx/src/data"}
```

---

### 4. MOVIES_LOG_LEVEL env var filters log output correctly

**PASS**

Tested locally against `src/data/`:

| Setting | Expected | Observed |
|---|---|---|
| `MOVIES_LOG_LEVEL=info` | startup Info + request Info lines emitted | ✓ both present |
| `MOVIES_LOG_LEVEL=error` | startup Info suppressed, request Info suppressed | ✓ no output (empty file) |

`main.go:20–23` uses `slog.Level.UnmarshalText` with fallback to `LevelInfo` on parse error; `*slog.LevelVar` passed to `slog.HandlerOptions{Level: &programLevel}` — correct per plan T4.

---

### 5. Prometheus Operator installed, ServiceMonitor deployed and targeting movies-api

**PASS**

Cluster state verified:
```
prometheus-operator-59f856bf97-m7tfr   1/1   Running   default ns
```

Prometheus target query result:
```
moviesx   up   http://10.42.0.62:8080/metrics
```

**Plan deviation (documented, correct):** T10 specified adding `release: kube-prom` label to the ServiceMonitor. The implementation instead uses `serviceMonitorSelector: {}` on the Prometheus CR (JSON-patched, documented in `kube-prom-values.yaml` as `serviceMonitorSelectorNilUsesHelmValues: false`). This is explicitly documented in both the changes doc and CLAUDE.md as the chosen approach, and is verified working. Not a defect.

---

### 6. Grafana running, datasource auto-provisioned, dashboard loads with live data

**PASS**

Pod: `grafana-79fddf78df-t57ss  1/1 Running`

Datasource (via Grafana API):
```
Prometheus   prometheus   http://prometheus-operated:9090
```

Dashboard (via `GET /api/dashboards/uid/moviesx-overview`):
```
title:  moviesx Overview
uid:    moviesx-overview
panels: 2
```

Panel queries match plan T11:
- Panel 1 (Request Rate): `sum by(handler, code) (rate(http_requests_total[5m]))`
- Panel 2 (Latency p99): `histogram_quantile(0.99, sum by(handler, le) (rate(http_request_duration_seconds_bucket[5m])))`

**Note on provisioning approach:** The dev cluster's Grafana uses static ConfigMap volume mounts (patched from the existing `grafana-dashboard-movies` ConfigMap), not the sidecar label approach. The repo file `k8s/base/grafana-dashboard-configmap.yaml` is written for the sidecar approach and is correct for a fresh kube-prometheus-stack install. This discrepancy is explicitly documented in the changes doc (Key Decision #5) and CLAUDE.md. No action needed before ship; the correct behavior will apply on a fresh install.

---

### 7. Kustomize base/ + overlays/dev/ structure correct, kubectl apply -k brings up full stack

**PASS**

`kubectl kustomize k8s/overlays/dev/` renders 5 resources without error:

| Kind | Name | Namespace |
|---|---|---|
| ConfigMap | moviesx-overview-dashboard | monitoring |
| Secret | grafana-admin-secret | monitoring |
| Service | moviesx | default |
| Deployment | moviesx | default |
| ServiceMonitor | moviesx | default |

Deployment image tag: `moviesx:0.3.0` — correctly overridden by the dev overlay `images:` stanza.  
Resource patch applied: `cpu: 50m/200m`, `memory: 32Mi/64Mi`.

Tests pass: `make test` → 90.3% coverage (gate: 80%).

---

## Minor Finding (No-Ship Blocker)

**kube-prom-values.yaml: redundant sidecar option**

`k8s/monitoring/kube-prom-values.yaml` sets both `folderAnnotation: grafana_folder` (annotation-based folder assignment) and `provider.foldersFromFilesStructure: true` (file-path-based folder assignment). These are two separate mechanisms for determining Grafana folder names from ConfigMap keys vs. file paths. They can coexist without error (annotation takes precedence when set), but only `folderAnnotation` is relevant here. Cosmetic only — does not affect dashboard provisioning behavior in the dev cluster or a fresh install with the sidecar. Can be cleaned up in a future session.

---

## Items NOT Checked (Cluster Not Accessible or Out of Scope)

- Grafana panel "No data" state cannot be verified from CLI alone (requires browser); the Prometheus target is UP and scraping, so panels should populate within one scrape interval (15s). The dashboard configuration is confirmed correct.
- Alertmanager is disabled per values file (`alertmanager.enabled: false`) — expected, not a gap.
- `overlays/prod/` — not in scope for 0.3.0 (parking lot item).
