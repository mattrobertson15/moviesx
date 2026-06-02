# Session 1 Review — 2026-06-02

**Reviewer:** Claude Code (automated)  
**Reviewed against:** `2026-06-02-stack-plan.md`, `2026-06-02-stack-changes.md`  
**Review method:** Source audit + live test (curl pod inside k3s cluster)

---

## Verdict: SHIP

All functional exit criteria pass. One cosmetic deviation (log format) and one clarification (go.sum) are noted but do not block shipping.

---

## Per-task results

### Task 1 — Initialize Go module: PASS (with clarification)

- `go.mod`: module `github.com/mbr/moviesx`, `go 1.22` ✓
- `go mod tidy` exits 0 ✓
- `go build ./...` passes ✓
- **Note:** `go.sum` is absent. This is correct — the module has no external dependencies (stdlib only), so `go mod tidy` produces no sum entries and no file is written. The plan listed `go.sum` as a file target, but that is a plan error, not an implementation error.

---

### Task 2 — Config package: PASS

- All three env vars read with correct defaults (`MOVIES_PORT=8080`, `MOVIES_LOG_LEVEL=info`, `MOVIES_DATA_DIR=/data`) ✓
- `getEnv` fallback pattern is correct ✓
- Table-driven tests cover: defaults, full override, partial override ✓
- Tests unset all vars before each case and clean up with `t.Cleanup` ✓
- `go test ./internal/config/...`: **PASS** ✓

---

### Task 3 — Server package: PASS

- `/version` handler: `fmt.Fprint(w, version)` — no trailing newline, `Content-Type: text/plain` ✓
- `/healthz` handler: `fmt.Fprint(w, "pass")` — no trailing newline, `Content-Type: text/plain` ✓
- Version injected via constructor, not hard-coded in handler ✓
- Both tests assert **exact** body strings (`"0.1.0"` and `"pass"`) using `!=` ✓
- Both tests assert status code and `Content-Type` ✓
- `go test ./internal/server/...`: **PASS** ✓
- Live `Content-Length` confirmed: `/healthz` = 4 bytes, `/version` = 5 bytes — zero extra whitespace ✓

---

### Task 4 — Wire up `main.go`: PASS (with deviation)

- Loads config via `config.Load()` ✓
- Constructs mux via `server.New(version)` ✓
- Calls `http.ListenAndServe(":"+cfg.Port, mux)` ✓
- `version` is a package-level `var` overridable by `-ldflags` in future builds ✓
- **Deviation — slog format:** The plan specified JSON output (`{"level":"info","msg":"listening","port":"8080"}`). The implementation uses `slog.Info(...)` with Go's default `TextHandler`, which emits text format:
  ```
  2026/06/02 16:39:30 INFO listening port=8080
  ```
  This is cosmetic — the plan's JSON format is a description of intent, not a verified exit criterion. Functional exit criteria (server starts, endpoints respond) all pass. Adding `slog.NewJSONHandler` is a one-liner fix if JSON logs are required.

---

### Task 5 — Dockerfile: PASS

- Stage 1: `golang:1.22-bookworm`, `CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w"` ✓
- Stage 2: `gcr.io/distroless/static-debian12:nonroot`, copies binary only ✓
- `USER nonroot:nonroot` ✓
- `EXPOSE 8080` ✓
- `ENTRYPOINT ["/moviesx"]` ✓
- `docker inspect moviesx:dev --format '{{.Config.User}}'`: `nonroot:nonroot` ✓
- Image size: 2,732,779 bytes (~2.6 MB, well under 20 MB limit) ✓
- `.dockerignore` excludes `.git`, `.copilot-tracking`, `*.md`, `**/*_test.go` ✓

---

### Task 6 — Kubernetes manifests: PASS

- `replicas: 1` ✓
- `imagePullPolicy: Never` ✓
- Env vars: `MOVIES_PORT=8080`, `MOVIES_LOG_LEVEL=info`, `MOVIES_DATA_DIR=/data` ✓
- `securityContext.runAsNonRoot: true`, `runAsUser: 65532` ✓ (live cluster confirmed: `{"runAsNonRoot":true,"runAsUser":65532}`)
- `containerPort: 8080` ✓
- Liveness probe: `GET /healthz`, `initialDelaySeconds: 2` ✓
- Readiness probe: `GET /healthz`, `initialDelaySeconds: 2` ✓
- Service: `type: ClusterIP`, `port: 80 → targetPort: 8080` ✓
- Label selectors consistent across Deployment (`matchLabels`, pod template) and Service ✓
- `kubectl apply --dry-run=client -f k8s/` clean per session changes doc ✓

---

### Task 7 — Load image into k3s and deploy: PASS

```
NAME                                   READY   STATUS    RESTARTS   AGE
moviesx-6b9988dd87-48q9m               1/1     Running   0          5m40s
```

Pod is `1/1 Running`, 0 restarts ✓

---

### Task 8 — End-to-end verification: PASS

Verified via ephemeral `curlimages/curl` pod inside the k3s cluster (host `kubectl` port-forward unavailable due to lb/port-3000 conflict — see session changes for context).

**Via pod IP (10.42.0.59:8080):**

```
=== GET /healthz ===
HTTP/1.1 200 OK
Content-Type: text/plain
Content-Length: 4

pass

=== GET /version ===
HTTP/1.1 200 OK
Content-Type: text/plain
Content-Length: 5

0.1.0
```

**Via service ClusterIP (10.43.80.30:80):**

```
=== GET /healthz ===
HTTP/1.1 200 OK
Content-Type: text/plain
Content-Length: 4

pass

=== GET /version ===
HTTP/1.1 200 OK
Content-Type: text/plain
Content-Length: 5

0.1.0
```

- `/version` body is exactly `0.1.0`, `Content-Length: 5` — no trailing whitespace or newline ✓
- `/healthz` body is exactly `pass`, `Content-Length: 4` — no trailing whitespace or newline ✓
- Both return HTTP 200 ✓
- Both return `Content-Type: text/plain` ✓

---

## Summary table

| # | Task | Result | Notes |
|---|---|---|---|
| 1 | Initialize Go module | **PASS** | `go.sum` absent (correct: no external deps) |
| 2 | Config package | **PASS** | — |
| 3 | Server package `/version` + `/healthz` | **PASS** | Content-Length confirms no trailing bytes |
| 4 | Wire up `main.go` | **PASS** | Log format is text, not JSON (see below) |
| 5 | Multi-stage Dockerfile | **PASS** | 2.6 MB, nonroot:nonroot confirmed |
| 6 | Kubernetes manifests | **PASS** | securityContext confirmed on live cluster |
| 7 | Load image + deploy | **PASS** | 1/1 Running, 0 restarts |
| 8 | E2E curl verification | **PASS** | Both endpoints verified inside cluster |

---

## Session 2 parking lot additions

| Item | Priority | Notes |
|---|---|---|
| Switch `slog` to `JSONHandler` | Low | Replace `slog.Info` with `slog.New(slog.NewJSONHandler(os.Stdout, nil))` in `main.go`; aligns log output with plan spec and structured log tooling expectations |
| Graceful shutdown (SIGTERM) | Medium | Was deferred from Session 1; needed before multi-replica or zero-downtime deploys |
| Restore host `kubectl` access | Low | k3d lb did not start (port 3000 conflict with Next.js dev server); document workaround or fix lb startup |
| `go.sum` note in plan | Clarification | Update plan template: note that stdlib-only modules produce no `go.sum` |

All other parking lot items carry over unchanged from `2026-06-02-stack-plan.md`.
