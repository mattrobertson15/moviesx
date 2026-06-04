# Retro

> **Open this file at Session 1, not Session 10.** The strongest lessons happen mid-experiment; reconstructing them at the end is harder than appending one bullet per session as you go.
>
> At each session's close ritual, append one bullet under **Per-session notes** below. At `1.0.0`, write a short synthesis at the top from the bullets you already have.

## Synthesis (write at `1.0.0`)

- Stack chosen: Go (1.25), distroless container, k3d, kube-prometheus-stack, Swagger 2.0
- Total sessions to `1.0.0`: 6
- Total focused time: ~6.5 hours across sessions 1–6
- Where the methodology helped: Per-session framing kept scope tight; fit checks caught over-scoping before it became wasted work; RPI artifacts gave Session 6 a clear, ordered task list with no guesswork
- Where the methodology got in the way: The CLAUDE.md session map became slightly stale (version string, docker builder image) — needed a gap-close session to reconcile; could have been caught with a lighter review step
- What I'd do differently: Add a "dockerfile builder pin" note to the session close ritual checklist so go.mod bumps don't silently break the image build

## Per-session notes (append one bullet per session, in the close ritual)

- **Session 1 —** Useful template to maintain focus and context.Would spend more time understanding deciison made by AI agent.
- **Session 2 —** Went very smoothly.
- **Session 3 —** Went well but didn't finish before having to leave terminal, was able to wrap up the next day
- **Session 4 —** Smooth sailing
- **Session 5 —** Slightperformance tweaks next session
- **Session 6 —** CLI flags + bench overlay + govulncheck audit + dev-loop README + §14 all green → 1.0.0 tagged. Surprise: govulncheck dep bumped go.mod to 1.25 which broke the Dockerfile; always check go.mod drift vs. builder image after `go get`.
