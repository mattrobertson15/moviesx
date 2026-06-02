---
name: project-movies-experiment
description: Movies API experiment — session-based RPI build of a Kubernetes-native HTTP API. Tracks methodology, scope, and session plan.
metadata:
  type: project
---

This is the `movies` experiment: build a Kubernetes-native HTTP API (movies catalog) following the sessions + RPI methodology from context-first/core and Microsoft HVE Core.

**Goal:** All §14 acceptance criteria green on a fresh local k3s cluster, tagged `1.0.0`.

**Data files already in repo:** `src/data/movies.json`, `src/data/actors.json`, `src/data/ratings.json`
- movies: id (tt########), title, year, runtime, genres[], roles[] (actorId, name, category, characters[])
- actors: id (nm########), name, birthYear, deathYear, profession[], movies[] (movieId, title)  
- ratings: movieId, rating (float), votes (int)

**Stack:** Not yet chosen. Decision happens in Session 1 Research phase.

**Session plan (6 sessions):**
1. 0.1.0 — Stack choice + skeleton + /version + /healthz on k3s
2. 0.2.0 — Data layer + all /api/* endpoints + validation + unit/integration tests ≥80%
3. 0.3.0 — Prometheus metrics + structured logging + Grafana dashboard + Kustomize base+overlay
4. 0.4.0 — OpenAPI/Swagger + /readyz + security hardening (NetworkPolicy, securityContext)
5. 0.5.0 — Custom HTTP replay tool (§10.3 baseline + §10.4 benchmark — no off-the-shelf tools)
6. 1.0.0 — Inner-loop README docs + §14 gap close + final acceptance pass

**Why:** Testing whether sessions+RPI closes the "26-week team discovery gap" down to a small number of focused solo sessions.

**How to apply:** Each session gets its own session-log.md entry framed before starting. RPI artifacts go in `.copilot-tracking/YYYY-MM-DD-<topic>-{research,plan,changes,review}.md`. Never skip the fit check.
