# seals-and-resume.md — Trajectory IR v0.1

**Status:** Phase 0 freeze candidate  
**Version:** 0.1  
**Depends on:** [node-kinds.md](./node-kinds.md), [effect-classes.md](./effect-classes.md)

---

## 1. Purpose

Normative rules for **seals**, **step/sub-step fence**, **resume matrix**, and **block-and-gate** when a `NON_IDEMPOTENT_WRITE` (or similar) crashes mid-tool.

This is the fix for: *re-infer → new tool ids → double side effects*.

---

## 2. Seals

### 2.1 Seal types

| Seal | Covers | When written |
|------|--------|--------------|
| `DECISION_SEAL` | Canonical hash of sealed `DECISION` plan for `step_n` | Immediately after DECISION is durable |
| `STEP_SEAL` | All committed nodes of `step_n` including `COMMIT_STEP` | After successful commit |
| `TRAJECTORY_SEAL` | Full finished trajectory (optional v0.1) | On complete/archive |

### 2.2 Seal record

```text
Seal {
  seal_id: sha256_hex(canonical_bytes_of_covered_node_ids + seal_type + step_n)
  seal_type: DECISION_SEAL | STEP_SEAL | TRAJECTORY_SEAL
  step_n?: integer
  covered_node_ids: string[]   # sorted
  created_at: ISO-8601
  signature?: string           # optional v0.1
}
```

---

## 3. Step protocol (normative)

```text
STEP n:
  1. Optionally append INPUT / CONSTRAINT (usually earlier)
  2. PROJECT_CONTEXT  (durable)
  3. MODEL_INFER      (ephemeral — not a node until DECISION)
  4. Append DECISION with full concrete tool args
     - Validate linear known-args rule (node-kinds §5.1)
     - Reject seal if any arg depends on future tool result
  5. Write DECISION_SEAL
  6. For each tool in sealed plan order:
       a. Append TOOL_CALL
       b. Execute with effect policy
       c. Append TOOL_RESULT (durable) OR enter BLOCKED_NEEDS_GATE
  7. STATE_SET / ARTIFACT_* as needed
  8. COMMIT_STEP(n) + STEP_SEAL
```

### 3.1 Lease (recommended v0.1, required for multi-worker)

- Single writer per trajectory via lease + fencing token.  
- Long tools: **heartbeat / renew lease** while tool runs.  
- Stale fence epoch → reject writes.

---

## 4. Resume matrix (normative)

Let `n` = current incomplete step (last committed is `n-1` or none).

| State found on open | Action on resume |
|---------------------|------------------|
| No `DECISION` for step `n` | Safe to re-run `PROJECT_CONTEXT` + **new inference** + new `DECISION` |
| `DECISION` + `DECISION_SEAL` present; some tools without `TOOL_RESULT` | **Do not re-infer.** Continue sealed tool list from first incomplete `stable_call_id` |
| All tools have `TOOL_RESULT`; no `COMMIT_STEP` | Finish STATE/ARTIFACT + `COMMIT_STEP` (no re-infer) |
| `COMMIT_STEP(n)` present | Step done; start `n+1` only via new project/infer |
| Tool in `IN_PROGRESS` / crash mid-tool (see §5) | Apply effect-class policy (block-and-gate vs replay) |
| `BLOCKED_NEEDS_GATE` | Wait for gate resolution; do not auto-continue |

**Default:** After `DECISION_SEAL`, **never re-infer** for that step unless explicit **unsafe** mode (not in v0.1 GA).

---

## 5. Crash mid-tool policies

### 5.1 PURE / READ_ONLY

- May re-execute or re-fetch per policy.  
- Write `TOOL_RESULT` when done.  
- **READ_ONLY re-fetch on resume (divergence rule):** a re-fetch may observe a **different** value than the original run. Write it as a **new** `TOOL_RESULT` (new `seq` / identity). The **original** observation, if any, is **preserved** (never overwritten). Downstream work after resume sees the **new** observation. This is honest, recorded divergence — not silent replay of stale data.

### 5.2 IDEMPOTENT_WRITE

- Replay with **same** `idempotency_key`.  
- If result store has completed result → return it, do not re-hit world if adapter supports.  
- If `IN_PROGRESS` without heartbeat beyond TTL → mark failed/retryable per registry.

### 5.3 NON_IDEMPOTENT_WRITE — **block-and-gate (normative default)**

If execution **started** and durable `TOOL_RESULT` is **missing**:

1. Set call status = **`BLOCKED_NEEDS_GATE`** (durable).  
2. **Do not** auto-retry the tool.  
3. **Do not** re-infer.  
4. Require explicit gate:

| Gate choice | Meaning |
|-------------|---------|
| `CONFIRM_SUCCESS` | Operator asserts world already applied; write synthetic/confirmed `TOOL_RESULT` |
| `CONFIRM_FAILURE` | Assert no apply / failed; write failed `TOOL_RESULT` |
| `RUN_COMPENSATION` | If compensation tool registered; then re-decide later step |
| `EXPLICIT_RERUN` | Dangerous; only with new DECISION in **new step** after human ack |

**Accepted as v0.1 default** for coding-agent “kill mid-deploy” safety.

### 5.4 AGENT_SPAWN

Treat like NON_IDEMPOTENT unless registry says otherwise → block-and-gate.

### 5.5 in_progress TTL / heartbeat

```text
tool_attempt {
  status: IN_PROGRESS | COMPLETED | FAILED | BLOCKED_NEEDS_GATE
  heartbeat_at
  ttl_ms
}
```

- Worker heartbeats while tool runs.  
- If `now - heartbeat_at > ttl` and not COMPLETED → **not** infinite block: transition to `BLOCKED_NEEDS_GATE` or `FAILED` per class (NON_IDEMPOTENT → gate; IDEMPOTENT → retryable failed).

---

## 6. Linear known-args + resume (interaction)

Because v0.1 forbids B depending on A inside one sealed plan:

- Mid-step resume only replays **remaining sealed tools with known args**.  
- Tools that need prior results only appear in **later steps** after prior `TOOL_RESULT` exists for the next inference.

---

## 7. Staged / uncommitted nodes

On re-entry to step `n` when recovering a dirty write path:

- Nodes after last durable fence without `COMMIT_STEP` must be **superseded or deleted** if they conflict with sealed plan replay.  
- **Normative:** append-only log preferred: mark invalid region with `ABORT` + reason `superseded_on_resume`, then continue from sealed DECISION.  
- Never leave two competing TOOL_CALLs for same `stable_call_id`.

---

## 8. Freeze checklist

- [ ] Decision-before-tools accepted  
- [ ] Resume matrix accepted  
- [ ] Block-and-gate for NON_IDEMPOTENT mid-tool accepted  
- [ ] No re-infer after DECISION_SEAL (default)  
- [ ] Heartbeat/TTL for long tools accepted  

**Spec tag target:** `spec-v0.1`
