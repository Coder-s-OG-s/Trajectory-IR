# Scope and non-goals

Short public summary for contributors and CNCF reviewers. Normative detail:
root [README.md](../README.md).

## In scope

- Typed trajectory nodes and stable identity hashing
- Decision sealing and resume **semantics** (not custom crash engines)
- Effect classes and MCP annotation mapping (fail closed)
- Block-and-gate for non-idempotent tools
- Portable `.tir` packages (thin / fat; redacted export heuristics)
- Dual SDK: **Go primary**, Python reference/parity
- Pluggable durable backends (Temporal for Go production; DBOS for Python reference)
- Conformance tests (R01–R11) and CAS / NodeLog storage drivers

## Out of scope (explicit)

| Topic | Status |
|-------|--------|
| Reimplement Temporal/Restate/DBOS crash/retry/leases | Never (adapter only) |
| Agent graph orchestration / “be LangGraph” | Out |
| LTM recall quality product | Out (optional node shapes only) |
| Multi-tenant SaaS control plane | Future / not active |
| Fluid productization | Future / not active |
| Package signatures (`trajir-pkg-sig-v1` Ed25519) | Shipped (Go + Python, R09–R11) |
| Sigstore / `sigstore-bundle` for `.tir` | Future [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149) Phase D |

## One-line pitch

Portable, hash-verifiable intermediate representation for agent execution
trajectories (seals, effects, `.tir`) on top of existing durable backends.

## Product separation

This project is **upstream open source** (Apache-2.0 libraries and format). It
is not a hosted multi-tenant commercial control plane. See
[docs/ROADMAP.md](ROADMAP.md).
