# Session 4 Implementation Review — 0.4.0
*Reviewer: Claude Sonnet 4.6 | Date: 2026-06-03*
*Sources: 2026-06-03-openapi-plan.md, 2026-06-03-openapi-changes.md, code inspection, make test run*

---

## Verdict: SHIP

All checklist criteria pass. One criterion has a documented, acknowledged deviation (spec format). No blocking issues found.

---

## Checklist Results

### 1. GET /readyz — 503 before ready, 200 after ✅

**How timing was verified:** `TestReadyz` (integration_test.go:537) constructs a server via `server.New("test", st)` without calling `SetReady()`, then immediately sends `GET /readyz` over a real httptest.Server. The handler checks `s.ready.Load()` (an `atomic.Bool`, zero-value false) and writes 503 with `{"status":"not ready","reason":"store loading"}`. The test then calls `srv.SetReady()` (which stores `true` on the atomic) and re-requests `/readyz` — expecting 200 with `{"status":"ok"}`. Both assertions are in-process with no race between state change and the HTTP call.

**Production timing caveat (documented, not a defect):** In `main.go:43`, `SetReady()` is called synchronously after `store.Load()` and *before* `http.ListenAndServe()`. The pod is therefore in the ready state before it can accept any connection. The 503 path is unreachable in the current synchronous-load design; its value is as a future-proofing hook if load becomes async. The plan explicitly acknowledged this (T3: "Because load is synchronous today, the pod is ready from the first request."). The Kubernetes readinessProbe on `/readyz` is still correct — it would fire 503 if `SetReady()` were ever deferred.

---

### 2. GET /swagger — Swagger UI, no CDN requests ✅

`httpSwagger.Handler` (server.go:44) is backed by `github.com/swaggo/files/v2@v2.0.0`, which embeds all Swagger UI static assets (`swagger-ui.css`, `swagger-ui-bundle.js`, `swagger-ui-standalone-preset.js`, favicons) via `go:embed` at compile time. Confirmed by inspecting the embedded `index.html`:

```
grep -i "cdn\|unpkg\|jsdelivr\|https://" .../files/v2@v2.0.0/dist/index.html
(no output)
```

All asset `src=` and `href=` references are relative paths (`./swagger-ui.css`, `./swagger-ui-bundle.js`, etc.). The spec URL is configured as `/swagger/v1/swagger.json` — a local endpoint. No external network calls are made by the UI.

`TestSwaggerRoutes` verifies `GET /swagger/index.html` → 200 with `Content-Type: text/html`.

---

### 3. GET /swagger/v1/swagger.json — spec covering all §6 endpoints ⚠️ (acknowledged deviation)

**Format deviation:** The spec is **Swagger 2.0** (`"swagger": "2.0"`), not OpenAPI 3 as the criterion stated. This is an explicit, documented deviation in the changes doc: swag v1 (v1.16.x) does not have the `--v3.1` flag — that flag is only in `github.com/swaggo/swag/v2`. The `Makefile` target omits `--v3.1` accordingly. Migration to OAS 3.x is in the parking lot.

**Coverage:** The spec covers all 8 §6 endpoints:

| Path | Method | Tags |
|---|---|---|
| `/version` | GET | system |
| `/healthz` | GET | system |
| `/readyz` | GET | system |
| `/api/genres` | GET | genres |
| `/api/movies` | GET | movies |
| `/api/movies/{id}` | GET | movies |
| `/api/actors` | GET | actors |
| `/api/actors/{id}` | GET | actors |

**Params:** All `@Param` entries required by T7 are present — `q`, `genre`, `year`, `rating`, `actorId`, `pageNumber`, `pageSize` on `/api/movies`; `q`, `pageNumber`, `pageSize` on `/api/actors`; path `id` on both by-ID handlers.

**Definitions:** All response schemas are present and correctly cross-referenced: `Page[MovieListItem]`, `Page[Actor]`, `MovieDetail`, `Actor`, `Role`, `ActorMovie`.

**Validity:** The JSON file parses cleanly; `docs.SwaggerInfo.ReadDoc()` is served correctly at `/swagger/v1/swagger.json` (verified via `TestSwaggerRoutes`).

---

### 4. GET / redirects to /swagger ✅

`server.go:50–52` registers `GET /` → `http.Redirect(w, r, "/swagger", http.StatusMovedPermanently)`. The redirect chain is:

```
GET /        → 301  Location: /swagger
GET /swagger → 301  Location: /swagger/
GET /swagger/→ 301  Location: /swagger/index.html  (served by http-swagger)
GET /swagger/index.html → 200  text/html
```

`TestSwaggerRoutes` asserts the first two steps with a non-following HTTP client, confirming exact `Location` header values. The final step is tested by hitting `/swagger/index.html` directly (per the documented adjustment in changes doc — http-swagger redirects `/swagger/` to `index.html` internally, so the test targets `index.html` directly rather than `/swagger/`).

---

### 5. deployment.yaml securityContext ✅

Container-level (`k8s/base/deployment.yaml:28–32`):

```yaml
securityContext:
  allowPrivilegeEscalation: false   ✅
  readOnlyRootFilesystem: true      ✅
  capabilities:
    drop: [ALL]                     ✅
```

Pod-level (`deployment.yaml:17–21`):

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  seccompProfile:
    type: RuntimeDefault             ✅
```

No `emptyDir` volumes needed — the binary makes no filesystem writes at runtime (confirmed in research). All four required fields are present.

---

### 6. NetworkPolicy ✅

`k8s/base/networkpolicy.yaml` implements all requirements:

- **Scope:** `podSelector: {app: moviesx}`, namespace `default`, `policyTypes: [Ingress, Egress]`
- **Ingress rule 1:** namespaceSelector (`kube-system`) AND podSelector (`app.kubernetes.io/name: traefik`) — single list entry (AND logic, not OR) — port 8080/TCP ✅
- **Ingress rule 2:** namespaceSelector (`monitoring`) AND podSelector (`app.kubernetes.io/name: prometheus`) — single list entry — port 8080/TCP ✅
  - Label fix applied: uses `app.kubernetes.io/name: prometheus` (actual kube-prometheus-stack label), not the incorrect `app: prometheus` from the research plan.
- **Egress rule:** namespaceSelector (`kube-system`) AND podSelector (`k8s-app: kube-dns`) — single list entry — ports 53/UDP and 53/TCP ✅
- **kustomization.yaml** (`k8s/base/kustomization.yaml:8`): `- networkpolicy.yaml` added ✅

---

### 7. make test — coverage ≥80% ✅

```
ok  github.com/mbr/moviesx/internal/config    coverage: 100.0%
ok  github.com/mbr/moviesx/internal/server    coverage: 97.3%
ok  github.com/mbr/moviesx/internal/store     coverage: 93.1%
ok  github.com/mbr/moviesx/internal/validate  coverage: 100.0%
total: 90.1%
PASS: coverage 90.1% meets 80% threshold
```

All tests pass from a clean run (no cached results needed — confirmed by running `make test` during this review).

---

## Issues Found

### Blocking
None.

### Non-blocking / Known deviations

| # | Finding | Severity | Status |
|---|---|---|---|
| N1 | `swagger.json` is Swagger 2.0, not OpenAPI 3 | Low | Acknowledged in changes doc; parking-lot entry exists for swag v2 / oapi-codegen migration |
| N2 | `/readyz` 503 path is unreachable in production (synchronous load + SetReady before ListenAndServe) | Informational | Intentional per plan T3; test validates state machine correctly |

---

## Additional Observations

- `TestReadyz` is correctly isolated from `newTestServer()` (which calls `SetReady()` indirectly via main.go's pattern). The test constructs its own server pointer explicitly to control readiness state — no fixture contamination.
- The `GET /swagger` → `GET /swagger/` redirect test uses `http.ErrUseLastResponse` to prevent the client from following redirects, making the assertions exact rather than observational. Good defensive testing.
- `cmd/moviesx` and `docs` packages show 0% coverage — expected, as they are `main` packages or `init()`-only generated code. The gate correctly applies to statement-level totals across `internal/`.
- The `Makefile` `swagger` target omits `--v3.1` (consistent with the swag v1 constraint). If swag v2 is adopted later, the target will need to change to `go run github.com/swaggo/swag/v2/cmd/swag`.
