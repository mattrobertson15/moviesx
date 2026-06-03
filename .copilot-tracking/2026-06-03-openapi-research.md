# OpenAPI & Security Hardening Research
*Date: 2026-06-03 | Project: moviesx | Session prep for 0.4.0*

---

## 1. OpenAPI 3 Spec Generation

### Context

moviesx has 7 routes already written as plain `net/http` handlers with no framework:
`GET /version`, `GET /healthz`, `GET /api/genres`, `GET /api/movies`, `GET /api/movies/{id}`,
`GET /api/actors`, `GET /api/actors/{id}`, plus `GET /metrics` (Prometheus). The goal is to
surface an OpenAPI 3 spec (JSON or YAML) at `/openapi.json` and serve a UI at `/docs`.

---

### Option A: swaggo/swag

**Import path:** `github.com/swaggo/swag` (v1) / `github.com/swaggo/swag/v2` (v2, active)
**UI companion:** `github.com/swaggo/http-swagger/v2`
**Install CLI:** `go install github.com/swaggo/swag/cmd/swag@latest`

**How it works:**

swag is a source-code annotation scanner. You add structured comment blocks directly above your
handler functions and above `main()` (for global metadata). The `swag init` CLI command scans all
Go files starting from a specified directory, parses the annotations, and writes generated files
into a `docs/` directory:

- `docs/docs.go` — an init() that registers the spec with the swag runtime
- `docs/swagger.json` — the raw spec as JSON
- `docs/swagger.yaml` — the raw spec as YAML

You import `_ "yourmodule/docs"` for its side-effect and wire the UI handler.

**Annotation example for a handler:**

```go
// handleMovies godoc
// @Summary      List movies
// @Description  Returns paginated list of movies with optional filtering
// @Tags         movies
// @Produce      json
// @Param        q         query  string  false  "Title search substring"
// @Param        genre     query  string  false  "Genre filter"
// @Param        year      query  int     false  "Release year"
// @Param        pageNumber query int    false  "Page number (default 1)"
// @Param        pageSize  query  int     false  "Page size (default 20, max 100)"
// @Success      200  {object}  store.Page[store.MovieListItem]
// @Failure      400  {object}  map[string]string
// @Router       /api/movies [get]
func (s *server) handleMovies(w http.ResponseWriter, r *http.Request) {
```

**Global annotation** goes in a comment block in `main.go`:

```go
// @title          moviesx API
// @version        0.4.0
// @description    Read-only movie catalog API
// @host           localhost:8080
// @BasePath       /
```

**Developer workflow:**

1. Add annotations to handlers and main.go.
2. Run `swag init -g cmd/moviesx/main.go --parseDependency` (or `make swagger`).
3. The `docs/` directory is regenerated. Commit it (or regenerate in CI).
4. Import `_ "github.com/mbr/moviesx/docs"` and wire `http-swagger`.
5. Repeat step 2 whenever handlers change.

**OpenAPI version output:**

swag's primary native output is **Swagger 2.0**. OpenAPI 3.x output is available via the flag
`swag init --v3.1` (v2 branch only). However, the v3.1 flag is still experimental and carries
several known gaps:

- No way to specify `explode: false` for array query parameters (defaults to `true`).
- Global top-level `security` array is not emitted in v3 mode (only per-operation security).
- `discriminator` annotation for `oneOf` polymorphism is missing.
- The conversion is done internally by translating the 2.0 AST — meaning subtle schema differences
  between Swagger 2.0 and OAS 3 are not always handled correctly.

An alternative is to generate Swagger 2.0 and pipe it through the official converter
`swagger2openapi` (Node tool, or the swagger.io hosted converter). This adds a build step.

**Pros for moviesx:**
- No handler rewrites. Annotations are additive comments.
- Very large community; tons of examples for `net/http`.
- Generates spec and registers it at build time; no runtime spec-building overhead.
- `http-swagger/v2` uses `go:embed` internally — no CDN dependency.

**Cons for moviesx:**
- Native output is Swagger 2.0, not OAS 3.0 or 3.1. The `--v3.1` flag is experimental.
- Annotations are stringly typed; typos in field names are not caught until `swag init` runs.
- Annotations must stay in sync with code manually — no compile-time enforcement.
- Adds a `docs/` package to the module (generated code in VCS or requires CI step).
- Parsing generics (`Page[T]`) can require `--parseInternal --parseDependency` flags and may
  occasionally fail to resolve; requires extra flags: `swag init --parseDependency --parseInternal`.
- The `swag` CLI binary is a dev dependency not in `go.mod` (must be installed separately or via
  `go tool`).

---

### Option B: oapi-codegen (oapi-codegen/oapi-codegen)

**Import path (current):** `github.com/oapi-codegen/oapi-codegen/v2`
**Old import path (pre-May 2024):** `github.com/deepmap/oapi-codegen/v2` — do NOT use
**Install CLI:** `go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest`
**net/http middleware:** `github.com/oapi-codegen/nethttp-middleware`

**Important migration note:** In May 2024 the project moved from the `deepmap` GitHub org to its
own org `oapi-codegen`. Any project still importing from `deepmap` is pinned to pre-May-2024 code.
The current canonical import path is `github.com/oapi-codegen/oapi-codegen/v2`.

**How it works:**

oapi-codegen is **spec-first**: you write the OpenAPI 3 YAML first, then run the code generator,
which produces:

- A `ServerInterface` — an interface whose methods exactly match your operation IDs, with path
  parameters and query parameters parsed from the spec and passed as typed Go function arguments.
- `RegisterHandlers()` — wires the interface implementation to a router.
- Type structs for all request/response schemas defined in the spec.

Since v2.2.0 (March 2024), a `std-http` generator target is supported, which uses Go 1.22's
enhanced `net/http` routing (pattern `GET /api/movies/{id}`). Prior versions required chi, echo, or
gin.

**Configuration file** (`oapi-codegen.yaml`):
```yaml
package: server
generate:
  std-http-server: true
  models: true
output: internal/server/api.gen.go
```

**Developer workflow:**

1. Write `api/openapi.yaml` first (the source of truth).
2. Add `//go:generate oapi-codegen -config oapi-codegen.yaml api/openapi.yaml` to a Go file.
3. Run `go generate ./...` to produce `api.gen.go`.
4. Implement the generated `ServerInterface` in your handler structs.
5. Call `RegisterHandlers(mux, myServer)` to wire routes.

**Retrofitting existing handlers:**

For moviesx this means rewriting handler signatures to match the generated interface. The generated
interface for `GET /api/movies` would look roughly like:

```go
type ServerInterface interface {
    GetApiMovies(w http.ResponseWriter, r *http.Request, params GetApiMoviesParams)
    GetApiMoviesId(w http.ResponseWriter, r *http.Request, id string)
    // etc.
}
```

The body of each handler can be moved verbatim. The main change is that path/query params arrive
as typed structs rather than being parsed via `r.URL.Query()` calls inside the handler — though
the existing `validate` package calls can remain.

**Pros for moviesx:**
- Spec is the authoritative document; code and spec are always in sync (compile-time enforcement).
- OAS 3.0 native (not a 2.0 conversion). Can be written as OAS 3.1 if needed (see note below).
- Generated `ServerInterface` prevents routing/handler drift.
- `std-http-server` target works with Go 1.22+ `net/http` — no framework dependency.
- Active development; well-maintained.

**Cons for moviesx:**
- Requires writing the entire OpenAPI spec from scratch (or from scratch for 7 routes — this is
  not a huge burden, but it is work).
- Handlers must conform to the generated interface; cannot be wired in ad hoc.
- OAS 3.1 support: as of early 2026, oapi-codegen officially targets OAS 3.0; OAS 3.1 can be
  tricked into working (see jvt.me post from May 2025) but is not fully supported.
- The generated file is checked in or regenerated; adds a CI step.

---

### Option C: huma v2

**Import path:** `github.com/danielgtaylor/huma/v2`
**Go requirement:** Go 1.25+ (as of current version — verify before adopting)
**Website:** https://huma.rocks

**How it works:**

Huma is a micro-framework that wraps any router (including `net/http`, chi, gin, echo, etc.) with
an OpenAPI-aware registration layer. Instead of calling `mux.Handle(...)`, you call:

```go
huma.Register(api, huma.Operation{
    OperationID: "list-movies",
    Method:      http.MethodGet,
    Path:        "/api/movies",
    Summary:     "List movies",
}, func(ctx context.Context, input *ListMoviesInput) (*ListMoviesOutput, error) {
    // handler body
})
```

Huma introspects the `ListMoviesInput` struct's field tags (`query:"q"`, `path:"id"`, etc.) and
the `ListMoviesOutput` struct to build the OpenAPI 3.1 schema at startup. The spec is auto-served
at `/openapi.json` and `/openapi.yaml`. Swagger UI is served at `/docs` by default.

**Integrating with existing net/http mux:**

Huma supports `humahttp` adapter for `net/http`. You can mix Huma-registered operations with plain
`mux.Handle()` calls on the same mux. However, handlers must be rewritten into the Huma input/output
struct pattern — existing `http.HandlerFunc` functions cannot be registered with Huma as-is.

**Retrofitting existing handlers:**

Each handler needs to be restructured:
- Query params move from `r.URL.Query().Get(...)` calls to fields on an `Input` struct.
- Response bodies move from `writeJSON(w, 200, v)` calls to returning a typed output struct.
- Validation moves from the `validate` package to struct field tags and Huma's built-in validators.

For moviesx's 7 handlers this is a non-trivial but bounded rewrite.

**Pros for moviesx:**
- Generates OAS 3.1 (the most current spec version) automatically.
- No separate spec file to maintain; spec is derived from code.
- Swagger UI included by default (embedded).
- Input/output types are enforced at compile time.
- Automatic validation of inputs per OpenAPI constraints.

**Cons for moviesx:**
- Requires rewriting all handlers into Huma's input/output struct pattern.
- Introduces a framework dependency where there was none (counter to the project's "stdlib only"
  philosophy).
- Go version requirement is now 1.25+ (project currently targets 1.23).
- The generated spec format can be surprising (strict JSON Schema / OAS 3.1 semantics differ from
  OAS 3.0 in nullable handling, etc.).
- Larger dependency footprint.

---

### Recommendation

**Use `swaggo/swag` with `--v3.1` flag, accepting OAS 3.1 output with known limitations.**

Rationale:

1. **Zero handler rewrites.** moviesx already has well-tested, working handlers. swag annotations
   are purely additive comments; the handlers and their tests do not change.

2. **Smallest dependency surface.** swag is a dev/build-time CLI tool plus a tiny runtime import
   (`docs` package). oapi-codegen adds a generated `ServerInterface` that the entire server package
   must conform to. huma introduces a framework.

3. **7 routes is a small annotation burden.** Writing ~5 annotation lines per handler for 7 routes
   is 35 lines total — fast and low-risk.

4. **OAS 3.x limitations are acceptable here.** moviesx has no polymorphic responses, no
   `oneOf`/`discriminator` shapes, no global security arrays. The known gaps in swag's `--v3.1`
   mode do not apply to this API's shape.

5. **If strict OAS 3.0 spec is required in the future**, the migration path from swag annotations
   to a hand-maintained spec (for oapi-codegen) is straightforward: `swag init` can produce a draft
   spec that is then cleaned up and used as the oapi-codegen input.

**Alternative if spec correctness is paramount:** oapi-codegen v2 with `std-http-server` target.
Writing the OpenAPI YAML for 7 routes is roughly 200-300 lines of YAML and the handler body can
be moved verbatim; only the signature changes. This approach should be chosen if the spec will be
used to generate client SDKs or drive contract testing.

---

## 2. Serving Swagger UI (no CDN)

### Option A: swaggo/http-swagger (v2)

**Import path:** `github.com/swaggo/http-swagger/v2`

http-swagger is a thin `net/http` wrapper that serves Swagger UI's static assets, bundled via
`go:embed`. The library embeds a pinned copy of swagger-ui-dist. As of v2, it requires Go 1.16+
for `embed` support.

**Wiring to net/http ServeMux:**

```go
import (
    _ "github.com/mbr/moviesx/docs" // generated by swag init
    httpSwagger "github.com/swaggo/http-swagger/v2"
)

// In server.New():
mux.Handle("GET /docs/", httpSwagger.Handler(
    httpSwagger.URL("/docs/doc.json"), // path where spec is served
))
```

The handler serves:
- `GET /docs/` → redirect to `/docs/index.html`
- `GET /docs/index.html` → Swagger UI HTML
- `GET /docs/doc.json` → the generated spec (re-served from the `docs` package)
- `GET /docs/oauth2-redirect.html` → OAuth2 redirect page (unused for read-only APIs)
- All other `/docs/*` → static swagger-ui-dist assets (JS, CSS, fonts)

**Gotchas:**
- The handler must be registered as a **prefix route** (`GET /docs/` with trailing slash) for the
  stdlib mux to pass sub-paths to it. Without the trailing slash the mux will only match `/docs`
  exactly.
- The `httpSwagger.URL(...)` argument must point to where the spec JSON is accessible from the
  browser, not a server-internal path. For this project: `/docs/doc.json`.
- The spec is embedded in the generated `docs` package as a Go string, not served as a file.
  `http-swagger` re-exposes it at `doc.json`. Do not also serve the file separately.
- In a distroless container there is no `/docs` directory conflict; the route is purely in-memory.

---

### Option B: swaggest/swgui

**Import path (embedded):** `github.com/swaggest/swgui/v5emb`
**Import path (CDN):** `github.com/swaggest/swgui/v5cdn`

swgui provides a standalone `http.Handler` for Swagger UI with all assets embedded via `go:embed`.
The embedded version (`v5emb`) uses Go 1.16+ native embed. The CDN version (`v5cdn`) fetches assets
from `cdnjs.cloudflare.com` — NOT suitable for a distroless/air-gapped image.

The current embedded version is built from **swagger-ui v5.29.1** (as of June 2026).

**Usage:**

```go
import "github.com/swaggest/swgui/v5emb"

mux.Handle("GET /docs/", v5emb.New("moviesx API", "/openapi.json", "/docs/"))
```

`New(title, specURL, basePath)` returns an `http.Handler`. The third argument must match the route
prefix under which it is mounted.

**Gotchas:**
- `basePath` must match the mux prefix exactly, including the trailing slash.
- The spec URL (`/openapi.json`) is fetched by the browser — it must be a route the server also
  serves.
- Binary size impact: the `v5emb` package embeds the full swagger-ui dist (~1.6 MB of minified
  JS/CSS). This is acceptable for a development/staging environment; for a production binary add
  a build tag or a `-tags nouiembed` equivalent.
- No dependency on swag/annotations — swgui only serves the UI; it does not generate or serve the
  spec itself. The spec route must be wired separately.

---

### Option C: Manual go:embed of swagger-ui-dist

For full control, download `swagger-ui-dist` from npm, vendor the files, and embed with `//go:embed`:

```
internal/swagger/dist/          # vendored from npm swagger-ui-dist@5.x
internal/swagger/embed.go       # //go:embed all:dist
```

```go
//go:embed all:dist
var swaggerDist embed.FS

func Handler(specURL string) http.Handler {
    sub, _ := fs.Sub(swaggerDist, "dist")
    fileServer := http.FileServer(http.FS(sub))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // rewrite index.html to inject the specURL
        // serve static files via fileServer
    })
}
```

The swagger-ui-dist npm package is currently at **v5.32.6** (June 2026). The dist directory
contains ~25 files including `swagger-ui-bundle.js` (~1.4 MB), `swagger-ui.css` (~270 KB),
`index.html`, and `oauth2-redirect.html`.

**Gotchas:**
- `index.html` hardcodes the `url:` for the Petstore spec. You must either patch the HTML or
  serve a custom `index.html` that reads `?url=` from the query string.
- `oauth2-redirect.html` must be served at the same path prefix for OAuth2 flows (unused here
  but can cause 404 noise in logs).
- `fs.Sub()` is needed to strip the `dist/` prefix from the embedded FS paths.
- Vendoring the dist files adds ~2 MB to the repository. Use `.gitignore` with a pre-build step,
  or use a `go generate` script that downloads from npm.

---

### Option D: Scalar

**Repository:** https://github.com/scalar/scalar
**Go embedding:** The Scalar UI is a single HTML page that can be served as a `go:embed` file.

Scalar is an actively maintained open-source alternative to Swagger UI (14K+ GitHub stars as of
2026). It provides cleaner UI, multi-language code snippet generation (curl, Go, Python, etc.),
and supports OAS 3.0/3.1 and Swagger 2.0.

**Embedding Scalar in Go:**

Scalar provides a standalone HTML page (single file) that references CDN assets by default.
For a no-CDN approach, download the self-contained `@scalar/api-reference` bundle, embed the JS,
and serve a wrapper HTML. This is a more manual process than using swgui.

As of 2026, there is no well-maintained Go library that embeds Scalar's static assets with `go:embed`
in the same way `swaggest/swgui` does. The easiest path is to use Scalar's `cdn: false` standalone
HTML mode with the JS file manually vendored.

---

### Recommendation for moviesx

Use **`swaggo/http-swagger/v2`** if using swag for spec generation (the two are designed to work
together). If using oapi-codegen or a hand-written spec, use **`swaggest/swgui/v5emb`** for the UI.

For the `/docs/*` route registration with Go 1.22+ `net/http`:

```go
mux.Handle("GET /docs/", httpSwagger.Handler(
    httpSwagger.URL("/docs/doc.json"),
))
```

The index.html redirect gotcha: accessing `/docs` (no trailing slash) will 404 with the stdlib mux
if only `GET /docs/` is registered. Either also register `GET /docs` with a redirect to `/docs/`,
or accept that the canonical URL is `/docs/`.

---

## 3. GET /readyz Pattern

### liveness vs. readiness — the distinction

| Probe | Path | Semantics | Failure action |
|---|---|---|---|
| liveness | `/healthz` | "the process is alive; kill it if this fails" | kubelet restarts the container |
| readiness | `/readyz` | "this pod is ready to serve traffic" | kubelet removes pod from Service endpoints |

For moviesx, `store.Load()` is called synchronously before `server.New()` in `main.go` today — the
server does not start until the store is loaded. This means the existing `/healthz` probe currently
doubles as readiness. However, splitting them is a Kubernetes best practice and enables future
cases where re-loading or warmup is needed.

### HTTP status codes

The Kubernetes convention (documented at kubernetes.io) is:
- **200–399** → probe success
- **400+** → probe failure

**503 Service Unavailable** is the correct status code for "not yet ready". It unambiguously signals
a temporary condition (compare 404 which implies the resource doesn't exist, or 500 which implies
a bug). The Kubernetes community uses 503 universally for not-ready readiness probes.

Response body should be a plain-text or JSON message indicating the state. Kubernetes itself ignores
the body; it is for human operators. Example bodies:
- Not ready: `{"status":"not ready","reason":"store loading"}` 
- Ready: `{"status":"ok"}`

### Synchronization primitive

For this specific case — a boolean flag set once at startup — three approaches are viable:

**1. `atomic.Bool` (recommended)**

```go
import "sync/atomic"

type server struct {
    version string
    store   *store.Store
    ready   atomic.Bool
}

func (s *server) handleReadyz(w http.ResponseWriter, r *http.Request) {
    if !s.ready.Load() {
        writeJSON(w, http.StatusServiceUnavailable, map[string]string{
            "status": "not ready", "reason": "store loading",
        })
        return
    }
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

Set from `main.go` after `store.Load()` completes and before the HTTP server starts:

```go
h, readyFn := server.New(version, st)
// or: server.New returns *server directly and readyFn is s.ready.Store(true)
go func() {
    // In the current design, store loads synchronously in main, so:
    readyFn()
}()
```

Since the current design loads synchronously, `ready.Store(true)` is called immediately in
`main.go` after `store.Load()` succeeds. The readiness probe will return 200 from first
request. The value of splitting is future-proofing for async reload.

`atomic.Bool` (`sync/atomic`, Go 1.19+) is safe for concurrent reads from many goroutines
(one per incoming probe request) with a single write from the main goroutine. It is the
simplest primitive with no cleanup required.

**2. `sync.Once` (wrong tool here)**

`sync.Once` is for "run a function exactly once". It is not a state query — you cannot ask
"has this been called yet" without an additional flag. Use `atomic.Bool` instead.

**3. Channel close**

```go
ready := make(chan struct{})
// mark ready:
close(ready)
// check:
select {
case <-ready: // ready
default: // not yet
}
```

This is idiomatic Go but adds complexity for a simple boolean. The non-blocking select is harder
to read than `atomic.Bool.Load()`. Prefer `atomic.Bool`.

**4. `sync.RWMutex`**

Overkill for a single boolean. Use `atomic.Bool`.

### Where to call `ready.Store(true)` in moviesx

In the current `main.go`, the sequence is:
1. `store.Load(cfg.DataDir)` — synchronous, exits on error
2. `server.New(version, st)` — constructs and wires the mux
3. `http.ListenAndServe(...)` — starts accepting connections

The server struct must own the `atomic.Bool`. Options:
- Add `ready atomic.Bool` to the `server` struct in `internal/server/server.go`.
- Export a `SetReady()` method or have `New()` return a "mark ready" function.
- Call `SetReady()` (or equivalent) in `main.go` after `server.New()` returns but before
  (or just after) `ListenAndServe` — since loading is synchronous, this means the pod will
  be ready immediately on startup, which is correct.

For the async-load future case, `store.Load()` would become async, and `SetReady()` would be
called from the goroutine watching for load completion.

### Kubernetes probe configuration

The Deployment's `readinessProbe` should point to `/readyz` instead of `/healthz`:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 2
  periodSeconds: 10
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 2
  periodSeconds: 5
  failureThreshold: 3
```

For moviesx (fast startup, synchronous store load), `initialDelaySeconds: 2` and
`failureThreshold: 3` are fine. The first readiness probe will succeed immediately.

If store loading were async and could take up to N seconds, consider:
- `initialDelaySeconds: N` to avoid early probe failures
- Or a `startupProbe` that allows longer warmup without affecting liveness (not needed for moviesx now)

---

## 4. securityContext Hardening

### Current state

The deployment already has:
```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
```

These are at the **pod** level. The target for 0.4.0 is to add **container-level** security context
hardening. All fields below go under `spec.containers[0].securityContext`.

---

### readOnlyRootFilesystem: true

**Does a static Go binary need any writable paths?**

For a Go HTTP server that:
- Makes no outbound HTTPS calls (no dial-out to external APIs)
- Writes no files (no temp files, no log files — logging is to stdout via slog)
- Has no shell or cgo

The answer is **no writable paths are needed at runtime**. Specifically:

- `/tmp` — not needed. The Go runtime does not use `/tmp` for a plain HTTP server. `net/http`
  does not create temp files unless you call `r.FormFile()` or `ParseMultipartForm()` with files
  that exceed the in-memory limit. moviesx has no file upload endpoints.
- `/etc/ssl/certs` — already present read-only in `distroless/static`. The CA cert bundle is
  baked into the image. Since moviesx makes no outbound TLS calls (it only serves, not dials),
  this is moot anyway.
- Go runtime temp dir — the Go runtime may use `os.TempDir()` for certain operations (e.g.,
  `testing` package, pprof goroutine dumps), but in a plain HTTP server none of these code paths
  are exercised at runtime. `CGO_ENABLED=0` eliminates all C runtime temp usage.
- `/proc` — read-only access to `/proc/self` is needed by some profiling tools but not by
  the Prometheus client library's default HTTP metrics handler.

**distroless/static filesystem layout:**
The `gcr.io/distroless/static-debian12:nonroot` image contains:
- `/etc/passwd` and `/etc/group` (with root, nobody, nonroot entries)
- `/etc/ssl/certs/ca-certificates.crt`
- `/usr/share/zoneinfo/` (timezone data)
- `/tmp` — present and writable (owned by root, world-writable via sticky bit)
- No shell, no package manager, no libc

So `/tmp` is technically available but the Go server will not write to it.

**Conclusion:** `readOnlyRootFilesystem: true` is safe for moviesx with no volume mounts needed.

If future code paths require `/tmp` (e.g., large multipart uploads), add:
```yaml
volumeMounts:
  - name: tmp
    mountPath: /tmp
volumes:
  - name: tmp
    emptyDir: {}
```

---

### capabilities: drop: [ALL]

A plain HTTP server listening on a non-privileged port (8080, not 80 or 443) requires **no Linux
capabilities** at runtime:

- `NET_BIND_SERVICE` — only needed to bind ports < 1024. Port 8080 does not require it.
- `CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `SETUID`, `SETGID` — not needed; no file ownership changes.
- `SYS_PTRACE` — not needed.
- `NET_ADMIN` — not needed.

**Conclusion:** `capabilities: drop: [ALL]` is safe. Do not add any capabilities back.

If the port were changed to 443 or 80 in future, you would need to either:
- Keep port 8080 behind a Kubernetes Service/Ingress (correct approach), or
- Add `add: [NET_BIND_SERVICE]`

---

### allowPrivilegeEscalation: false

No gotchas for a static Go binary. This flag prevents the process from gaining more privileges
than its parent via `setuid` binaries or kernel mechanisms. Since the binary is not setuid and
runs as nonroot (UID 65532) with no capabilities, this flag has no functional effect but should
always be set explicitly to satisfy security scanners (Trivy, Checkov, kube-bench).

Setting `allowPrivilegeEscalation: false` is required by the Kubernetes Restricted Pod Security
Standard (PSS) and is therefore mandatory for clusters enforcing PSS.

---

### seccompProfile: RuntimeDefault

**What it does:** Applies the container runtime's built-in seccomp filter, which allows a broad
safe list of syscalls and denies known-dangerous ones (e.g., `ptrace`, `mount`, `kexec_load`,
`clone` with certain flags). This is less restrictive than a custom profile but much better than
`Unconfined`.

**Is it safe for a static Go binary in distroless?**

Yes, with the following caveats:

1. **Go runtime syscall usage:** The Go runtime uses a known set of syscalls (`futex`, `epoll`,
   `read`, `write`, `mmap`, `clone` for goroutines, `sigaltstack`, `rt_sigaction`). All of these
   are in the RuntimeDefault allow list for both `containerd` and `CRI-O`.

2. **Runtime differences between containerd and CRI-O:** The exact allow list differs between
   runtimes and versions. In practice, a plain Go HTTP server has never been observed to trigger
   a RuntimeDefault seccomp block in containerd (k3d uses containerd).

3. **`crun` vs `runc` error modes:** If a blocked syscall is hit, `crun` returns `EPERM` while
   `runc` returns `ENOSYS`. This difference can cause subtle failures, but it is not relevant
   since the Go HTTP server does not call any blocked syscalls.

4. **k3d/containerd version:** k3d uses containerd. RuntimeDefault in containerd 1.6+ is based on
   the Docker/Moby default seccomp profile, which allowlists all syscalls needed by Go.

5. **Kubernetes version requirement:** `seccompProfile` in the pod/container spec requires
   Kubernetes 1.19+ (GA in 1.22). k3d with a recent k3s version supports this.

**Gotcha:** Setting `seccompProfile` at the container level overrides any pod-level setting.
If both are set, the container-level wins.

**Recommendation:** Set `seccompProfile: {type: RuntimeDefault}` at the **pod** spec level
(alongside the existing `runAsNonRoot`/`runAsUser`) so it applies uniformly:

```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
```

---

### runAsNonRoot: true + runAsUser: 65532

Already in the deployment. Confirming correctness:

- `gcr.io/distroless/static-debian12:nonroot` configures UID 65532 as the `nonroot` user.
- The Dockerfile uses `USER nonroot:nonroot` as the final user directive.
- Kubernetes `runAsNonRoot: true` validates that the UID is not 0. Since UID 65532 is non-zero,
  this passes.
- Using a numeric UID (`runAsUser: 65532`) rather than the named user (`nonroot`) is required
  because Kubernetes's `runAsNonRoot` validation is done against the numeric UID in the image
  manifest, not the `/etc/passwd` name. Named-user-only specs can fail admission in some clusters.

**Reference:** `gcr.io/distroless/static-debian12:nonroot` image documentation:
https://github.com/GoogleContainerTools/distroless

---

### Full recommended container securityContext

```yaml
containers:
  - name: moviesx
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop: [ALL]
```

Combined with the pod-level:
```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    seccompProfile:
      type: RuntimeDefault
```

This configuration satisfies the Kubernetes **Restricted** Pod Security Standard — the most
hardened built-in policy level.

---

## 5. NetworkPolicy

### CNI/enforcement note for k3d

**k3d uses k3s, which uses Flannel as its default CNI.** Flannel alone does NOT enforce
NetworkPolicy objects — it treats them as no-ops.

However, **k3s ships with an embedded network policy controller** (based on kube-router's `netpol`
library). This controller enforces NetworkPolicy rules at the iptables/ipset level alongside
Flannel, without requiring a full CNI replacement (Calico, Cilium, etc.). The controller can
be disabled with `--disable-network-policy` flag on the k3s server, but is enabled by default.

**Important limitation:** The kube-router network policy controller in k3s does not support IPv6.
For the IPv4-only dev cluster this is not a concern.

**Verification:** Apply a deny-all policy and test with `wget`/`curl` to confirm enforcement. In
some k3d versions the embedded controller may be present but not enforce all edge cases.

---

### Namespace labels

NetworkPolicy `namespaceSelector` matches namespace **labels**, not names. Since Kubernetes 1.21,
the control plane **automatically adds an immutable label to every namespace**:

```
kubernetes.io/metadata.name: <namespace-name>
```

This means you do not need to manually label `kube-system`, `monitoring`, `default`, or any other
namespace to use them in NetworkPolicy selectors — use `kubernetes.io/metadata.name` as the
selector key.

**For k3d/k3s with Traefik:** Traefik's ingress controller runs in the `kube-system` namespace
(deployed by k3s as a DaemonSet/Deployment). Use `kubernetes.io/metadata.name: kube-system` for
the namespace selector to allow ingress from Traefik.

---

### Precise policy rules for moviesx

**Policy 1: Ingress to moviesx pods**

Target: pods with label `app=moviesx` in namespace `default`.

Ingress rules:

1. **Allow from ingress controller (Traefik in k3s/k3d):**
   - `namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}`
   - No `podSelector` restriction (Traefik pods may have varying labels across k3s versions;
     alternatively use `podSelector: {matchLabels: {app.kubernetes.io/name: traefik}}`)
   - Port: TCP 8080

2. **Allow from Prometheus:**
   - `namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: monitoring}}`
   - `podSelector: {matchLabels: {app: prometheus}}`
   - Port: TCP 8080

3. **Deny all other ingress** — implicit when any `Ingress` rule is present in a NetworkPolicy.
   No additional rule needed; the policy itself creates the default-deny for the targeted pods.

**Policy 2: Egress from moviesx pods**

1. **Allow DNS to kube-dns/CoreDNS:**
   - `namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}`
   - `podSelector: {matchLabels: {k8s-app: kube-dns}}`
   - Ports: UDP 53, TCP 53 (both; TCP fallback for large DNS responses)

2. **Deny all other egress** — implicit when any `Egress` rule is present in a NetworkPolicy.

---

### Policy description summary

**NetworkPolicy: `moviesx-ingress-egress`**
- `spec.podSelector: matchLabels: app: moviesx`
- `spec.policyTypes: [Ingress, Egress]`

Ingress from Traefik (kube-system namespace):
```
namespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: kube-system
podSelector:
  matchLabels:
    app.kubernetes.io/name: traefik
port: 8080/TCP
```

Ingress from Prometheus (monitoring namespace):
```
namespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: monitoring
podSelector:
  matchLabels:
    app: prometheus
port: 8080/TCP
```

Egress to CoreDNS (kube-system namespace):
```
namespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: kube-system
podSelector:
  matchLabels:
    k8s-app: kube-dns
ports: 53/UDP, 53/TCP
```

All other ingress: **denied**
All other egress: **denied**

---

### Gotchas

1. **AND vs OR logic for combined selectors:** When `namespaceSelector` and `podSelector` are
   both specified **inside the same `from` rule entry**, Kubernetes applies AND logic — the pod
   must be in the matching namespace AND have the matching label. If they are in separate list
   entries (using `-` in YAML), Kubernetes applies OR logic. Use a single list entry (AND) for
   cross-namespace pod targeting:
   ```yaml
   - from:
     - namespaceSelector:
         matchLabels:
           kubernetes.io/metadata.name: monitoring
       podSelector:
         matchLabels:
           app: prometheus
   ```
   NOT two separate `-` entries, which would allow all pods in `monitoring` OR all `app:prometheus`
   pods in any namespace.

2. **`monitoring` namespace label:** The `kubernetes.io/metadata.name` label is automatically set
   on the `monitoring` namespace when it is created. No manual label step is required.

3. **Flannel + k3d NetworkPolicy enforcement:** Always verify enforcement after applying. The
   embedded kube-router controller in k3s should enforce the policies, but test with:
   ```bash
   # from inside a pod NOT in the allow list — should be blocked:
   kubectl run -it test --image=busybox --restart=Never -- wget -qO- http://<moviesx-pod-ip>:8080/healthz
   ```

4. **Prometheus scraping port:** The NetworkPolicy allows port 8080 from Prometheus. The
   `/metrics` endpoint is served on the same port (8080), so no additional rule is needed.

5. **Inter-pod communication within the same namespace:** If multiple moviesx replicas need to
   communicate (not the case for this read-only API), add a rule allowing ingress from
   `podSelector: {matchLabels: {app: moviesx}}` to enable pod-to-pod traffic within the
   same namespace.

6. **Egress to Kubernetes API server:** moviesx does not call the k8s API, so no egress rule
   for port 443 (or 6443 for k3d) is needed. If future code imports `k8s.io/client-go`, add
   an egress rule for the API server IP/port.

---

## Summary of Key Decisions

- **OpenAPI generation:** Use `swaggo/swag` with annotation comments (additive, no handler rewrites).
  Generate with `swag init --v3.1` for OAS 3.1 output. Known v3.1 flag limitations (no global
  security array, no discriminator, no array explode=false) do not affect moviesx's 7 read-only routes.
  Migrate to oapi-codegen spec-first approach if/when client SDK generation is required.

- **Swagger UI embedding:** Use `swaggo/http-swagger/v2` (go:embed, no CDN, works with http-swagger's
  `httpSwagger.Handler`). Register as `GET /docs/` (trailing slash required for prefix matching in
  stdlib mux). Also register `GET /docs` → redirect to `/docs/` to avoid 404 on bare path.

- **Swagger UI alternatives:** `swaggest/swgui/v5emb` is the cleanest standalone UI package
  (swagger-ui v5.29.1, go:embed, no CDN). Use this if decoupled from swag. Avoid `v5cdn` — CDN
  dependency is inappropriate for distroless images.

- **oapi-codegen migration note:** The old import path `github.com/deepmap/oapi-codegen/v2` is
  frozen as of May 2024. All new work must use `github.com/oapi-codegen/oapi-codegen/v2`. The
  `std-http-server` generator target (added v2.2.0) enables stdlib net/http without chi/gin.

- **readyz implementation:** Add `atomic.Bool` to the `server` struct. `/readyz` returns 503 if
  `!ready.Load()`, 200 if ready. Call `s.ready.Store(true)` in `main.go` after `store.Load()`
  succeeds. Update Deployment `readinessProbe.httpGet.path` from `/healthz` to `/readyz`.

- **securityContext:** Add container-level `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`,
  `capabilities: drop: [ALL]`. Add pod-level `seccompProfile: {type: RuntimeDefault}`. No writable
  volume mounts needed for this API. Result satisfies Kubernetes Restricted PSS.

- **distroless/static includes `/tmp`** but the Go HTTP server does not write to it. No emptyDir
  mount required unless file upload routes are added in future.

- **NetworkPolicy enforcement on k3d/k3s:** Flannel does not enforce NetworkPolicy; k3s embeds a
  kube-router netpol controller that does. Verify enforcement after applying.

- **Cross-namespace selectors:** Use `kubernetes.io/metadata.name: <name>` label (auto-assigned by
  k8s to all namespaces since 1.21) — no manual namespace labeling required.

- **AND logic in NetworkPolicy from-rules:** Use a single `from` list entry with both
  `namespaceSelector` and `podSelector` for cross-namespace pod targeting. Two separate entries
  apply OR logic and over-permit.

- **DNS egress rule:** Always include UDP+TCP 53 to `k8s-app: kube-dns` in `kube-system` in any
  deny-all egress policy. Missing this breaks all hostname-based connections.
