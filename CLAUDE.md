# CLAUDE.md — moviesx

Repo memory for the movies experiment. Updated at the close of each session.
Last updated: Session 2 (2026-06-02, tag 0.2.0).

## Stack

**Language:** Go (local toolchain: 1.26.3 via Homebrew; Docker builder pins `golang:1.22-bookworm`)  
**Module:** `github.com/mbr/moviesx`  
**Runtime image:** `gcr.io/distroless/static-debian12:nonroot` — static binary, no libc, 2.6 MB final image  
**Build:** `CGO_ENABLED=0 GOOS=linux go build ./cmd/moviesx`

## Project Layout

```
cmd/moviesx/main.go                  entry point — loads store, starts HTTP server
internal/config/config.go            env-var config layer (MOVIES_PORT, MOVIES_LOG_LEVEL, MOVIES_DATA_DIR)
internal/server/server.go            HTTP handler registration; server struct holds store + version
internal/server/genres.go            GET /api/genres
internal/server/movies.go            GET /api/movies, GET /api/movies/{id}
internal/server/actors.go            GET /api/actors, GET /api/actors/{id}
internal/server/integration_test.go  end-to-end tests via httptest against fixture dataset
internal/store/types.go              data types: Movie, Actor, Role, Page[T], MovieListItem, MovieDetail
internal/store/store.go              Load(dataDir) — parses data files, pre-joins ratings, pre-sorts
internal/validate/validate.go        input validation helpers (no net/http dependency)
k8s/deployment.yaml                  Deployment (non-root, liveness/readiness on /healthz)
k8s/service.yaml                     ClusterIP, port 80 → targetPort 8080
src/data/                            movies.json, actors.json, ratings.json (baked into image at /data)
Dockerfile                           multi-stage: golang:1.22-bookworm → distroless/static:nonroot
Makefile                             make test (runs tests + coverage gate), make build
```

## Config

| Env var | Default | Purpose |
|---|---|---|
| `MOVIES_PORT` | `8080` | HTTP listen port |
| `MOVIES_LOG_LEVEL` | `info` | Minimum log level |
| `MOVIES_DATA_DIR` | `/data` | Data file directory |

CLI flags deferred to a later session. Env-var config only for now.

## Key Implementation Decisions

- **Version string:** `var version = "0.2.0"` in `cmd/moviesx/main.go` — overridable via `-ldflags "-X main.version=X.Y.Z"` in future builds.
- **`runAsUser: 65532`** in Deployment `securityContext` — distroless nonroot UID; required because Kubernetes `runAsNonRoot` validation needs a numeric UID, not just the named user `nonroot`.
- **`go.mod` pins `go 1.22`** even though local toolchain is 1.26.3, to match the Docker builder image and keep builds reproducible.
- **D1 — Response envelope:** All list endpoints return `{ "items": [...], "total": N, "page": N, "pageSize": N }`. Implemented as `Page[T any]` generic in `internal/store/types.go`.
- **D2 — Roles scope:** `GET /api/movies` list items include cast-only roles (`actor`/`actress` categories) in a `cast` field. `GET /api/movies/{id}` includes all roles in a `roles` field.
- **Store immutability:** `store.Store` has no exported mutable fields. `AllMovies()` and `AllActors()` return copies of the internal sorted slices.
- **Pre-sorted at load:** movies sorted (rating desc, title asc), actors sorted (name asc) at `Load` time; handlers iterate and filter without re-sorting.
- **actorId + rating filters on `/api/movies`:** Deferred to Parking Lot — not implemented in 0.2.0.

## Local Cluster

- **Cluster:** k3d cluster named `movies`
- **Known issue:** k3d load-balancer failed to bind host port 3000 (occupied by a Next.js dev server). The LB pod is in `Pending`/`Error` state. This does not affect the API — use `kubectl port-forward` or `docker exec` for access.
- **kubectl access:** `docker exec k3d-movies-server-0 kubectl <args>` (avoids needing the LB)
- **Image import:** `k3d image import moviesx:<tag> -c movies`
- **Port-forward:** `kubectl port-forward svc/moviesx 8080:80`

## Testing

```
make test                          # all tests + coverage gate (≥80%)
go test ./...                      # all tests without coverage gate
go test ./internal/config/...      # config unit tests
go test ./internal/store/...       # store unit tests
go test ./internal/validate/...    # validation unit tests
go test ./internal/server/...      # server unit + integration tests
```

Coverage as of 0.2.0: **92.4%** total (gate: 80%).

## Build & Deploy Inner Loop (Session 2)

```bash
make test
docker build -t moviesx:0.2.0 .
k3d image import moviesx:0.2.0 -c movies
# update image tag in k8s/deployment.yaml
docker exec k3d-movies-server-0 kubectl apply -f /path/to/k8s/
kubectl port-forward svc/moviesx 8080:80
curl localhost:8080/version
curl localhost:8080/healthz
curl localhost:8080/api/genres
curl "localhost:8080/api/movies?genre=Action&pageSize=5"
curl localhost:8080/api/movies/tt0120737
curl "localhost:8080/api/actors?q=Wood"
curl localhost:8080/api/actors/nm0000704
```

## Session Map

| Tag | Goal | Status |
|---|---|---|
| `0.1.0` | Stack + /version + /healthz on k3s | ✅ done |
| `0.2.0` | Data layer + /api/* endpoints + validation + tests ≥80% | ✅ done |
| `0.3.0` | Prometheus metrics + structured logging + Grafana + Kustomize | next |
| `0.4.0` | OpenAPI/Swagger + /readyz + security hardening + NetworkPolicy | — |
| `0.5.0` | Custom HTTP replay tool (§10.3 baseline + §10.4 benchmark) | — |
| `1.0.0` | §14 gap close + inner-loop README + acceptance pass | — |

## Spec References

- Full spec: [docs/spec.md](docs/spec.md)
- Acceptance criteria: [docs/spec.md §14](docs/spec.md#14-acceptance-criteria)
- Methodology: [docs/METHODOLOGY.md](docs/METHODOLOGY.md)
- Session log: [session-log.md](session-log.md)
- RPI artifacts: [.copilot-tracking/](.copilot-tracking/)
