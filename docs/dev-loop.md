# moviesx — Developer Inner Loop

This document covers everything from fresh clone to a live Grafana dashboard.
No step depends on knowledge outside this file.

---

## Prerequisites

| Tool | Minimum version | Install |
|---|---|---|
| Go | 1.23 | https://go.dev/dl/ |
| Docker | 24.x | https://docs.docker.com/get-docker/ |
| k3d | 5.x | `curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh \| bash` |
| kubectl | 1.28+ | Bundled with k3d; or https://kubernetes.io/docs/tasks/tools/ |
| kustomize | 5.x | Bundled with kubectl (`kubectl kustomize`); or https://kubectl.docs.kubernetes.io/installation/kustomize/ |
| make | Any | System default |
| Helm | 3.x | https://helm.sh/docs/intro/install/ (only needed for monitoring setup) |

---

## One-time cluster setup

### 1. Create the k3d cluster

```bash
k3d cluster create movies --port "8080:80@loadbalancer"
```

> **Note:** In this environment the load-balancer pod may fail to bind host port 8080. That is OK — the API is still reachable via `kubectl port-forward` or `docker exec`. See §"Accessing the API" below.

Verify:
```bash
docker exec k3d-movies-server-0 kubectl get nodes
```

### 2. Install kube-prometheus-stack (Prometheus + Grafana)

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install kube-prom prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  -f k8s/monitoring/kube-prom-values.yaml
```

Confirm pods are Running (may take ~2 min):
```bash
docker exec k3d-movies-server-0 kubectl get pods -n monitoring
```

### 3. Patch Prometheus to discover all ServiceMonitors

Without this patch, Prometheus will only pick up ServiceMonitors with a matching release label:
```bash
docker exec k3d-movies-server-0 kubectl patch prometheus \
  -n monitoring kube-prom-kube-prometheus-stack-prometheus \
  --type=merge \
  -p '{"spec":{"serviceMonitorSelector":{}}}'
```

---

## Inner loop (change → deploy → verify → observe)

### Step 1 — Make your change

Edit Go files. If you also changed the version, update `cmd/moviesx/main.go`:
```go
var version = "1.0.0"  // bump to your new version
```

### Step 2 — Regenerate Swagger docs (if annotations changed)

Only needed when you modify `// @...` Swagger annotations:
```bash
make swagger
```

### Step 3 — Run tests

```bash
make test
```

Coverage gate is 80%. Tests must be green before building.

### Step 4 — Build the image

```bash
VERSION=1.0.0
docker build -t moviesx:${VERSION} .
```

### Step 5 — Import into k3d

```bash
k3d image import moviesx:${VERSION} -c movies
```

### Step 6 — Update image tag in dev overlay

Edit `k8s/overlays/dev/kustomization.yaml` and set the `newTag`:
```yaml
images:
  - name: moviesx
    newTag: "1.0.0"
```

### Step 7 — Apply and rollout

```bash
kubectl kustomize k8s/overlays/dev/ | docker exec -i k3d-movies-server-0 kubectl apply -f -
docker exec k3d-movies-server-0 kubectl rollout restart deployment/moviesx
docker exec k3d-movies-server-0 kubectl rollout status deployment/moviesx
```

### Step 8 — Verify the deployment

```bash
POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx -o jsonpath='{.items[0].status.podIP}')

# Version matches what you built
docker exec k3d-movies-server-0 wget -qO- http://${POD_IP}:8080/version

# Health and readiness
docker exec k3d-movies-server-0 wget -qO- http://${POD_IP}:8080/healthz
docker exec k3d-movies-server-0 wget -qO- http://${POD_IP}:8080/readyz

# Swagger UI (JSON spec)
docker exec k3d-movies-server-0 wget -qO- http://${POD_IP}:8080/swagger/v1/swagger.json \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['info']['version'])"

# Metrics endpoint
docker exec k3d-movies-server-0 wget -qO- http://${POD_IP}:8080/metrics \
  | grep -E 'http_requests_total|http_request_duration_seconds_bucket|http_requests_in_flight' \
  | head -5
```

### Step 9 — Run validation tests (baseline replay suite)

Build the Linux replay binary and copy into the k3d container:
```bash
make replay-build

docker cp bin/replay-linux k3d-movies-server-0:/tmp/replay
docker cp scenarios/ k3d-movies-server-0:/tmp/scenarios

POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx -o jsonpath='{.items[0].status.podIP}')

docker exec k3d-movies-server-0 /tmp/replay \
  --base-url http://${POD_IP}:8080 \
  --scenarios /tmp/scenarios/baseline.yaml
```

All 54 scenarios must pass.

Alternatively, if the binary is macOS-native (run from host with port-forward active):
```bash
kubectl port-forward -n default deployment/moviesx 8080:8080 &
make e2e BASE_URL=http://localhost:8080
```

### Step 10 — Run dependency audit

```bash
make audit
```

Findings are shown as warnings. Exit 0 means the build is clean (or findings are non-critical Go stdlib vulnerabilities). Exit non-zero means an internal govulncheck error — investigate before shipping.

### Step 11 — Inspect Grafana dashboard

```bash
GRAF_IP=$(docker exec k3d-movies-server-0 kubectl get pod -n monitoring \
  -l app.kubernetes.io/name=grafana \
  -o jsonpath='{.items[0].status.podIP}')

# Confirm dashboard exists
docker exec k3d-movies-server-0 wget -qO- \
  "http://${GRAF_IP}:3000/api/dashboards/uid/moviesx-overview" \
  -U "" --header "Authorization: Basic $(echo -n 'admin:dev-password' | base64)" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print('dashboard:', d['dashboard']['title'])"
```

After running the baseline replay (step 9), open Grafana in a browser via port-forward:
```bash
kubectl port-forward -n monitoring svc/kube-prom-grafana 3000:80
# Browse to http://localhost:3000 — admin / dev-password
# Navigate to Dashboards → moviesx-overview
```

---

## Accessing the API without port-forward

If the k3d load-balancer is not binding host ports, use `docker exec` to reach pod IPs directly:
```bash
POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx \
  -o jsonpath='{.items[0].status.podIP}')
docker exec k3d-movies-server-0 wget -qO- http://${POD_IP}:8080/api/genres
```

---

## Benchmark section

### When to use the bench overlay

Use `k8s/overlays/bench/` instead of `k8s/overlays/dev/` when running the §10.4 benchmark. The dev overlay caps the container at 200m CPU, which causes CPU-throttle-induced p95 spikes at high RPS. The bench overlay inherits base limits (500m CPU / 512Mi memory), giving the server headroom for accurate latency measurements.

### Deploy the bench overlay

```bash
VERSION=1.0.0
k3d image import moviesx:${VERSION} -c movies
kubectl kustomize k8s/overlays/bench/ | docker exec -i k3d-movies-server-0 kubectl apply -f -
docker exec k3d-movies-server-0 kubectl rollout restart deployment/moviesx
docker exec k3d-movies-server-0 kubectl rollout status deployment/moviesx
```

### Run the benchmark

```bash
make replay-build
docker cp bin/replay-linux k3d-movies-server-0:/tmp/replay
docker cp scenarios/ k3d-movies-server-0:/tmp/scenarios

POD_IP=$(docker exec k3d-movies-server-0 kubectl get pod -l app=moviesx \
  -o jsonpath='{.items[0].status.podIP}')

docker exec k3d-movies-server-0 /tmp/replay \
  --base-url http://${POD_IP}:8080 \
  --scenarios /tmp/scenarios/benchmark.yaml \
  --benchmark --duration 30s --rps 500 --concurrency 50
```

Or via the Makefile (requires port-forward or host reachable pod IP):
```bash
make bench BASE_URL=http://${POD_IP}:8080
```

### Expected output

```
Benchmark results:
  Duration:     30s
  Requests:     15000
  Success rate: 100.00%
  p50:          ~2ms
  p95:          <20ms   (should not be dominated by CPU throttle)
  p99:          <50ms
```

p95 above ~100ms at 500 RPS is a sign of CPU throttling. Confirm with:
```bash
docker exec k3d-movies-server-0 kubectl top pod -l app=moviesx
```

---

## CLI flags reference

The binary supports these flags (all override env vars, which override built-in defaults):

| Flag | Env var | Default | Purpose |
|---|---|---|---|
| `--movies-port` | `MOVIES_PORT` | `8080` | HTTP listen port |
| `--movies-log-level` | `MOVIES_LOG_LEVEL` | `info` | Log level |
| `--movies-data-dir` | `MOVIES_DATA_DIR` | `/data` | Data file directory |
| `--version` / `-v` | — | — | Print semver and exit |
| `--help` / `-h` | — | — | Show flag list and exit |

---

## Acceptance criteria quick-check (§14)

| Criterion | Verification command |
|---|---|
| §14.1 dev-loop README | This file exists and is linked from `README.md` |
| §14.2 replay baseline | `docker exec k3d-movies-server-0 /tmp/replay --base-url ... --scenarios /tmp/scenarios/baseline.yaml` → 54/54 pass |
| §14.3 metrics | `wget ... /metrics \| grep http_requests_total` |
| §14.4 JSON logs | `kubectl logs ... \| head -1 \| python3 -c "import sys,json; json.load(sys.stdin)"` |
| §14.5 Grafana dashboard | `GET /api/dashboards/uid/moviesx-overview` returns HTTP 200; at least one panel shows non-zero rate |
| §14.6 security context | `kubectl exec ... -- id` returns uid=65532; `kubectl exec ... -- touch /tmp/x` fails (read-only FS) |
| §14.7 inner-loop end-to-end | Run steps 1–11 above; `/version` returns semver matching git tag |
