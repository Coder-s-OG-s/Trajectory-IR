# Scope and non-goals

Short public summary for contributors and CNCF reviewers. Normative detail:
[docs/MASTER_SPECIFICATION.md](MASTER_SPECIFICATION.md) (root [README.md](../README.md)
is the landing page).

## In scope

- Typed trajectory nodes and stable identity hashing
- Decision sealing and resume **semantics** (not custom crash engines)
- Effect classes and MCP annotation mapping (fail closed)
- Named open-world primitives (`bash`, `python`, `sql`, browser, …) defaulting
  to `NON_IDEMPOTENT_WRITE`. This is a name denylist, not an AST of the command.
- Block-and-gate for non-idempotent tools (at-most-one automatic attempt)
- Seal-derived idempotency keys recorded on `TOOL_CALL` for the host to forward
- Portable `.tir` packages (thin / fat; redacted export heuristics)
- Dual SDK: **Go primary**, Python reference/parity
- Pluggable durable backends (Temporal for Go production; DBOS for Python reference)
- Conformance tests (R01–R11) and CAS / NodeLog storage drivers

## Out of scope (explicit)

| Topic | Status |
|-------|--------|
| Reimplement Temporal/Restate/DBOS crash/retry/leases | Never (adapter only) |
| Exactly-once remote writes without a server-side key | Never. Client gates cannot solve Two Generals. |
| Pause or re-validate the physical world on resume | Never automatic. Honest resume ≠ world-valid resume. |
| Parse `bash` / `python` / `sql` bodies for effect class | Never. No command AST. |
| Process / syscall sandbox, seccomp, gVisor | Never. R06 is an effect-class gate for demos and CI. |
| Agent graph orchestration / “be LangGraph” | Out |
| LTM recall quality product | Out (optional node shapes only) |
| Multi-tenant SaaS control plane | Future / not active |
| Fluid productization | Future / not active |
| Package signatures (`trajir-pkg-sig-v1` Ed25519) | Shipped (Go + Python, R09–R11) |
| Sigstore / `sigstore-bundle` for `.tir` | Future [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149) Phase D |

## Hard limits (read these before you pitch this)

1. **A seal freezes the plan, not the environment.** If the node you reserved is
   gone five minutes later, replaying the sealed `ReserveNode` is still the
   honest resume. The API should fail; the host then starts a **new** step.
   Re-checking the world is `READ_ONLY` observation, not re-inference.
2. **Block-and-gate does not know if the packet landed.** A claimed `TOOL_CALL`
   with no `TOOL_RESULT` means *do not retry and do not re-infer*. A human or
   host policy confirms success, confirms failure, compensates, or re-runs in a
   new step. Forward `idempotency_key` to the callee if you want exactly-once.
3. **NodeLog is the portable record. Temporal/DBOS is the crash engine.** Do
   not grow a second workflow engine inside this repo.

## One-line pitch

Portable, hash-verifiable runtime IR for agent execution trajectories
(seals, effects, `.tir`) on top of existing durable backends.

## Product separation

This project is **upstream open source** (Apache-2.0 libraries and format). It
is not a hosted multi-tenant commercial control plane. See
[docs/ROADMAP.md](ROADMAP.md).
