# Retro

> **Open this file at Session 1, not Session 10.** The strongest lessons happen mid-experiment; reconstructing them at the end is harder than appending one bullet per session as you go.
>
> At each session's close ritual, append one bullet under **Per-session notes** below. At `1.0.0`, write a short synthesis at the top from the bullets you already have.

## Synthesis (write at `1.0.0`)

- Stack chosen: Go (1.25), distroless container, k3d, kube-prometheus-stack, Swagger 2.0
- Total sessions to `1.0.0`: 6
- Total focused time: ~7.5 hours across sessions 1–6
- Where the methodology helped: Per-session framing kept scope tight; fit checks caught over-scoping before it became wasted work; RPI artifacts gave Session 6 a clear, ordered task list with no guesswork
- Where the methodology got in the way: The CLAUDE.md session map became slightly stale (version string, docker builder image) — needed a gap-close session to reconcile; could have been caught with a lighter review step.
- What I'd do differently: Add a "dockerfile builder pin" note to the session close ritual checklist so go.mod bumps don't silently break the image build

## Per-session notes (append one bullet per session, in the close ritual)

- **Session 1 (97 min) —** Stack selected (Go, distroless, k3d) via the RPI research cycle; shipped `/version` and `/healthz` running in a k3d pod. The methodology template was itself being stress-tested here — it held scope tight. Lesson: the AI agent's design decisions were hard to inspect in real-time; reading the research/plan artifacts before approving would pay off in later sessions.
- **Session 2 (37 min) —** Fastest session of the project — full data layer, all `/api/*` endpoints, input validation (400s + 404s), and ≥80% coverage in 37 minutes. Fit check caught actorId/rating filters as risky scope; cut them cleanly before starting (they stayed in the parking lot through 1.0.0). The `Page[T]` generic and pre-sort-at-load design were locked in here.
- **Session 3 (130 min, split over two days) —** Most complex session: Prometheus metrics, structured JSON logging, Kustomize base/overlays/dev, ServiceMonitor, and Grafana dashboard. Discovered mid-session that data files had never been copied into the Docker image — a silent gap since Session 1. Had to leave mid-session; wrapped up the next morning. `go 1.23` bump required by the Prometheus dep.
- **Session 4 (76 min) —** Swagger/OpenAPI (swag v1, 2.0 spec), `/readyz` with the `server.SetReady()` pattern, full security hardening (readOnlyRootFilesystem, drop ALL caps, seccompProfile RuntimeDefault), and NetworkPolicy. Key trap: Kubernetes `runAsNonRoot` requires a numeric UID — the named user `nonroot` alone wasn't enough, had to set `runAsUser: 65532` explicitly.
- **Session 5 (50 min) —** Built the custom HTTP replay tool from scratch: YAML scenario format, template vars discovered at runtime, 8 assertion types, 54-scenario baseline suite covering all §6 endpoints and negative cases, plus benchmark mode with a 50-worker pool and p95 via sort+index. Fit check honest score was "No" — took on slightly more than fit; performance tuning landed in the Session 6 parking lot.
- **Session 6 (58 min) —** CLI flags + bench overlay + govulncheck audit + dev-loop README + §14 all green → 1.0.0 tagged. Surprise: govulncheck dep bumped go.mod to 1.25 which broke the Dockerfile; always check go.mod drift vs. builder image after `go get`.
- **Session 7 (60 min) —** Testing went smoothly other than a few issues loading grafana. Metrics all tested well and able to close out project cleanly.