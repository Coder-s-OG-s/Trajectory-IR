# Trajectory IR: Master Specification and Contributor Guide

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/14075/badge)](https://www.bestpractices.dev/projects/14075)

| | |
|---|---|
| **Project** | Trajectory IR (package format: `.tir`) |
| **Purpose of this file** | The single canonical reference for the project: architecture, data model, protocols, tooling, and working rules. |
| **Audience** | Human contributors, reviewers, and **AI coding agents** (including Claude Code instances) implementing against this spec. |
| **Status** | `spec-v0.2-draft`. Supersedes the frozen `spec-v0.1` under the version bump process in §14. This revision narrows the project's scope from a standalone execution runtime to a semantics and portability layer over a pluggable, existing durable execution backend, see §3.1. |
| **Precedence** | This document is authoritative. If any prior document, chat message, or verbal agreement conflicts with this file, this file wins. |

**Project links:** [License (Apache-2.0)](LICENSE) · [Code of Conduct](CODE_OF_CONDUCT.md) · [Contributing (DCO)](CONTRIBUTING.md) · [AI Usage Policy](AI_POLICY.md) · [Security](SECURITY.md) · [Maintainers](MAINTAINERS.md) · [Governance](GOVERNANCE.md) · [Roadmap](docs/ROADMAP.md) · [Adopters](ADOPTERS.md) · [Third-party licenses](docs/THIRD_PARTY_LICENSES.md) · [Scope and non-goals](docs/SCOPE_AND_NON_GOALS.md) · [Quickstart (Go first)](go/QUICKSTART.md) · [CNCF Sandbox prep](docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md)

---

## 0. Purpose of this document

This file exists so that no two contributors, human or AI, build a different mental model of the same system. It replaces the earlier working documents (CAMI proposal, CLOOP proposal, standalone Trajectory IR notes, Fluid orientation notes) as the single source of truth. Those documents are kept in `docs/history/` for context only; **they are not normative**. If anything in this file appears to disagree with them, this file is correct.

Any AI coding agent picking up a task on this repository should treat this document as its full brief. For anything this document leaves undefined, it should not invent behavior from general knowledge of similar systems (LangGraph, CSI, event sourcing frameworks, etc.), it should implement exactly what is specified here, and flag a question (as an issue or a code comment tagged `SPEC-QUESTION`) rather than guessing. The one deliberate exception is the durable execution backend (§3.1, §12.0): that dependency's documented behavior, DBOS's, specifically, should be relied on directly rather than reimplemented, second guessed, or wrapped in redundant safety logic "just in case."

---

## 1. Executive summary

Trajectory IR is a typed, **portable** intermediate representation for agent execution. An agent run is a **trajectory**: an append only sequence of typed nodes. Before any world changing tool executes, the model's plan for that step is **sealed**.

Trajectory IR is **not** a durable execution engine and does not compete with one. Crash detection, retry, deterministic replay, and lease/heartbeat coordination are already solved, hardened problems, durable execution engines such as Temporal, Restate, and DBOS provide them today, and are already being adopted directly underneath agent frameworks. Reimplementing that machinery would be redundant engineering effort with no advantage over adopting it. Trajectory IR instead runs **on top of** a pluggable durable execution backend (§3.1) and owns the layer that backend does not provide: the semantics of what a sealed agent decision *means*, a tool effect classification tuned to LLM driven action, and, the part of this problem with no existing open answer, a **portable, hash verifiable, runtime independent export format** for a trajectory, so that what an agent did can be moved, audited, or handed to another agent outside the runtime that produced it.

The project addresses a gap identified in the CNCF TAG Infrastructure white paper on data storage for cloud native AI: agentic AI workloads require mutable short term state, append only event histories, artifact repositories, and long term memory consolidation, and no existing open standard defines the **portable semantics** of that closed loop. Fact memory products (Mem0, Zep), framework checkpoints (LangGraph, CrewAI, Google ADK), durable execution engines (Temporal, Restate, DBOS), and MCP's own tool safety annotations each cover part of the problem, see §3.1 for the specific boundary this project draws around each of them. None of them define a portable, effect safe, auditable, cross runtime unit of "what the agent actually did."

Trajectory IR is that unit, not another execution engine underneath it.

---

## 2. Problem statement (CNCF white paper context)

The white paper describes agentic AI as a state driven, closed loop, iterative architecture: session initialization with long term memory recall, an iterative loop of state mutation, event append, tool/agent calls, and artifact production, followed by delivery and memory consolidation. It identifies five storage requirements this loop creates, mutable short term state, append only event history, versioned artifacts, consolidated long term memory, and session bound input, and states plainly that no existing Kubernetes native or storage native primitive addresses them together.

Production systems built on top of this loop fail in the same recurring ways:

- A process dies mid tool call and a naive retry re executes a non idempotent side effect (a duplicate charge, a duplicate deployment, a duplicate email).
- "Resume" in practice re invokes the model, which may produce a different plan than the one that was interrupted, silently diverging from the original run.
- State and history are locked inside a single framework's checkpoint format, making the run impossible to move, audit, or replay elsewhere.
- Logs are unstructured free text; incident review and compliance audits become manual archaeology rather than a query.

The first two failure modes above, duplicate side effects on retry, and resume silently re inferring a different plan, are, generically, already solved by durable execution engines: that is their entire reason to exist. Trajectory IR does not re solve them; it relies on a durable execution backend (§3.1) for that guarantee and adds the LLM specific piece those engines don't have an opinion on, that the unit being sealed and replayed is *the model's plan for a step*, not just arbitrary workflow code. The third and fourth failure modes, history locked inside one framework's format, and logs that are archaeology rather than a queryable, verifiable record, are the part with no existing open answer, and are what Trajectory IR is actually for.

Trajectory IR exists to close *that* gap with a normative, implementable contract rather than another best effort logging convention, and to close it without re implementing the crash safety machinery that already exists elsewhere.

---

## 3. Architecture decision: how this project consolidates prior work

Three earlier proposals were explored in parallel before this consolidation: **CAMI** (a Kubernetes provisioning layer for agent memory, modeled on PVC/StorageClass), **CLOOP** (a multi plane session storage runtime), and **Trajectory IR** (a semantic layer for the closed loop itself). Running three separate frameworks under one small team was assessed as a credibility and maintenance risk with no corresponding benefit, since their concerns overlap substantially. The team has resolved this as follows, and this resolution is final for the current phase:

| Layer | Project | Status | Role |
|---|---|---|---|
| **Crash safe replay, retry, lease/heartbeat coordination** | A pluggable durable execution backend: **DBOS** (Python reference/default), **Temporal** (Go production backend), or **Restate** (optional additional adapter) | **External dependency, not reimplemented** | Owns crash detection, at most once side effect execution, deterministic replay, and lease/heartbeat timeout. Trajectory IR wraps its `DECISION`/`TOOL_CALL` steps as calls into this backend's own durable step primitive and trusts its replay guarantee. See §3.1. |
| **Semantics of the closed loop** | **Trajectory IR** | **Active, this is the project** | Defines nodes, seals, effect classes, the resume matrix (as *semantics*, backed by the chosen durable execution engine's mechanics), and the portable `.tir` package. Owns the event/data schema, node identity hashing, and export/import/graft. |
| **Session storage runtime** | CLOOP | **Folded into Trajectory IR** | CLOOP's mutable state, event, and artifact planes are implemented as Trajectory IR's storage backend. CLOOP does not exist as a separate schema or a separate public project. There is one schema, owned by Trajectory IR. |
| **Kubernetes provisioning (claims/classes for memory backends)** | CAMI | **Deferred, optional, non blocking** | A future, separate concern: how a Trajectory IR backend gets provisioned declaratively on Kubernetes. Not required for Trajectory IR to function correctly, and not part of the current implementation phase. May be revisited after Trajectory IR ships and proves adoption. |
| **Data locality / caching for large artifacts** | Fluid (CNCF incubating project) | **Optional accelerator, later phase** | Used only for large, remote, read heavy datasets under Kubernetes deployments. See §11.3 for the exact integration contract. Never a dependency for correctness. |

**Rule for all contributors and agents:** Do not create new top level frameworks, new schemas, or new package formats for concerns already covered above. Do not implement custom crash detection, retry, or lease/heartbeat logic under any row of this table, that responsibility belongs to the durable execution backend row, full stop. If a new concern appears that does not fit any row in this table, raise it as an issue before implementing.

---

## 3.1 Relationship to durable execution engines, agent frameworks, and MCP (prior art)

This section exists so that no contributor, human or AI, spends effort reimplementing something already solved elsewhere, and so that anyone evaluating this project for CNCF hosting can see exactly where the boundary is drawn. It is binding: treat every "does not attempt to duplicate" statement below as a hard scope limit, same weight as §16.

| Existing system | What it already solves | What Trajectory IR does **not** duplicate | What Trajectory IR adds on top |
|---|---|---|---|
| **Temporal, Restate, DBOS** (durable execution engines) | Append only execution history, deterministic replay, at most once/exactly once side effects, retry policies, lease/heartbeat coordination across workers | Trajectory IR does not build its own crash detection, retry, or lease/heartbeat protocol. It calls into one of these as a backend. | A node/seal schema specific to *agent* decisions (not generic workflow steps), effect classes tuned to LLM tool use, and, the part these engines don't provide, a portable, cross language, hash verifiable export of the resulting trajectory, independent of any one engine's internal storage format. |
| **LangGraph, CrewAI, Google ADK, OpenAI Agents SDK checkpointing** | Agent native state snapshots, thread based resume, human in the loop interrupts | Trajectory IR is not an agent orchestration framework and does not provide graph execution, routing, or tool calling loops. | A framework agnostic schema these checkpoint formats could export *into*, none of them currently define a format meant to leave their own runtime. |
| **MCP tool annotations** (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) | A basic, already standardized, fail closed vocabulary for tool safety | Trajectory IR does not invent a rival tool safety vocabulary from nothing. | A mapping from MCP's four hints into Trajectory IR's six effect classes (§7), adding the two distinctions MCP doesn't make, `AGENT_SPAWN` (a peer agent invocation) and `SENSITIVE` (a layered PII/secrets flag), and wiring that classification into the resume matrix (§8.2), which MCP has no opinion on. |
| **Mem0, Zep** (long term memory products) | Fact extraction and recall quality over consolidated memory | Trajectory IR does not compete on recall quality or memory consolidation. | An `LTM_QUERY`/`LTM_HIT`/`LTM_PROJECT` node shape any such system can write into, without adopting its storage engine. |

**The one sentence version, for anyone asking "why not just use Temporal/Restate/DBOS directly":** because none of them define a runtime independent, hash verifiable format for handing a finished or in flight agent trajectory to a different agent, a different tool, or an external auditor, they solve *durability inside one runtime*, not *portability across runtimes*. That is the whole project. Everything else in this document is scaffolding in service of that one property.

**Why Go uses Temporal instead of DBOS:** DBOS remains the Phase 1A reference/default for the Python SDK, an embedded library with the lowest operational ceremony, which matters for an early stage project. The Go port targets a different ecosystem, server and Kubernetes style deployments where a standalone durable execution cluster is the ecosystem norm, so `go/trajir/durable/temporal` (issues #16, #24) uses Temporal as its production backend behind the same adapter interface. Both choices honor the one rule that actually matters (§4 principle 6, §16): crash detection, retries, and lease/heartbeat coordination are never reimplemented, only consumed. Restate remains a welcome additional adapter for either language once its interface is stable, but is not required.

---

## 4. Design principles (non negotiable)

1. **A run is a trajectory, not a log.** Every meaningful event is a typed node with a stable identity, not a free text line.
2. **Decisions are sealed before side effects run.** The model may be non deterministic; the sealed plan is not. Resume replays the seal; it does not re infer. The replay guarantee itself, that a sealed step already executed is never re executed, is provided by the chosen durable execution backend (§3.1), not reimplemented here.
3. **Effect safety is explicit, not assumed.** Every tool is classified. Unknown or ambiguous tool metadata is treated as the most dangerous class by default (fail closed), consistent with and extending MCP's own fail closed tool annotation default (§3.1).
4. **Correctness never depends on a cache.** Any performance layer (including Fluid) may be cold, missing, or stale looking; the system must still be correct by falling back to the durable source of truth and verifying content hashes.
5. **Portability is a first class requirement.** A trajectory can be exported and understood outside the runtime that produced it, including outside the durable execution backend that ran it. This is the project's core differentiator (§3.1) and the one property no dependency below already provides.
6. **Thin core, no reimplementation of solved problems.** This project does not compete with Mem0/Zep on recall quality, with LangGraph/CrewAI on being "the" agent framework, with vector databases on search quality, or with Temporal/Restate/DBOS on durable execution. It defines a portable semantic contract those systems can sit underneath or feed into, it does not re build any of them.
7. **Spec before code.** No implementation detail overrides this document. Disagreements are resolved by editing this document first.

---

## 5. Scope for the current phase

### 5.0 Phase 1A (shipped library baseline)

Phase 1A delivered the dual language IR surface through `v0.1.0`: Python SDK with DBOS, Go port with Temporal as production durable backend, R01–R08, thin/fat `.tir`, CAS, and CI Phase A/B. That baseline stays supported.

### 5.1 Phase 1B (current development focus: Go primary)

Tracked under epic #113.

**In scope for Phase 1B:**

- Treat **Go as the primary SDK** for new features, storage drivers, and adoption demos (`go/trajir`, `go/examples`).
- Keep **Python as the reference and parity port** (DBOS local profile, existing client and conformance stay green).
- Go production durable backend remains **Temporal** (`go/trajir/durable/temporal`); LocalSQLite and Memory stay test fakes for coding.
- Go Postgres NodeLog and S3 compatible CAS drivers (parity with Python `drivers.postgres` / `drivers.s3`).
- Go first QUICKSTART and adoption host demo so a newcomer can succeed without installing Python.
- CONTRIBUTING process: implement new Phase 1B work in Go first, or dual language in one PR.

**Still out of scope (same as Phase 1A product non goals):** package signatures, Fluid/k8s-fluid productization, multi tenant SaaS control plane, custom crash engines, plan DAGs.

### 5.2 Phase 1A historical in scope list

The following records what Phase 1A required. It is not a license to prefer Python for new Phase 1B work.

**In scope (Phase 1A record):**

- A working adapter to **one** durable execution backend for the Python SDK, **DBOS** as the Phase 1A default (Python native, embeds as a library, SQLite/Postgres backed, no separate server to operate, the lowest ceremony fit for an early stage project), so that `DECISION`/`TOOL_CALL` steps get crash safe, at most once execution for free instead of via a hand rolled resume engine. This is a Python/DBOS conformance gate (R01/R02), not a restriction on a language appropriate backend in other ports: the Go port's `go/trajir/durable/temporal` (issues #16, #24) is a recognized production backend for Go under this same gate, not a second Phase 1A adapter (see §3.1, §12.0). A Restate adapter is a welcome additional implementation, for either language, once the interface is stable, but is not required for Phase 1A completion.
- Linear step execution (no parallel plan graph / DAG of tool calls with data dependencies).
- The full node kind set (§6.2) and node identity scheme (§6.3).
- All six effect classes and the MCP mapping (§7).
- Decision sealing and the resume matrix as *semantics* recorded via the backend adapter (§8), plus block and gate policy (§8.3), which is genuinely Trajectory IR's own logic, no existing engine gates on "this is a non idempotent tool interrupted mid call and needs human resolution before an LLM gets to touch it again."
- The `.tir` package format in `thin` and `fat` modes (§9). `redacted` mode is defined but not required to ship in Phase 1A.
- Conformance tests R01 and R02 as the hard gate for calling Phase 1A complete, now defined as backend adapter conformance tests (§10), since the guarantee they check is provided by the backend and merely verified through the adapter.
- Local deployment profile: DBOS backed step execution, SQLite for both DBOS's own state and the Trajectory IR node/seal log, filesystem backed artifact store, Python SDK.

**Out of scope for this phase (defined for later, not to be implemented now):**

- Any custom crash detection, retry, or lease/heartbeat implementation. If a task seems to require one, the adapter to the durable execution backend is missing a feature, extend the adapter, do not build a parallel mechanism.
- A plan level dependency graph (parallel or data dependent tool calls within a single sealed decision).
- Multi agent branch/merge UX beyond the basic graft by hash pattern.
- Any Kubernetes provisioning layer (CAMI style claims/classes).
- Fluid integration of any kind.
- A second durable execution backend adapter for the Python SDK (Restate or otherwise) before the DBOS adapter is conformant. This does not apply to the Go port's already shipped, language appropriate Temporal backend (§3.1, §12.0).
- Package signature **Python parity and conformance R09–R11** remain Future ([#179](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/179), epic [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)). The scheme itself is normative in §9.1. **Go** already ships optional `Sign` / `Verify` and Export `SignKey` under `go/trajir/tir`. Unsigned packages remain the default (`SIGNATURE` absent, `manifest.signature` null) unless a caller opts in.
- Multi region or multi writer high availability guarantees.
- Live full duplex media/streaming as a first class node type.

Any contributor or agent that finds themselves implementing something in the "out of scope" list should stop and raise an issue instead of proceeding.

---

## 6. Core data model

### 6.1 Trajectory object

```text
Trajectory {
  trajectory_id       # stable identifier for the run
  tenant_id            # multi-tenant scope
  schema_version
  parent_trajectory_id?  # set when this trajectory is a fork/child (agent-to-agent)
  status: OPEN | SEALED | ABORTED | ARCHIVED
  nodes[]              # ordered, append-only
  seals[]
  artifacts{}          # content-hash-keyed references
  metadata{}
}
```

### 6.2 Node kinds (v0.1, closed set, do not extend without a spec issue)

Every node carries the common fields: `kind`, `step_n`, `seq`, `ts`, `payload_hash`, `node_id`.

| Kind | Role |
|---|---|
| `INPUT` | User or system input bound to the run |
| `CONSTRAINT` | A hard rule that the context projector must not silently drop |
| `STATE_SET` | A mutable short term state write |
| `PROJECT_CONTEXT` | Record of what was assembled into the model's context: budget, pinned items, hashes |
| `THOUGHT` | Optional reasoning text; may be redacted on export |
| `DECISION` | The sealed plan for the current step: concrete tool calls with concrete arguments |
| `TOOL_CALL` / `TOOL_RESULT` | A tool invocation and its outcome |
| `ARTIFACT_PUT` / `ARTIFACT_REF` | A content addressed blob write, or a reference to one produced elsewhere |
| `LTM_QUERY` / `LTM_HIT` / `LTM_PROJECT` | Optional long term memory driver interaction |
| `COMMIT_STEP` | Marks a step as closed and durable |
| `ABORT` / `REDACTION` | Failure marker / privacy redaction marker |

### 6.3 Node identity

Node identity must be computable identically regardless of which language or implementation produced it. This is required for cross agent, cross language, and cross host consistency, and it is the most common place where independent implementations silently diverge, treat it as strict, not advisory.

1. Canonicalize the node payload using **RFC 8785 (JSON Canonicalization Scheme, JCS)**. Do not use an ad hoc "sorted keys" implementation, JCS pins number formatting, string escaping, and whitespace so that independent implementations in different languages produce byte identical output. Do not include wall clock `ts` in the hashed payload.
2. `payload_hash = sha256(canonicalized_payload)`
3. `node_id = sha256(tenant_id | trajectory_id | step_n_or_none | seq | kind | payload_hash)`

Every language implementation (Go and Python; both are first class for hashing) **must** use a conformant JCS library, not a hand rolled equivalent. If no conformant library exists for a target language, that is a blocking issue to raise before writing hashing code in that language.

### 6.4 Artifacts and content addressing

Artifact bytes are identified by `content_hash = sha256(bytes)` and stored by that hash, never by a mutable path alone. A logical path (e.g. `src/main.py`) may be recorded for human convenience, but the hash is the only thing the system treats as identity. See §11.2 for the required physical layout of the content addressed store.

---

## 7. Effect classes and tool safety

### 7.1 The six classes

| Class | Meaning | Resume default |
|---|---|---|
| `PURE` | No external world change | May recompute freely |
| `READ_ONLY` | Reads an external system | May re fetch or use cache per policy |
| `IDEMPOTENT_WRITE` | Write that is safe to retry with the same key | Replay with the same key |
| `NON_IDEMPOTENT_WRITE` | Deploy, payment, email, delete, or anything ambiguous | Never retried blindly, see block and gate, §8.3 |
| `AGENT_SPAWN` | Invokes a peer agent | Policy gated |
| `SENSITIVE` | Touches secrets or PII (a flag layered on another class) | Extra handling / redaction |

**Re fetch divergence rule (`READ_ONLY`):** a re fetch performed on resume may return a value different from the original run. This is written as a **new** `TOOL_RESULT` node; the original result, if one exists, is never overwritten. Steps that run after a resume see the new observation. This is intentional, honest divergence, it is recorded, not hidden, and it is not treated as an error.

### 7.2 MCP mapping and the fail closed rule

Tools registered from MCP (or any other source) must be mapped into this table before use. This mapping is deliberately built as a refinement of MCP's own tool annotations, not a replacement for them, the fail closed default below is the same posture the MCP specification itself already takes when annotations are absent.

| MCP annotation combination | Trajectory IR effect class |
|---|---|
| `readOnlyHint: true` | `READ_ONLY` (or `PURE` if additionally known to touch no external system) |
| `readOnlyHint: false`, `idempotentHint: true`, `destructiveHint: false` | `IDEMPOTENT_WRITE` |
| `readOnlyHint: false`, `destructiveHint: true`, or `idempotentHint: false` | `NON_IDEMPOTENT_WRITE` |
| Missing, absent, or ambiguous annotations | `NON_IDEMPOTENT_WRITE` (fail closed) |
| Any tool identified as invoking a peer agent | `AGENT_SPAWN`, regardless of its MCP annotations |
| Any tool flagged (by operator config, not by MCP) as touching secrets or PII | `SENSITIVE`, layered on top of whichever class above applies |

`AGENT_SPAWN` and `SENSITIVE` have no MCP equivalent, they are the two distinctions this project adds on top of MCP's four hints, driven by what the resume matrix (§8.2) and redaction (§9, R08) need that MCP has no reason to define. **If a tool's effect metadata is missing or ambiguous, it is classified as `NON_IDEMPOTENT_WRITE` by default.** This is a deliberate fail closed posture. An operator may explicitly override a tool to a safer class, but the default must never silently assume safety.

### 7.3 Idempotency keys

`idempotency_key = trajectory_id : step_n : stable_call_id`

`stable_call_id` is derived from the **sealed** `DECISION` node, never from a fresh model utterance produced after a crash. This is what prevents a re inference after resume from minting a new call id and causing a duplicate side effect.

---

## 8. Seals and resume protocol

### 8.0 What Trajectory IR owns here, and what it delegates

This section describes a **protocol semantics**, not an implementation Trajectory IR builds from scratch. Crash detection, the actual replay mechanism, and lease/heartbeat timing are provided by the durable execution backend adapter (§3.1, §12.0). Concretely: `DECISION` and `TOOL_CALL` are wrapped as the backend's own durable step primitive (e.g. a DBOS `@DBOS.step()` / workflow function). When the backend replays, its existing guarantee, an already completed step is returned from its recorded result, never re executed, is what makes "do not re invoke the model" true. Trajectory IR's own responsibility is: recording the seal in the node/seal log with the right hash and identity (§6.3), classifying the tool so the backend's retry policy is configured correctly (§7), and layering the block and gate policy below, which is agent specific and has no equivalent in a generic workflow engine.

### 8.1 Step lifecycle (happy path)

```text
1. PROJECT_CONTEXT     , assemble and record what went into the model's context
2. Model produces a plan (ephemeral, not yet durable)
3. DECISION is persisted with concrete tool arguments, then DECISION_SEAL is written
4. Tools execute only from the sealed plan, wrapped as durable-execution-backend steps; TOOL_RESULT nodes are written from the backend's step result
5. STATE_SET / ARTIFACT_PUT as needed
6. COMMIT_STEP is written, then STEP_SEAL
```

**The linear known args rule:** a sealed tool list is valid only if every argument in it is concrete and known at seal time. If tool B requires the result of tool A, B belongs in the **next** step (its own `PROJECT_CONTEXT` → inference → seal), not in the same `DECISION`. This is what keeps v0.1 implementable without a dependency graph, and it must not be worked around by embedding symbolic references into a single decision.

### 8.2 Resume matrix

This table describes what the trajectory log should show at each state; the "action" column is enforced jointly by the backend adapter (replay/retry mechanics) and Trajectory IR (semantic bookkeeping and gating).

| State found on disk | Action | Enforced by |
|---|---|---|
| No `DECISION` for step `n` | Start fresh: new `PROJECT_CONTEXT`, new inference is allowed | Trajectory IR |
| `DECISION` sealed, tool results incomplete | Replay the sealed tools. **Do not re invoke the model.** | Durable execution backend (replay guarantee) |
| All tool results present, no `COMMIT_STEP` | Finish any remaining state/artifact writes, then commit | Trajectory IR |
| Step already committed | Move on; never reopen a committed step | Trajectory IR |
| Crash mid tool on a `NON_IDEMPOTENT_WRITE` | Block and gate, see §8.3 | Trajectory IR (policy), backend (crash detection) |

### 8.3 Block and gate

For a crash mid execution of a non idempotent tool call, the default v0.1 behavior is:

- Mark the call `BLOCKED_NEEDS_GATE`.
- Do not auto retry.
- Do not re invoke the model.
- A human or an explicit policy decision resolves it: confirm the side effect succeeded, confirm it failed, run a compensation, or explicitly re run in a **new** step.

This is genuinely Trajectory IR's own contribution: a generic durable execution engine will happily retry or resume a step according to its configured policy, but it has no concept of "this step touched something irreversible from an *agent's* perspective and a human should look at it before an LLM gets another turn." That gating decision is what this project adds on top of the backend's mechanics.

Long running tools rely on the backend adapter's own heartbeat/lease primitive to detect an `IN_PROGRESS` call that has gone stale; Trajectory IR does not implement a second heartbeat mechanism alongside it. A call with no heartbeat eventually times out, at the backend's configured interval, into the gated state above rather than blocking resume forever.

This is the mechanism that makes "kill the process mid deploy, restart, and the world still only got one deploy" a true statement rather than a hopeful one, provided by the backend, gated by Trajectory IR's policy.

---

## 9. Package format: `.tir`

The extension `.tir` (not `.traj`, that name is already used by SWE-agent for a similar concept, and colliding with it causes unnecessary confusion and search engine overlap) is used for portable exports of a trajectory.

```text
manifest.json
nodes.ndjson            # one node per line, in seq order
seals.json
artifacts-manifest.json
artifacts/               # present only in fat packages
projector-policy.yaml    # optional; see §7 default policy note below
COMPAT.json
SIGNATURE                 # optional; scheme trajir-pkg-sig-v1 (§9.1). Absent = unsigned
```

**Modes:**

- **fat**, artifact bytes are embedded in the package.
- **thin**, only hashes and URIs are included; bytes are rehydrated from the object store on import.
- **redacted**, thoughts and flagged sensitive content are stripped before sharing. Not required to ship in Phase 1A.

Import verifies node ids and seals against their recorded hashes. Import does not automatically acquire a writer lease on the trajectory.

**Default projector policy (resolves the R04 dependency on an optional policy file):** when `projector-policy.yaml` is absent, the runtime applies a built in minimal policy: pinned items and all `CONSTRAINT` nodes are always included in context and are never silently dropped; if constraints alone exceed the token budget, the projector raises a hard `BUDGET_IMPOSSIBLE` error rather than trimming silently. A custom policy file must preserve this same invariant to be considered conformant.

### 9.1 Package signatures (`trajir-pkg-sig-v1`)

Package signatures authenticate **who produced a `.tir` export** and detect **tampering of package members** after export. They do **not** replace node identity hashing or decision seals (§6.3, §8). Content hashes prove payload consistency; signatures prove origin of the **bundle**.

This scheme is **normative**. **Go** implements optional `Sign` / `Verify` and Export-time signing via `SignKey` in `go/trajir/tir` (primary SDK). **Python** sign/verify parity and conformance R09–R11 remain Future ([#179](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/179), epic [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)). Producers **may** leave packages unsigned: no `SIGNATURE` zip member, and `manifest.json` **must** keep `"signature": null` (or omit the key) whether or not `SIGNATURE` is present. Implementations **must not** invent alternate schemes.

**Distinct from CI release signing:** cosign/SLSA for GitHub release tarballs and binaries (see `docs/CI_HARDENING.md`) is supply-chain signing of *release artifacts*. This section is **in-package** authenticity for `.tir` content exchanged between runtimes and partners.

#### Representation (file-only)

| Slot | Role |
|---|---|
| Zip member `SIGNATURE` | **Canonical** signature document (JSON). Absent ⇒ package is **unsigned**. |
| `manifest.json` field `signature` | **Always `null` (or omitted) in v1**, even when `SIGNATURE` is present. All signature material lives in the zip member so the signed payload never includes the signature itself (no circularity). |

Verification **always** uses the `SIGNATURE` member when present. Tools that only peek at the manifest treat `signature: null` as “see zip member if any,” not as “definitely unsigned.”

#### Signed payload (`trajir-pkg-payload-v1`)

The **signed payload** is a deterministic digest of package members **excluding** `SIGNATURE`.

1. List all zip entry names except the exact name `SIGNATURE` (case-sensitive).
2. Reject unsafe paths with the same rules as package Load (no absolute paths, no `..` segments).
3. Sort names by UTF-8 byte order.
4. For each name `N` with raw uncompressed bytes `B`:
   - Let `H = SHA-256(B)` as 32 raw bytes.
   - Append to a stream: `uint64_be(len(N)) || N || H`  
     (length-prefixed name + content hash; not full body bytes, so fat packages stay cheap to re-hash).
5. `payload_hash = SHA-256(stream)` (32 raw bytes; display as lowercase hex when needed).

**Fat mode:** `artifacts/cas/...` members are included like any other member.  
**Thin mode:** only non-artifact members (and any optional policy file if present).  
**Optional members** (e.g. `projector-policy.yaml`): included if present.  
**Redacted packages:** sign **after** redaction so the signature covers what is actually shared.

Changing any covered member byte changes `payload_hash`. Re-export requires re-sign.

#### Signature document (`SIGNATURE` JSON)

```json
{
  "scheme": "trajir-pkg-sig-v1",
  "payload_alg": "trajir-pkg-payload-v1",
  "payload_hash": "<64 lowercase hex chars of SHA-256>",
  "alg": "ed25519",
  "public_key": "<base64 of raw 32-byte Ed25519 public key>",
  "signature": "<base64 of raw 64-byte Ed25519 signature>",
  "signed_at": "<RFC3339 UTC>",
  "signer": {
    "id": "<optional free-form identity string>",
    "key_id": "<optional; recommended: first 16 hex chars of SHA-256(public_key raw bytes)>"
  },
  "extensions": {}
}
```

**Ed25519 signature input (domain-separated):**

```text
message = SHA-256( "trajir-pkg-sig-v1" || 0x00 || payload_hash_raw )
signature = Ed25519.Sign(private_key, message)
```

where `payload_hash_raw` is the 32-byte digest from `trajir-pkg-payload-v1` (not the hex string). Both Go and Python **must** produce identical `message` bytes for the same package.

Unknown `scheme` or `alg` values **must** fail verification (fail closed). A future optional mode may set `"alg": "sigstore-bundle"` with an embedded Sigstore/cosign bundle under a documented key (e.g. `bundle`); offline hosts may reject that mode. Base **Ed25519 requires no transparency log** (air-gapped friendly).

#### Verification order and policy

Load / import **must** apply checks in this order:

1. Path safety and size limits (zip bomb / traversal defenses).
2. Required members present; parse manifests.
3. Node id and seal hash verification (existing integrity).
4. **If** `SIGNATURE` is present: parse document, recompute `payload_hash`, verify Ed25519 over the domain-separated message, then apply trust policy.
5. **If** `SIGNATURE` is absent: package is unsigned; proceed unless policy requires a signature.

| Situation | Default Load | Strict (`require_signature=true`) |
|---|---|---|
| No `SIGNATURE` member | OK (unsigned) | **Fail** |
| `SIGNATURE` present and valid | OK; expose signature metadata to the caller | OK |
| `SIGNATURE` present but invalid / payload mismatch | **Fail** (tamper) | Fail |
| Unknown `scheme` / `alg` | Fail | Fail |
| Trust store configured and key not listed | **Fail** | Fail |
| No trust store configured | Accept any cryptographically valid Ed25519 key (dev / open exchange) | Same as default unless policy says otherwise |

A valid package signature is **never** a substitute for node/seal hash checks.

#### Trust model (no project CA)

v1 supports local trust only:

1. Raw Ed25519 public keys in a local trust file or environment (e.g. `TRAJIR_TRUSTED_KEYS`).
2. Optional `key_id` allowlist.
3. Later (optional): Sigstore identities when `sigstore-bundle` is implemented.

No project-hosted CA or multi-tenant key product is required for conformance to this scheme.

#### Implementation status

- **Go primary (shipped):** optional `Sign` / `Verify`, optional Export `SignKey` (full 64-byte Ed25519 private key; wrong length fails closed), golden payload vectors under `go/trajir/tir/testdata/sig_v1/`.
- **Python reference (Future):** byte-identical payload construction and verify parity ([#179](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/179)).
- **Conformance (proposed for Phase C):** R09 sign/verify + mutate fails; R10 unsigned still passes R05, strict mode rejects unsigned; R11 Go-signed verifies in Python and vice versa.
- **Security-review** required for further sign/verify changes. Private keys never in the repository; tests use ephemeral keys.

---

## 10. Conformance suite

These are runnable tests, not descriptions of intent. R01 and R02 are the hard gate for Phase 1A.

| ID | Check |
|---|---|
| **R01** | Seal a decision, kill the process, resume: the model is not invoked again; tools execute from the seal. |
| **R02** | A non idempotent tool (e.g. simulated deploy) is killed mid execution; on resume, the system does not double execute it, it follows the block and gate path. |
| R03 | A `PURE` tool may be recomputed freely on resume. Runnable: `conformance/r03_pure_recompute_test.py` (and Go `go test ./trajir/resume -run R03`). |
| R04 | A `CONSTRAINT` node is never silently dropped under budget pressure; the system either includes it or raises a hard error. Runnable: `conformance/r04_constraint_budget_test.py` (and Go `go test ./trajir/projector`). |
| R05 | Export and re import of a `.tir` package preserves all node hashes and seals exactly. Runnable: `conformance/r05_tir_roundtrip_test.py` (and Go `go test ./trajir/tir`). |
| R06 | A sandbox/what if branch rejects any real `NON_IDEMPOTENT_WRITE` execution. Runnable: `conformance/r06_sandbox_test.py`. |
| R07 | Grafting an artifact between agents transfers only the artifact reference, never private `THOUGHT` nodes. Runnable: `conformance/r07_graft_test.py`. |
| R08 | Redaction removes secrets/flagged content from what gets projected into context. Runnable: `conformance/r08_projection_redaction_test.py`. |
| R09 | (Proposed; Future) Sign a thin package; verify succeeds; mutate one byte of `nodes.ndjson`; verify fails. Blocked on §9.1 implementation ([#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)). |
| R10 | (Proposed; Future) Unsigned package still passes R05; strict `require_signature` rejects unsigned. |
| R11 | (Proposed; Future) Go-signed package verifies in Python and vice versa (cross-language payload parity). |

**Phase 1A completion bar:** DBOS backend adapter wired in, SQLite (or file backed) IR log, Python SDK, R01 and R02 passing against the adapter, and a recorded kill-mid-deploy demonstration. R03–R08 are tracked but not blocking for this milestone.

---

## 11. Storage, data locality, and Fluid integration

### 11.1 Layer responsibilities

| Concern | Owner |
|---|---|
| What a step means; node/seal semantics | Trajectory IR |
| Safe resume mechanics; no double side effects; retry; lease/heartbeat | Durable execution backend (DBOS default, Restate optional) via the backend adapter |
| Portable export/import/graft (`.tir`) | Trajectory IR, the one concern no dependency below provides |
| Durability of artifact bytes | Object store (S3 compatible; MinIO for local/dev) |
| IR metadata (nodes, seals) | SQLite (local) or PostgreSQL (server profile) |
| Fast access to large, remote, read heavy data on Kubernetes | Fluid (optional, later phase) |

Trajectory IR's correctness never depends on Fluid, or on any cache, being present or warm. This is a hard invariant, not a preference.

### 11.2 Content addressed object store layout

Artifact objects **must** be stored using a sharded key layout, not a flat prefix:

```text
s3://<bucket>/cas/<first-2-hex-chars>/<remaining-hex-chars>
# e.g. s3://trajir/cas/ab/cdef0123...
```

A flat `cas/<full-hash>` layout was considered and rejected: at scale it creates exactly the small object metadata listing pressure this project is trying to avoid, and it also degrades badly under Fluid's UFS metadata synchronization if Fluid is introduced later. Sharding is required from Phase 1A onward even though Fluid is not yet in use, so that the object store layout never has to change later.

### 11.3 Fluid integration contract (applies only when the `k8s-fluid` profile, §11.4, is active)

Fluid (a CNCF incubating Kubernetes native dataset orchestrator, providing pluggable cache runtimes such as Alluxio and JuiceFS behind a `Dataset` abstraction) is an **optional, read path accelerator** for large, remote, read heavy data. It is not a dependency for any correctness property in this specification. The following rules are binding whenever Fluid is enabled, and they resolve issues identified during architectural review:

1. **Fluid is read path only.** All durable writes go directly to the object store via the storage SDK. No implementation may route a durable write through a Fluid FUSE mount or rely on Fluid's write through/write back behavior. This avoids a class of cache consistency bugs and keeps the commit path simple: a step commits once the object store acknowledges the write and the IR log records it, a warm cache is never a precondition for commit.
2. **Fluid is reserved for large, shared, read mostly data**, specifically: input corpora (e.g. a repository snapshot or document set an agent reads from) and, optionally, materialized `.tir` fat packages for multi worker distribution. Fluid is **not** the default path for typical agent produced artifacts (source files, small JSON blobs, patches), which are usually small enough that a direct object store SDK call with a local disk cache is simpler and sufficient. A decision to mount a given dataset through Fluid must be justified by size and read fan out, not applied uniformly by default.
3. **Multi tenancy uses a shared `Dataset` with path based isolation**, not one `Dataset` (and therefore one Fluid `Runtime`, with its own cache worker pods) per tenant. A per tenant Dataset/Runtime pattern does not scale operationally once tenant count grows; tenant isolation is expressed as a path or prefix boundary within a shared Dataset, enforced by the same `tenant_id` scoping already used in the data model.
4. **Content addressed data does not go stale.** Because artifact identity is a content hash, a given hash's bytes never change once written. The only cache related failure mode is a **miss** (cold node, not yet synced), never staleness. Any implementation or documentation describing this as a "stale cache" risk is incorrect and should be corrected to "cache miss, resolved by falling back to the object store and verifying the hash."
5. **New objects are not assumed to be instantly visible through a Fluid mount.** Metadata synchronization between the object store and a Fluid `Dataset` is periodic or on demand, not guaranteed instant. Every code path that reads an artifact through a mount must have a fallback: if the file is not present or the hash does not verify, fetch directly from the object store and verify the hash before use. This fallback is the normal path, not an exceptional one.
6. **Resume correctness never depends on cache state.** If a pod resumes on a cold node with no warm Fluid cache, resume must still succeed, only more slowly, via direct object store reads with hash verification.

### 11.4 Deployment profiles

| Profile | When used | Fluid |
|---|---|---|
| `local` | Development, Phase 1A | Off. Filesystem backed content addressed directory. |
| `server-s3` | Single region API deployment | Off. Direct S3 compatible SDK calls. |
| `k8s-fluid` | Multi pod agent fleets on Kubernetes with large shared datasets | On, per §11.3. |

The Trajectory IR API surface is identical across all three profiles. Fluid is a deployment time profile choice, never a fork of the framework or a change to the SDK contract.

---

## 12. Technology stack and tooling

This section is binding for implementation choices. Do not introduce alternative tools for the same purpose without raising an issue.

### 12.0 Durable execution backend

- **DBOS**, the Phase 1A default for the **Python** SDK. Embeds as a Python library backed by SQLite (local) or PostgreSQL (server profiles); no separate server process to operate, which matters for an early stage project with limited operational capacity. `DECISION` and `TOOL_CALL` steps are wrapped as DBOS workflow/step functions so that crash safe, at most once execution and lease/heartbeat handling come from DBOS rather than a bespoke implementation.
- **Temporal**, the recognized production backend for the **Go** port (`go/trajir/durable/temporal`, issues #16, #24), behind the same internal adapter interface. Chosen for Go/Kubernetes style server deployments where a standalone durable execution cluster is the ecosystem norm; not a Phase 1A Python requirement and not a substitute for DBOS as the Python default (§3.1).
- **Restate**, an optional additional adapter behind the same internal interface, appropriate for deployments that want a standalone durable execution server rather than an embedded library. Not required for Phase 1A; must not be started for the Python SDK until the DBOS adapter is conformant (R01/R02 passing).
- The backend is selected by deployment profile (§11.4), never hardcoded into `pkg/runtime`, see the adapter interface in `drivers/durable-backend/` (§13).

### 12.1 Languages

- **Go**, **primary SDK and runtime language for Phase 1B** (epic #113). New features, drivers, and adoption demos should land in `go/trajir` first, or dual language in the same PR. The Go client (`OpenTrajectory`, `Project`, `SealDecision`, `ExecTool`, `CommitStep`, `Resume`, `RunStep`), IR stack, filesystem CAS, `.tir`, and Temporal production durable backend are the default product surface for newcomers and server style deployments.
- **Python**, **reference and parity port**. Phase 1A shipped Python first (DBOS local profile, client SDK, R01–R08 harness). Python remains fully supported and must stay green in CI, but it is no longer the default onboarding or feature lead language. Prefer Python only for Python specific fixes, DBOS adapter work, or explicit parity follow ups.

### 12.2 Containers (Docker)

- The runtime and demo tooling are packaged as Docker images for the `server-s3` and `k8s-fluid` profiles. The `local` profile does not require a container: prefer the Go module under `go/` for the fastest Phase 1B onboarding; the Python package remains available for the reference port.
- One `Dockerfile` per profile that needs containerization, kept minimal (no unnecessary system packages), with a pinned base image.

### 12.3 Kubernetes and Fluid

- Kubernetes is used only for the `k8s-fluid` profile and any future server deployment; it is not required to develop, test, or demo Phase 1A.
- When Fluid is used, its `Dataset` and `Runtime` custom resources are managed via Helm, following the constraints in §11.3.
- A Helm chart (`charts/trajectory-ir/`) packages the server component, its configuration, and, when the `k8s-fluid` profile is selected, the associated Fluid `Dataset` manifests.

### 12.4 CI/CD

- GitHub Actions runs on every pull request: linting, unit tests, and the conformance suite (R01–R08, with R01/R02 required to pass; later checks may be marked informational until they are fully implemented).
- Every commit must carry a DCO sign off (`Signed-off-by:` trailer); PRs without it are rejected by CI, not by manual review.
- No PR merges with a failing R01 or R02 once those tests exist in the repository.

### 12.5 Storage backends

- **SQLite**, the IR log backend for the `local` profile, and also DBOS's own workflow state store in that profile (one SQLite file, two schemas, no separate database to stand up).
- **PostgreSQL**, the IR log backend for the `server-s3` and `k8s-fluid` profiles, and DBOS's workflow state store in those profiles.
- **MinIO**, S3 compatible object storage for local and CI testing, so contributors do not need cloud credentials to run the full test suite.
- **Amazon S3 or equivalent**, production object storage.

### 12.6 Testing and quality gates

- **pytest** for the Python conformance suite and unit tests.
- A `make e2e` target that runs the kill-mid-deploy demonstration end to end (spawn agent, seal a decision, forcibly kill the process, resume, assert exactly one execution of the non idempotent tool).
- No feature is considered complete without an accompanying conformance test reference (even if the full R03–R08 suite is not yet green, new work must state which conformance ID it targets).

---

## 13. Repository structure

```text
trajectory-ir/
  README.md                     # Master Specification (this file)
  LICENSE                       # Apache-2.0
  CODE_OF_CONDUCT.md            # Code of Conduct
  CONTRIBUTING.md               # DCO sign-off requirement, PR process
  MAINTAINERS.md                # Active maintainers (Name / GitHub / Company)
  GOVERNANCE.md                 # Decision process and maintainership
  SECURITY.md                   # Vulnerability reporting
  ADOPTERS.md                   # Adopter list (fill as consent is given)
  docs/ROADMAP.md               # Public roadmap
  docs/SCOPE_AND_NON_GOALS.md   # Short scope summary
  spec/
    node-kinds.md
    effect-classes.md
    seals-and-resume.md
    tir-package.md
    conformance.md
    durable-backend-adapter.md    # the adapter interface every backend driver must satisfy
  pkg/                           # Python package: trajectory_ir
    runtime/                     # node append, hashing (JCS), seal/commit logic
    resume/                      # resume matrix SEMANTICS and block-and-gate policy only —
                                  # no lease/heartbeat implementation; calls the backend adapter
    effects/                     # effect class registry, MCP mapping
    package/                     # .tir export/import (fat/thin/redacted)
  client/
    python/
  drivers/
    durable-backend/
      dbos/                      # Phase 1A default adapter
      restate/                   # optional second adapter, later
    sqlite/
    postgres/
    s3/
    fluid/                       # only relevant when k8s-fluid profile is used
  conformance/
    r01_seal_resume_test.py
    r02_non_idempotent_test.py
    ...
  examples/
    kill-mid-deploy/
    graft-artifact/
  charts/
    trajectory-ir/                # Helm chart for server-s3 / k8s-fluid profiles
  docker/
    Dockerfile.server
    Dockerfile.k8s-fluid
  docs/
    history/                      # prior CAMI / CLOOP / Fluid-orientation docs, non-normative
  test/
    e2e/
```

Contributors and agents should place new code according to this layout. If a change does not fit an existing directory, propose the new location in the pull request description rather than inventing a parallel structure.

---

## 14. Contribution workflow (humans)

1. Read this document in full before opening a pull request.
2. Spec changes (anything in `spec/` or the normative sections of this file) require an issue describing the problem and the proposed change, discussed and approved before the corresponding `spec/*.md` file is edited.
3. Implementation pull requests reference the conformance ID(s) they target.
4. Every commit carries a DCO sign off.
5. `spec-v0.1` is an immutable tag. Once tagged, changes to the frozen behavior are versioned as `spec-v0.2` proposals, not silent edits to v0.1 files.

---

## 15. Working agreement for AI coding agents (ECC Integration)

This section exists specifically so that multiple AI coding agents (including different Claude Code sessions and Everything Claude Code [ECC] specialized subagents, working on different parts of the codebase at overlapping times) converge on the same implementation rather than producing conflicting or duplicated logic.

### 15.1 Specialized Agent Roles (ECC Suite)
When developing Trajectory IR using AI coding assistants, human maintainers and agents must operate under three mandatory procedural gate roles:
- **Planner Agent**: Must be invoked for any major architectural change, module creation, or ambiguous task to research and draft an `implementation_plan.md` before coding begins.
- **TDD-Guide Agent**: Enforces test-driven development for all logic in `pkg/` and `drivers/`. Modules must be built test-first with over 80% test coverage.
- **Security-Review Agent**: A mandatory procedural peer-review gate that must analyze and sign off on any proposed modifications to `pkg/effects/` (tool safety boundaries) or `pkg/resume/` (block-and-gate semantics) before a human maintainer merges the changes.

### 15.2 Hard Implementation Rules
Treat every numbered rule below as an immutable constraint, not a suggestion:

1. **This document is the spec.** Do not derive undefined behavior from general familiarity with similar systems (event sourcing frameworks, other IR designs, etc.). Where this document is silent on a design point, do not invent one, open a `SPEC-QUESTION` issue or comment and stop short of implementing the undefined behavior. This is distinct from rule 2 below: the durable execution backend is not "a similar system to avoid copying", it is a declared, intentional dependency, and its documented behavior should be relied on, not reimplemented.
2. **Never implement custom crash detection, retry, or lease/heartbeat logic.** This is the single most important rule in this section. That responsibility belongs entirely to the durable execution backend adapter (§3.1, §12.0). If a task seems to need it, the adapter interface is incomplete, extend `drivers/durable-backend/`, and if the chosen backend (DBOS) genuinely cannot provide what's needed, raise an issue before writing a parallel mechanism in `pkg/resume/`. A pull request that adds retry loops, polling, or timeout tracking outside the adapter is a defect, not a feature, regardless of how small it looks.
3. **Do not create parallel schemas.** The node kinds in §6.2 are the complete set for this phase. Do not add new node kinds, rename existing ones, or introduce an alternative representation (e.g. a "simplified" event format) for convenience.
4. **Hashing must be exact and cross language stable.** Any node identity or content hash computation must follow §6.3 precisely, using a conformant RFC 8785 JCS implementation. Do not substitute a custom "sorted keys" JSON serialization, this is the single most likely source of silent, hard to debug divergence between components written by different agents or sessions.
5. **Respect the linear known args rule (§8.1).** Do not implement, suggest, or work around it with symbolic argument references, speculative execution, or a hidden dependency graph. If a task seems to require it, stop and raise it as an issue rather than solving it locally.
6. **Effect classification defaults to fail closed (§7.2).** Never write code that defaults an unclassified or ambiguous tool to anything other than `NON_IDEMPOTENT_WRITE`.
7. **Never make correctness depend on a cache.** Any code path touching Fluid, or any other caching layer, must have a working fallback to the durable source of truth with hash verification, per §11.3. Code review (automated or human) should treat a cache dependent correctness path as a blocking defect.
8. **Stay inside declared scope (§5).** Do not implement anything listed under "out of scope for this phase," even if it seems like a natural extension, without an approved issue first. This includes package signatures, Kubernetes provisioning, plan DAGs, a second backend adapter, and Fluid integration outside of the `k8s-fluid` profile.
9. **One module, one clear owner in the code, even without a manually assigned task.** Before modifying a directory under `pkg/`, `drivers/`, or `conformance/`, check whether existing code in that directory implies an established pattern (naming, error handling, data structures) and follow it exactly rather than introducing a second convention. If two implementations of the same responsibility already exist, that is a defect to be reported, not a choice to pick one arbitrarily.
10. **No feature merges without its conformance test.** If an agent implements behavior covered by R01–R08, it must also implement or update the corresponding test, and that test must pass before the change is considered complete.
11. **When in doubt, prefer doing less over doing something not specified here.** An incomplete implementation that matches this document exactly is preferable to a complete one that deviates from it.

---

## 16. Non goals (complete list)

Trajectory IR deliberately does not attempt to:

- **Reimplement durable execution.** Crash safe replay, retries, and lease/heartbeat coordination are Temporal/Restate/DBOS territory (§3.1); this project consumes one of them and does not compete with any of them.
- Win on long term memory recall quality or benchmark scores (Mem0/Zep territory).
- Become "the" agent framework (LangGraph/CrewAI/ADK territory).
- Guarantee LLM determinism, non determinism is accepted and handled by sealing outputs, not by pretending inference is repeatable.
- Replace CSI/COSI or any block/file/object storage primitive.
- Provide Kubernetes native provisioning of memory backends in this phase (CAMI's former scope; deferred, not abandoned).
- Provide a general dependency graph plan executor in v0.1.
- Provide multi region active active guarantees.
- Treat live full duplex media/streaming as a first class concern in this phase.

---

## 17. Glossary

| Term | Meaning |
|---|---|
| Trajectory | An append only sequence of typed nodes representing one agent run |
| Node | A single typed record within a trajectory (see §6.2) |
| Seal | A durable, immutable marker over a decision or a step, establishing what may safely be replayed |
| Effect class | The declared safety category of a tool call (see §7.1) |
| Block and gate | The default policy for a crash mid non idempotent tool: halt and require explicit resolution rather than auto retry |
| `.tir` | The portable package format for exporting/importing a trajectory |
| JCS | JSON Canonicalization Scheme, RFC 8785, used to make hashing deterministic across languages |
| Durable execution backend | The external engine (DBOS default for Python, Temporal for Go, Restate optional for either) that provides crash safe replay, retries, and lease/heartbeat coordination underneath Trajectory IR (see §3.1) |
| DBOS | An open source, Postgres/SQLite backed durable execution library; the Phase 1A default backend for the Python SDK |
| Temporal | An open source durable execution engine with an event history / deterministic replay model; the recognized production backend for the Go port (§3.1, §12.0) |
| Restate | An open source durable execution engine with a journaled step model; an optional additional backend adapter for either language |
| CAMI | The deferred, optional Kubernetes provisioning layer for memory backends (not part of the current phase) |
| CLOOP | The earlier session storage proposal, now folded into Trajectory IR's storage layer rather than existing separately |
| Fluid | A CNCF incubating Kubernetes native dataset orchestrator, used only as an optional read path cache accelerator (§11.3) |
| CAS | Content addressed storage, objects identified and retrieved by the hash of their bytes |
| ECC | Everything Claude Code: An expert suite of specialized AI developer subagents (Planner, TDD-Guide, Security-Review) and canonical workflows mandated for AI-assisted development (§15) |

---

## 18. References

| Source | Relevance |
|---|---|
| CNCF TAG Infrastructure white paper, "Data Analytics and AI/ML Workloads" (July 2026) | Defines the agentic AI storage gap this project addresses |
| RFC 8785, JSON Canonicalization Scheme | Required for node identity hashing (§6.3) |
| Fluid (CNCF incubating project) documentation | Reference for Dataset/Runtime concepts used in §11.3 |
| DBOS documentation | Reference for the Phase 1A default durable execution backend (§3.1, §12.0) |
| Restate documentation | Reference for the optional second durable execution backend adapter (§3.1, §12.0) |
| Temporal documentation | Reference for the Go port's production durable execution backend (§3.1, §12.0); also prior art for the event history / deterministic replay model this project deliberately does not reimplement itself |
| Model Context Protocol tool annotation specification | Prior art and base vocabulary for effect classification (§3.1, §7) |
| `docs/history/` in this repository | Prior CAMI, CLOOP, and Fluid orientation documents, historical context only, not normative |

---

## 19. Document history

| Version | Date | Notes |
|---|---|---|
| 1.0 | 2026-07-21 | Initial master specification. Consolidates the CAMI/CLOOP/Trajectory IR architecture decision, the reviewed and corrected spec v0.1 (JCS hashing, READ_ONLY divergence rule, deferred signature field, default projector policy), the corrected Fluid integration contract (read path only, sharded CAS keys, shared Dataset multi tenancy, cache miss not staleness framing, metadata sync fallback), the full technology/tooling stack, repository layout, and the working agreement for AI coding agents. |
| 1.1 (`spec-v0.2-draft`) | 2026-07-23 | Repositions Trajectory IR from a standalone execution runtime to a semantics and portability layer over a pluggable durable execution backend (DBOS default, Restate optional), see new §3.1. Removes custom crash detection/retry/lease heartbeat from scope; reframes §8's resume protocol as semantics enforced jointly with the backend; adds explicit prior art boundaries against Temporal, Restate, DBOS, LangGraph family checkpointing, and MCP tool annotations, so the project's non redundant contribution, a portable, hash verifiable, runtime independent trajectory export, is stated once, explicitly, and can be evaluated on its own terms (including for CNCF Sandbox review, where "why not just use X" is always the first question asked). |
| 1.2 | 2026-08-07 | Reconciles §3, §3.1, §5, and §12.0 with the Go port's already shipped Temporal adapter (`go/trajir/durable/temporal`, issues #16, #24). Formally recognizes Temporal as the production durable execution backend for Go, alongside DBOS (Python Phase 1A default) and Restate (optional additional adapter for either language), and adds rationale in §3.1 for why Go diverges from the DBOS-first posture. Clarifies that §5's "one backend adapter" Phase 1A gate and its "no second adapter" out-of-scope item apply to the Python/DBOS conformance track only, and do not restrict the Go port's own language appropriate backend. Updates §17 glossary and §18 references accordingly. Resolves #67. |
| 1.3 | 2026-08-07 | Declares **Phase 1B**: Go is the primary SDK and default onboarding language; Python is the reference/parity port. Updates §5 (phase structure), §12.1 languages, and §12.2 local onboarding guidance. Tracks work under epic #113. Resolves #114. |
| 1.4 | 2026-08-13 | Defines package signature scheme **`trajir-pkg-sig-v1`** in new §9.1 (payload over zip members excluding `SIGNATURE`, domain-separated Ed25519, file-only document, unsigned default). Implementation remains Future ([#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)). Updates §5 out-of-scope wording and lists proposed conformance R09–R11. |

---

*This document is the canonical reference for Trajectory IR. Contributors, reviewers, and AI coding agents should treat it as authoritative and raise an issue rather than deviate from it.*
