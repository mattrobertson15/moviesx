# Stack Research: Read-Only HTTP API for k3s

**Date:** 2026-06-02  
**Question:** Which server-side stack should back a small read-only HTTP API that must run in a minimal container, expose Prometheus metrics, serve OpenAPI 3 + Swagger UI, and deploy to k3s?

**Candidates evaluated:** Go, Python/FastAPI, TypeScript/Fastify, Rust/axum

---

## Scoring Matrix

| Criterion | Go | Python/FastAPI | TypeScript/Fastify | Rust/axum |
|---|---|---|---|---|
| Final image size | ★★★★★ | ★★ | ★★ | ★★★★★ |
| Prometheus client maturity | ★★★★★ | ★★★★ | ★★★★ | ★★★ |
| OpenAPI/Swagger ergonomics | ★★★ | ★★★★★ | ★★★★ | ★★ |
| Cold-start on k3s | ★★★★★ | ★★★ | ★★★★ | ★★★★★ |
| **Total** | **18/20** | **15/20** | **14/20** | **15/20** |

---

## Go

**Image size:** Multi-stage build with `CGO_ENABLED=0 GOOS=linux go build` produces a fully static binary. Final layer on `gcr.io/distroless/static-debian12` or `scratch` lands at **5–20 MB** depending on dependencies. No runtime required in the image.

**Prometheus:** `github.com/prometheus/client_golang` is maintained by the Prometheus project itself — the reference implementation. `promhttp.Handler()` exposes `/metrics` in four lines. Push gateway, histograms, summary, exemplars, and OpenMetrics format are all first-class. Widely deployed in production k8s tooling (kube-state-metrics, node-exporter, etc.).

**OpenAPI/Swagger:** Two credible paths:
- `swaggo/swag` — annotation-driven, generates spec at build time via `swag init`. Verbose but well-understood. Swagger UI served via `swaggo/http-swagger`.
- `deepmap/oapi-codegen` — spec-first: write `openapi.yaml`, generate server stubs. Better discipline for read-only APIs.
Neither is as automatic as FastAPI, but both are production-grade and well-established. Ergonomics are acceptable, not exceptional.

**Cold-start:** Essentially zero. Compiled binary with no JIT, no interpreter, no module graph to walk. Pod-ready in under 100 ms on k3s; often under 50 ms. Liveness probes pass immediately.

---

## Python / FastAPI

**Image size:** Even with `python:3.12-slim` or `python:3.12-alpine`, you carry the CPython interpreter (~50 MB), pip-installed packages, and `.pyc` files. Realistic final image: **120–300 MB**. Distroless is not a practical target; you need the Python runtime. Multi-stage helps trim build tools but not the runtime footprint.

**Prometheus:** `prometheus_client` (official Python client, maintained by the Prometheus project) is mature and straightforward. ASGI middleware integration with Uvicorn works well; async-safe with care under multiple Gunicorn workers (`multiprocess_mode`).

**OpenAPI/Swagger:** FastAPI auto-generates a complete, correct OpenAPI 3 spec from Python type annotations — **best-in-class ergonomics across all four options**. Swagger UI (`/docs`) and ReDoc (`/redoc`) are served out of the box with zero configuration. Pydantic models become both runtime validation and schema simultaneously. No annotation comments, no code generation step, no drift risk.

**Cold-start:** CPython startup + importing FastAPI, Pydantic, Uvicorn, and application modules takes **1–3 seconds** on a cold pod — a real operational cost on k3s where pods restart during rolling deploys or scale-to-zero.

---

## TypeScript / Fastify

**Image size:** Node.js 20 Alpine base (~50 MB) plus `node_modules` for a minimal Fastify app with swagger plugins can reach 80–150 MB. Careful `--omit=dev` and dependency hygiene gets a realistic final image to **130–220 MB**. Distroless Node images exist but are uncommon.

**Prometheus:** `prom-client` is battle-tested. Supports all metric types, push gateway, default process/GC metrics. `fastify-metrics` wraps it cleanly. Mature, though not maintained by the Prometheus project directly.

**OpenAPI/Swagger:** `@fastify/swagger` + `@fastify/swagger-ui` integrates well: route schemas defined in JSON Schema are auto-promoted to OpenAPI 3. TypeBox (`@sinclair/typebox`) tightens this with type-safe schema objects that serve as both TypeScript types and JSON Schema — genuinely pleasant DX. Requires an explicit `schema:` block on every route.

**Cold-start:** Node.js module loading takes **500 ms–1.5 s** on k3s cold start. Better than Python, worse than Go/Rust.

---

## Rust / axum

**Image size:** Multi-stage build (`rust:bookworm` builder → `gcr.io/distroless/cc-debian12` with `musl` target) yields a static binary of **5–15 MB**. Matches Go. `cargo-zigbuild` or `cross` simplifies cross-compilation in CI.

**Prometheus:** Two paths:
- `prometheus` crate (TiKV fork) — direct, functional, but maintenance and upstream feature alignment lags `client_golang`.
- `metrics` + `metrics-exporter-prometheus` — provider-agnostic abstraction layer, cleaner architecture but has had breaking API changes across versions.
Neither is as settled as the Go or Python clients. Works in production, but further from the Prometheus project's own code.

**OpenAPI/Swagger:** `utoipa` provides derive macros (`#[utoipa::path(...)]`) to annotate handlers. Works, but macros are verbose, error messages on misuse are poor, and keeping derives in sync with Axum extractor types requires manual discipline. `aide` is a more integrated alternative but less mature. Of the four options, **weakest OpenAPI ergonomics** with the highest spec-drift risk.

**Cold-start:** Comparable to Go — under 100 ms.

---

## Recommendation: Go

Go wins on the combination that matters most for this workload:

1. **Prometheus client is the reference implementation.** `client_golang` is maintained by the Prometheus project. Metric types, naming conventions, cardinality handling, and exemplar support are always current. Eliminates an entire category of library risk.

2. **Distroless image at 5–20 MB** is achievable with a single `CGO_ENABLED=0` flag. No runtime to carry, no Alpine libc shim.

3. **Cold-start under 100 ms.** On k3s, where pods restart during rolling deploys or node rebalancing, immediate readiness is operationally convenient.

4. **OpenAPI tooling is good enough.** `oapi-codegen` (spec-first) or `swag` (annotation-first) are both well-maintained. For a bounded read-only API, the verbosity is manageable.

5. **Ecosystem alignment.** Dockerfile patterns, Helm chart examples, and distroless guidance are most abundant for Go in the k3s/k8s ecosystem.

---

## Explicit Non-Choices

### Not Python/FastAPI
FastAPI's OpenAPI DX is the best of any framework evaluated — schema generation from type annotations with zero configuration is genuinely excellent. Ruled out because the 120–300 MB image size and 1–3 s cold-start are incompatible with the distroless/minimal-container requirement. If image size and cold-start were not constraints, FastAPI would be the first choice on ergonomics alone.

### Not TypeScript/Fastify
`prom-client` and `@fastify/swagger` are both solid. The TypeBox integration for type-safe JSON Schema is a genuine DX win. Ruled out because the Node.js runtime makes sub-50 MB images impractical, and cold-start latency is worse than Go without a compensating advantage in this specific feature set.

### Not Rust/axum
Image size and cold-start match Go exactly. But Rust loses on two of the three library-quality criteria: OpenAPI derive macros (`utoipa`) are the weakest ergonomics of the four options, and the Prometheus crate ecosystem is more fragmented and less aligned with the upstream project than `client_golang`. For a small API, the productivity cost of Rust compile times and OpenAPI macro verbosity is not justified when Go achieves identical binary size and startup time with better library support.

---

## Summary

| | Go | Python | TypeScript | Rust |
|---|---|---|---|---|
| Final image | ~10 MB distroless | ~200 MB slim | ~180 MB Alpine | ~10 MB distroless |
| Prometheus client | Official/reference | Official | Community (mature) | Community (fragmented) |
| OpenAPI auto-spec | Manual annotations | Auto from types ★ | Schema blocks | Derive macros (brittle) |
| Cold-start | <100 ms | 1–3 s | 500 ms–1.5 s | <100 ms |
| **Chosen?** | **Yes** | No | No | No |
