# Console events and metrics

Stable vocabulary for the **Trajectory console** (milestone
[Trajectory console](https://github.com/Coder-s-OG-s/Trajectory-IR/milestone/11),
epic [#391](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/391)).

Go and Python emitters, the ingest reader, and UI panels must agree on these
names. If a field is not listed here, do not invent it in the UI.

Schema version for this document: **`console-events-v1`**.

## Why this exists

Hosts already seal decisions, project context, redact, and move `.tir`
packages. The console needs a **shared event envelope** so:

1. Live SDK hooks and offline `.tir` analysis produce the same kinds
2. Seal / transfer / economy panels map to named metrics without ad-hoc math
3. Dual-language emitters stay byte-compatible on kinds and required fields

## Non-goals

- Exact LLM **provider billing** parity (OpenAI/Anthropic invoices, cached-token
  discounts, etc.). v1 uses **estimated** figures only.
- Replacing NodeLog / seals / `.tir` as sources of truth. Events are
  **observations**, not a second durable log.
- Multi-tenant SaaS identity, billing, or cloud control planes.
- Real-time streaming product requirements beyond append-only NDJSON / HTTP POST.

---

## 1. Event envelope

Every event is one JSON object (one NDJSON line on disk).

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `schema_version` | string | yes | Always `console-events-v1` for this doc |
| `id` | string | yes | Opaque unique id (UUIDv4 recommended) |
| `ts` | string | yes | RFC 3339 UTC when the observation was recorded |
| `kind` | string | yes | One of the kinds in §2 |
| `source` | string | yes | `go` \| `python` \| `cli` \| `derived` |
| `trajectory_id` | string | yes | Same id used by NodeLog / `.tir` |
| `tenant_id` | string | no | When the host is tenant-scoped |
| `runtime` | string | no | Free-form host label (`trajir-mcp`, `adoption_host`, …) |
| `payload` | object | yes | Kind-specific body (§2). May be `{}` |

Rules:

- Unknown top-level fields: **ignore** on read (forward compatible).
- Unknown `kind`: store raw, do not crash the reader; UI hides until known.
- `source: derived` is reserved for offline analysis of an existing `.tir` or
  NodeLog dump (§5). Live hooks use `go` / `python` / `cli`.

---

## 2. Event kinds (v1)

### 2.1 Lifecycle / nodes

#### `node.appended`

Emitted when a node is successfully appended to the durable log.

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `node_id` | string | yes | Content-addressed id |
| `kind` | string | yes | Node kind (`DECISION`, `TOOL_CALL`, …) |
| `seq` | integer | no | Log sequence when known |
| `step_n` | integer | no | Step number when known |
| `content_hash` | string | no | Hex SHA-256 of canonical payload when available |

### 2.2 Seals

In the current runtime a **decision seal** is a durable `DECISION` node
(`SealDecision` in Go / equivalent Python host path). Console kinds stay
explicit so the UI does not special-case node kinds alone.

#### `seal.created`

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `node_id` | string | yes | Id of the `DECISION` node |
| `step_n` | integer | yes | Step that was sealed |
| `content_hash` | string | yes | Hash that freeze the plan |
| `tool_names` | array[string] | no | Planned tool names when present in the plan |

#### `seal.verified`

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `node_id` | string | yes | |
| `ok` | boolean | yes | |
| `content_hash` | string | yes | Hash that was checked |
| `error` | string | no | Reason when `ok` is false |

Emit on import/load verification and on any explicit verify path. Do not emit a
successful `seal.verified` for every append — that is `seal.created`.

### 2.3 Context projection

Native projector metric today is **`rfc8785_bytes`** (JCS canonical byte length
of `{kind, payload}` per included node). See
`pkg/trajectory_ir/runtime/projector.py` / Go `trajir/projector`. Token figures
in the console are **derived estimates** (§3), not the projector budget unit.

#### `context.projected`

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `budget` | integer | yes | Budget in `metric` units |
| `metric` | string | yes | Usually `rfc8785_bytes` |
| `size_units` | integer | yes | Total size of included set |
| `included_ids` | array[string] | yes | |
| `dropped_ids` | array[string] | yes | |
| `raw_size_units` | integer | no | Size if all candidate nodes were kept |
| `raw_char_len` | integer | no | UTF-8 length of serialized pre-projection context (for estimates) |
| `projected_char_len` | integer | no | UTF-8 length of serialized projected context |

When `raw_size_units` is omitted, economy views that need a “before” figure
must fall back to offline derivation (§5) or show “unknown”.

### 2.4 Redaction

#### `redaction.applied`

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `mode` | string | yes | `projection` \| `export` |
| `thought_collapses` | integer | yes | THOUGHT (or nested thought) payloads collapsed |
| `secret_field_hits` | integer | yes | Secret-shaped keys/values scrubbed |
| `char_len_before` | integer | no | |
| `char_len_after` | integer | no | |

Heuristic redaction is not a secret scanner (`SECURITY.md`). Counts are
observational.

### 2.5 Package transfer

#### `export.started`

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `path` | string | no | Destination path if known |
| `mode` | string | yes | `thin` \| `fat` |
| `redacted` | boolean | yes | |

#### `export.completed`

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `path` | string | yes | |
| `mode` | string | yes | Effective mode (`redacted` fat→thin still reports final mode) |
| `redacted` | boolean | yes | |
| `bytes` | integer | yes | Package file size |
| `member_count` | integer | yes | Zip members |
| `node_count` | integer | yes | |
| `ok` | boolean | yes | |
| `error` | string | no | |

#### `import.completed`

| Payload field | Type | Required | Notes |
|---------------|------|----------|-------|
| `path` | string | yes | Source package |
| `mode` | string | yes | From manifest |
| `redacted` | boolean | no | From manifest when present |
| `bytes` | integer | yes | |
| `member_count` | integer | yes | |
| `node_count` | integer | yes | |
| `verify_ok` | boolean | yes | Hash/seal verification result |
| `error` | string | no | |

---

## 3. Token / context economy metrics

Computed by the **reader** (or offline deriver) from events + optional
char-length fields. UI must label estimates as estimates.

### 3.1 Estimator (v1)

```
estimated_tokens(char_len) = ceil(char_len / 4)
```

- Name in APIs/UI: **`estimated_tokens`**
- Do **not** call this “billed tokens”, “invoice tokens”, or “$ saved”
- Optional later: pluggable tokenizer id (`cl100k_base`, …). Until then the
  formula above is the only console-standard estimator. Fixture notes that use
  tiktoken (e.g. maintainer metrics snapshots) are **out of band** and must not
  redefine this field silently.

### 3.2 Aggregates

| Metric | Definition |
|--------|------------|
| `raw_estimated_tokens` | `estimated_tokens(raw_char_len)` from the latest usable `context.projected` (or derived) |
| `projected_estimated_tokens` | `estimated_tokens(projected_char_len)` |
| `tokens_avoided_estimated` | `max(0, raw_estimated_tokens - projected_estimated_tokens)` |
| `projection_size_units` | `size_units` from `context.projected` |
| `projection_budget` | `budget` from `context.projected` |
| `nodes_dropped` | `len(dropped_ids)` |
| `redaction_collapses` | sum of `thought_collapses + secret_field_hits` over `redaction.applied` |

If `raw_char_len` is missing, set `tokens_avoided_estimated` to `null` and show
size-unit savings (`raw_size_units - size_units`) when those fields exist.

---

## 4. Transfer metrics

| Metric | Source |
|--------|--------|
| `package_mode` | `export.completed.mode` / `import.completed.mode` |
| `package_redacted` | boolean on export/import |
| `package_bytes` | `bytes` |
| `package_members` | `member_count` |
| `package_nodes` | `node_count` |
| `transfer_verify_ok` | `import.completed.verify_ok` (and seal.verified rollup) |

A **handoff** in the Transfers view is an `export.completed` followed by an
`import.completed` that shares `trajectory_id` (and optionally matching
content hashes). The UI may draw them as one edge; the event log stays flat.

---

## 5. Offline derivation from `.tir`

When a host never emitted live events, the console may set `source: derived`
and synthesize a minimal stream:

1. Load/verify the package (existing `Load` / `import_tir` rules)
2. Emit `import.completed` (or a synthetic `export.completed` if treating the
   file as an artifact under study) with manifest mode/redacted/sizes
3. For each node, emit `node.appended`
4. For each `DECISION`, emit `seal.created`; after successful hash checks emit
   `seal.verified` with `ok: true` (or `ok: false` on failure — and stop)
5. Optionally rebuild projection/redaction **estimates** only if the deriver
   re-runs projector/redact over loaded nodes; do not invent `included_ids`
   without running that code path

Derived events must not claim stronger provenance than the package itself.

---

## 6. UI panel mapping

| Console panel (issues) | Primary kinds | Primary metrics |
|------------------------|---------------|-----------------|
| Overview (#394) | mix / latest of all | node count, last `ts`, verify rollup |
| Seals (#395) | `seal.created`, `seal.verified` | seal count, failed verifies |
| Context economy (#396) | `context.projected`, `redaction.applied` | §3 aggregates |
| Transfers (#397) | `export.*`, `import.completed` | §4 aggregates |

If a panel needs a number that is not in §3–§4, extend **this document** first.

---

## 7. Compatibility and versioning

- Bump `schema_version` only on breaking envelope/kind changes
- Additive optional payload fields do not require a version bump
- Readers accept older events with the same `console-events-v1` when new
  optional fields are absent

## References

- Projector metric: `rfc8785_bytes` — `pkg/trajectory_ir/runtime/projector.py`
- Redaction: `pkg/trajectory_ir/runtime/redact.py`, Go `trajir/redact`
- Packages: `pkg/trajectory_ir/package/tir.py`, Go `trajir/tir`
- Seals: Go `trajir/client.SealDecision` (DECISION node)
- Epic: [#391](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/391)
- Spec issue: [#392](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/392)
