# node-kinds.md — Trajectory IR v0.1

**Status:** Phase 0 freeze candidate  
**Version:** 0.1  
**Package unit:** `.tir` (see [traj-package.md](./traj-package.md); not `.traj` — collision with SWE-agent)

---

## 1. Purpose

Define the **v0.1 node set** and **node_id hash recipe** for Trajectory IR.  
This schema is the **source of truth** for event/step meaning. Stores (SQLite, Postgres, later CLOOP-style planes) **persist** these nodes; they do not invent a parallel event model.

---

## 2. Trajectory container (minimal)

```text
Trajectory {
  trajectory_id: string          # ULID or UUIDv7
  schema_version: "0.1"
  tenant_id: string
  principal: { subject_id?, agent_id? }
  parent_trajectory_id?: string  # fork / child
  status: OPEN | SEALED | ABORTED | ARCHIVED
  created_at: ISO-8601
  metadata: object               # harness, model defaults, etc.
}
```

---

## 3. v0.1 node set (normative)

| Kind | Required fields (beyond common) | Notes |
|------|----------------------------------|--------|
| `INPUT` | `payload` (text and/or refs) | User/system input |
| `CONSTRAINT` | `text` | Pinned in projection; hard rule |
| `STATE_SET` | `doc` or `patch`, `state_version` | Mutable short-term state |
| `PROJECT_CONTEXT` | `policy_id`, `policy_hash`, `tokenizer_id`, `included_node_ids[]`, `omitted[]`, `bundle_hash`, `ltm_snapshot_id?` | Context assembly record |
| `THOUGHT` | `text` | Optional; may be redacted |
| `DECISION` | `step_n`, `plan` (see §5) | **Sealed** before tools |
| `TOOL_CALL` | `step_n`, `stable_call_id`, `tool_name`, `args`, `effect_class`, `schema_version?` | Only from sealed DECISION |
| `TOOL_RESULT` | `step_n`, `stable_call_id`, `status`, `result?`, `error?` | Durable outcome |
| `ARTIFACT_PUT` | `artifact_id`, `content_hash`, `mime`, `path?`, `labels[]?`, `data_class` | Register blob |
| `ARTIFACT_REF` | `artifact_id` | Point at existing blob |
| `LTM_QUERY` | `query`, `policy?` | Optional v0.1 |
| `LTM_HIT` | `hits[]`, `snapshot_id` | Pin if used in hash |
| `LTM_PROJECT` | `items[]` | Consolidate outward |
| `COMMIT_STEP` | `step_n`, `seal_id` | Step fence |
| `ABORT` | `reason`, `step_n?` | Failed/cancelled |
| `REDACTION` | `target_node_ids[]` | Privacy overlay |

**Deferred to v0.2+:** `AGENT_CALL` / `AGENT_RESULT`, `BRANCH` / `MERGE_REPORT`, full plan DAG nodes.

### 3.1 Common fields (every node)

```text
{
  "kind": string,
  "step_n": integer | null,      // null only for pre-step INPUT/CONSTRAINT if needed
  "seq": integer,                // order within trajectory (monotonic)
  "ts": ISO-8601,
  "payload_hash": string,        // sha256 hex of canonical payload bytes
  "node_id": string              // see §4
}
```

---

## 4. node_id hash recipe (normative)

### 4.1 Canonical payload

1. Take node-specific fields only (exclude `node_id`; **exclude `ts`** from hash so clock skew does not rewrite identity).  
2. Serialize with **RFC 8785 JSON Canonicalization Scheme (JCS)** — not “sorted keys” alone. Number formats, Unicode, and whitespace must follow JCS so hashes match across languages/runtimes.  
3. `payload_hash = sha256_hex(jcs_utf8_bytes)`.  

**Normative reference:** [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785.html).  
If two implementations disagree on a hash, fix the serializer to JCS; do not invent a private “almost canonical” format.

### 4.2 node_id

```text
node_id = sha256_hex(
  utf8(
    tenant_id + "|" +
    trajectory_id + "|" +
    str(step_n if step_n is not null else "none") + "|" +
    str(seq) + "|" +
    kind + "|" +
    payload_hash
  )
)
```

**Rules:**
- `node_id` is **identity**, not a suggestion.  
- Same inputs ⇒ same `node_id`.  
- Do **not** use payload-only hash as identity (collisions across steps).

### 4.3 artifact content_hash

```text
content_hash = sha256_hex(raw_bytes)
artifact_id  = content_hash
# multi-tenant production later:
# storage_key = sha256_hex(tenant_id + "|" + content_hash)
```

---

## 5. DECISION.plan shape (v0.1 linear)

v0.1 is **linear**, not a DAG.

```json
{
  "step_n": 1,
  "model": { "provider": "...", "name": "...", "params": {} },
  "state_intent": { },
  "tools": [
    {
      "stable_call_id": "c1",
      "tool_name": "cloud.deploy",
      "args": { },
      "effect_class": "NON_IDEMPOTENT_WRITE",
      "schema_version": "1.0"
    }
  ]
}
```

### 5.1 Linear known-args rule (normative — critical)

> **A sealed tool list is valid only if every tool’s `args` are fully known at seal time.**  
> If tool B needs tool A’s **result**, B **must not** appear in the same sealed `DECISION`.  
> B goes to a **later step** after A’s `TOOL_RESULT` is durable, via a **new inference + new DECISION**.

This is why linear v0.1 works **without** a plan DAG / symbolic data deps.

**Invalid (reject at seal):**
```text
DECISION tools: [A, B] where B.args reference A.output
```

**Valid:**
```text
STEP 1 DECISION: [A]
STEP 1 TOOL_RESULT(A)
STEP 2 PROJECT + infer
STEP 2 DECISION: [B with concrete args from A]
```

---

## 6. Ordering

- Global order: `(seq)` ascending.  
- Within step: `PROJECT_CONTEXT` → optional `THOUGHT` → `DECISION` → `TOOL_CALL`/`TOOL_RESULT` pairs → `STATE_SET`/`ARTIFACT_*` → `COMMIT_STEP`.  
- Readers only treat a step as complete after `COMMIT_STEP` for that `step_n`.

---

## 7. Out of scope for this doc

- Effect class definitions → [effect-classes.md](./effect-classes.md)  
- Resume / block-and-gate → [seals-and-resume.md](./seals-and-resume.md)  
- On-disk package → [traj-package.md](./traj-package.md)  
- Tests → [conformance.md](./conformance.md)

---

## 8. Freeze checklist

- [ ] v0.1 node set accepted  
- [ ] node_id recipe accepted  
- [ ] Linear known-args rule accepted  
- [ ] Plan DAG deferred to v0.2  

**Spec tag target:** `spec-v0.1` after all five Phase 0 docs reviewed.
