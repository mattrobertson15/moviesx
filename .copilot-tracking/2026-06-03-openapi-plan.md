# Session 4 Task Plan — 0.4.0
*Date: 2026-06-03 | Based on: 2026-06-03-openapi-research.md*

---

## Execution Order

Tasks are ordered so each builds on the previous. Complete them top-to-bottom.
Group 1 (readyz) is independent of Group 2–3 (Swagger); both are independent of Groups 4–5 (k8s).
Run Groups 1–3 first since they produce the routes that Group 6 verifies.

---

## Group 1 — GET /readyz

### T1 — Add readiness flag to server struct

- [ ] **Files:** `internal/server/server.go`
- Add `ready atomic.Bool` field to the `server` struct (import `sync/atomic`).
- Add `func (s *server) SetReady()` method: calls `s.ready.Store(true)`.
- Change `New()` return signature from `http.Handler` to `(*server, http.Handler)` so `main.go` can call `SetReady()` — OR add a returned `func()` closure (`readyFn`). Either form is acceptable; pick the one with fewer cascading changes. The handler returned must still be the `loggingMiddleware`-wrapped mux.
- **Exit:** `internal/server/server.go` compiles; existing tests still pass (`go test ./internal/server/...`).

---

### T2 — Implement handleReadyz + register route

- [ ] **Files:** `internal/server/server.go` (handler + registration)
- Add `func (s *server) handleReadyz(w http.ResponseWriter, r *http.Request)`:
  - If `!s.ready.Load()` → `writeJSON(w, 503, map[string]string{"status":"not ready","reason":"store loading"})`.
  - Else → `writeJSON(w, 200, map[string]string{"status":"ok"})`.
- Register in `New()`: `mux.Handle("GET /readyz", instrumentedHandler("readyz", http.HandlerFunc(s.handleReadyz)))`.
- **Exit:** `curl http://localhost:8080/readyz` returns one of the two expected JSON bodies depending on readiness state.

---

### T3 — Wire SetReady in main.go

- [ ] **Files:** `cmd/moviesx/main.go`
- Update the call to `server.New()` to capture the server pointer (or readyFn).
- Call `s.SetReady()` (or `readyFn()`) immediately after `store.Load()` succeeds and before `http.ListenAndServe(...)`. Because load is synchronous today, the pod is ready from the first request.
- **Exit:** Starting the binary locally and hitting `/readyz` returns `{"status":"ok"}` immediately.

---

### T4 — Update Deployment readinessProbe to /readyz

- [ ] **Files:** `k8s/base/deployment.yaml`
- Change `readinessProbe.httpGet.path` from `/healthz` to `/readyz`.
- Add `periodSeconds: 5` and `failureThreshold: 3` to the readinessProbe block (currently missing; align with research recommendation).
- Add the same `periodSeconds: 10` / `failureThreshold: 3` to `livenessProbe` for completeness.
- **Exit:** Kustomize renders the patched deployment with correct probe paths — verify with `kubectl kustomize k8s/overlays/dev/ | grep -A5 readinessProbe`.

---

## Group 2 — OpenAPI Spec (swag)

### T5 — Add swag dependencies + Makefile target

- [ ] **Files:** `go.mod`, `go.sum`, `tools.go` (new file at module root), `Makefile`
- Create `tools.go` with build tag `//go:build tools` and blank imports for the swag CLI and http-swagger runtime:
  ```go
  //go:build tools
  package tools
  import (
      _ "github.com/swaggo/swag/cmd/swag"
      _ "github.com/swaggo/http-swagger/v2"
  )
  ```
- Run `go get github.com/swaggo/swag/cmd/swag@latest github.com/swaggo/http-swagger/v2@latest` to add to `go.mod`/`go.sum`.
- Add `make swagger` target to `Makefile`:
  ```makefile
  swagger:
      go run github.com/swaggo/swag/cmd/swag init \
          -g cmd/moviesx/main.go \
          --parseDependency --parseInternal \
          --v3.1 \
          -o docs
  ```
- **Exit:** `make swagger` runs without error and creates/updates `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`.

---

### T6 — Global swag annotations in main.go

- [ ] **Files:** `cmd/moviesx/main.go`
- Add the global swag comment block immediately before the `main()` function:
  ```go
  // @title          moviesx API
  // @version        0.4.0
  // @description    Read-only movie catalog API
  // @host           localhost:8080
  // @BasePath       /
  // @schemes        http
  ```
- Add blank import `_ "github.com/mbr/moviesx/docs"` (import side-effect registers spec with swag runtime).
- **Exit:** `go build ./cmd/moviesx` compiles with the new import; `make swagger` regenerates `docs/` cleanly.

---

### T7 — Per-handler swag annotations on all §6 endpoints

- [ ] **Files:** `internal/server/server.go` (version, healthz, readyz), `internal/server/genres.go`, `internal/server/movies.go`, `internal/server/actors.go`
- Add godoc annotation blocks above each of the following handlers. Every block must include at minimum `@Summary`, `@Tags`, `@Produce`, `@Success`, and `@Router`. Add `@Param` lines for each query parameter. Add `@Failure 400` for endpoints with validation; `@Failure 404` for by-ID endpoints.

  | Handler | @Tags | @Router |
  |---|---|---|
  | handleVersion | system | `/version [get]` |
  | handleHealthz | system | `/healthz [get]` |
  | handleReadyz | system | `/readyz [get]` |
  | handleGenres | genres | `/api/genres [get]` |
  | handleMovies | movies | `/api/movies [get]` |
  | handleMovieByID | movies | `/api/movies/{id} [get]` |
  | handleActors | actors | `/api/actors [get]` |
  | handleActorByID | actors | `/api/actors/{id} [get]` |

  For `handleMovies`, include `@Param` entries for: `q` (query, string, false), `genre` (query, string, false), `year` (query, int, false), `rating` (query, number, false), `actorId` (query, string, false), `pageNumber` (query, int, false), `pageSize` (query, int, false).

  For `handleActors`: `q` (query, string, false), `pageNumber` (query, int, false), `pageSize` (query, int, false).

  For by-ID handlers: `id` as a `@Param` of type `path`.

- After annotating, run `make swagger` and confirm the generated `docs/swagger.json` contains all 8 routes without parse errors.
- **Exit:** `make swagger` succeeds; `jq '.paths | keys' docs/swagger.json` lists all 8 paths; no `swag` warnings about unresolved types.

---

### T8 — Commit generated docs/

- [ ] **Files:** `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`
- Stage and commit the generated `docs/` directory (these are committed artifacts for this project; no CI regeneration step is in scope for 0.4.0).
- **Exit:** `git status` is clean after staging; `go build ./...` succeeds.

---

## Group 3 — Swagger UI + JSON endpoint + Redirects

### T9 — Register GET /swagger/v1/swagger.json

- [ ] **Files:** `internal/server/server.go`
- Add an import for the generated docs package: `"github.com/mbr/moviesx/docs"` (already added in T6 as a blank import; change to named if needed, or use `swaggerFiles "github.com/swaggo/files/v2"`).
- Register a handler that returns the raw spec JSON:
  ```go
  mux.HandleFunc("GET /swagger/v1/swagger.json", func(w http.ResponseWriter, r *http.Request) {
      w.Header().Set("Content-Type", "application/json; charset=utf-8")
      w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
  })
  ```
- Do **not** wrap in `instrumentedHandler` (avoid metric self-counting on doc fetches — or wrap it, just be consistent with how `/metrics` is handled).
- **Exit:** `curl http://localhost:8080/swagger/v1/swagger.json | jq '.info.title'` returns `"moviesx API"`.

---

### T10 — Mount swaggo/http-swagger UI handler + redirects

- [ ] **Files:** `internal/server/server.go`
- Import `httpSwagger "github.com/swaggo/http-swagger/v2"`.
- Register the Swagger UI handler at the `/swagger/` prefix route:
  ```go
  mux.Handle("GET /swagger/", httpSwagger.Handler(
      httpSwagger.URL("/swagger/v1/swagger.json"),
  ))
  ```
- Register `GET /swagger` → redirect to `/swagger/`:
  ```go
  mux.HandleFunc("GET /swagger", func(w http.ResponseWriter, r *http.Request) {
      http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
  })
  ```
- Register `GET /` → redirect to `/swagger`:
  ```go
  mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
      http.Redirect(w, r, "/swagger", http.StatusMovedPermanently)
  })
  ```
  Note: in Go 1.22+ mux, `GET /` only matches the exact root (not all unmatched paths) when other patterns are registered — confirm this behaves as expected; use `"/{path...}"` pattern if needed to catch root-only traffic.
- **Exit:** `curl -v http://localhost:8080/` returns 301 → `/swagger`; `curl -v http://localhost:8080/swagger` returns 301 → `/swagger/`; `curl -v http://localhost:8080/swagger/` returns 200 with Swagger UI HTML; `curl http://localhost:8080/swagger/v1/swagger.json` returns valid JSON.

---

## Group 4 — Kubernetes securityContext Hardening

### T11 — Add container-level securityContext to base Deployment

- [ ] **Files:** `k8s/base/deployment.yaml`
- Under `spec.template.spec.containers[0]`, add:
  ```yaml
  securityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop: [ALL]
  ```
- Under `spec.template.spec.securityContext` (existing pod-level block), add:
  ```yaml
  seccompProfile:
    type: RuntimeDefault
  ```
  Result: pod-level has `runAsNonRoot: true`, `runAsUser: 65532`, `seccompProfile: RuntimeDefault`.
- **No volume mounts needed.** The Go HTTP server makes no writes to the filesystem at runtime (confirmed in research §4). If this assumption ever breaks, add an `emptyDir` for `/tmp`.
- **Exit:** `kubectl kustomize k8s/overlays/dev/` renders without error; the rendered Deployment YAML contains both pod-level and container-level securityContext blocks. Validate fields are correct by inspection.

---

## Group 5 — NetworkPolicy

### T12 — Create k8s/base/networkpolicy.yaml

- [ ] **Files:** `k8s/base/networkpolicy.yaml` (new file)
- Create a single `NetworkPolicy` named `moviesx-ingress-egress` in the `default` namespace targeting `app: moviesx` pods, with `policyTypes: [Ingress, Egress]`.
- **Ingress rules:**
  1. Allow from Traefik (ingress controller in `kube-system`): use `namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}` + `podSelector: {matchLabels: {app.kubernetes.io/name: traefik}}` as a **single** `from` list entry (AND logic), port 8080/TCP.
  2. Allow from Prometheus (`monitoring` namespace): use `namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: monitoring}}` + `podSelector: {matchLabels: {app: prometheus}}` as a **single** `from` list entry (AND logic), port 8080/TCP.
- **Egress rules:**
  1. Allow DNS to CoreDNS: `namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}` + `podSelector: {matchLabels: {k8s-app: kube-dns}}` as a **single** entry, ports 53/UDP and 53/TCP.
- All other ingress and egress: denied implicitly by the policy's presence.
- **Exit:** `kubectl apply -f k8s/base/networkpolicy.yaml --dry-run=server` succeeds. After live apply: `curl` to moviesx pod from a test pod outside the allow list returns a connection timeout/refused; `wget` from Prometheus pod to `:8080/metrics` succeeds.

---

### T13 — Add networkpolicy.yaml to kustomization

- [ ] **Files:** `k8s/base/kustomization.yaml`
- Add `- networkpolicy.yaml` to the `resources:` list.
- **Exit:** `kubectl kustomize k8s/base/` renders all 5 resources without error.

---

## Group 6 — Tests + Verification

### T14 — Integration tests for /readyz

- [ ] **Files:** `internal/server/integration_test.go`
- Add test `TestReadyz`:
  - Create server with `server.New()` but do **not** call `SetReady()` → `GET /readyz` must return 503 with body containing `"not ready"`.
  - Call `SetReady()` → `GET /readyz` must return 200 with body containing `"ok"`.
- **Exit:** `go test ./internal/server/... -run TestReadyz` passes.

---

### T15 — Integration tests for new routes (GET /, GET /swagger, /swagger/v1/swagger.json)

- [ ] **Files:** `internal/server/integration_test.go`
- Add test `TestSwaggerRoutes`:
  - `GET /` returns 301 with `Location: /swagger`.
  - `GET /swagger` returns 301 with `Location: /swagger/`.
  - `GET /swagger/` returns 200 with `Content-Type` containing `text/html`.
  - `GET /swagger/v1/swagger.json` returns 200 with `Content-Type: application/json` and a body that unmarshals as valid JSON containing key `"info"`.
- **Exit:** `go test ./internal/server/... -run TestSwaggerRoutes` passes.

---

### T16 — Full test suite + coverage gate

- [ ] **Files:** none (gate check only)
- Run `make test` — must pass with ≥80% coverage (current baseline: 90.3%; new routes should not regress this).
- Fix any coverage gaps if the gate drops below 80%.
- **Exit:** `make test` exits 0 with coverage ≥80%.

---

### T17 — Build + deploy smoke test

- [ ] **Files:** none (ops verification)
- Bump version string in `cmd/moviesx/main.go` to `"0.4.0"`.
- Run full deploy inner loop from CLAUDE.md §Build & Deploy Inner Loop (replace tag `0.3.0` with `0.4.0`).
- Smoke test from k3d exec:
  ```bash
  # All new endpoints
  wget -qO- http://$POD_IP:8080/readyz                    # {"status":"ok"}
  wget -qO- http://$POD_IP:8080/swagger/v1/swagger.json   # JSON
  wget -S   http://$POD_IP:8080/                          # 301 Location: /swagger
  wget -S   http://$POD_IP:8080/swagger                   # 301 Location: /swagger/
  wget -qO- http://$POD_IP:8080/swagger/                  # HTML page
  ```
- Verify pod starts and passes readinessProbe (`kubectl get pods` shows `Running 1/1`).
- Verify securityContext fields visible in `kubectl get pod -o yaml | grep -A10 securityContext`.
- Apply NetworkPolicy and verify Prometheus scraping still works (Prometheus target health check from CLAUDE.md §Local Cluster).
- Tag the commit `0.4.0`.
- **Exit:** All smoke tests pass; pod 1/1 Ready; Prometheus target `moviesx` is `up`; `git tag 0.4.0` applied.

---

## Parking Lot

Items deferred out of 0.4.0 scope:

- **actorId + rating filters on /api/movies** — already in parking lot from 0.2.0; unchanged.
- **CLI flags** (`--version`/`-v`, `--config`, etc.) — deferred from earlier sessions.
- **oapi-codegen migration** — if client SDK generation or contract testing is required, migrate from swag annotations to a hand-maintained OpenAPI YAML used as oapi-codegen input. The swag-generated `docs/swagger.json` can serve as the starting draft.
- **OAS 3.1 strict compliance** — swag `--v3.1` output has known gaps (no global `security` array, no `discriminator`, no `explode: false` on array params). None of these affect the current 7 routes, but a future schema-validation CI step may flag them.
- **Scalar UI** — no well-maintained Go go:embed integration as of 2026-06; revisit in 1.0.0 polish pass.
- **startupProbe** — not needed while `store.Load()` is synchronous. Add if loading becomes async.
- **emptyDir for /tmp** — not needed; no file-upload routes. Add if multipart parsing is introduced.
- **IPv6 NetworkPolicy** — k3s kube-router netpol controller does not support IPv6; not applicable to dev cluster.
- **Custom seccomp profile** — `RuntimeDefault` satisfies Restricted PSS; a tighter custom profile is out of scope for this experiment.
- **`make swagger` in CI** — not added; `docs/` is committed as a static artifact for 0.4.0.
- **`/metrics` NetworkPolicy egress rule for API server** — not needed; moviesx does not call the Kubernetes API.
- **Ingress object** — no Ingress manifest; access via `kubectl port-forward` or `docker exec` per CLAUDE.md §Local Cluster.
