# conformance.md — Trajectory IR v0.1

**Status:** Phase 0 freeze candidate  
**Version:** 0.1  
**Depends on:** all other Phase 0 specs  
**Rule:** No `spec-v0.1` implementation claim without listed tests runnable in CI.

---

## 1. Purpose

Each test is **runnable**: given setup → action → expected observable result.  
Phase 1A minimum green: **R01, R02** (+ kill-mid-deploy demo).  
Full Phase 0 suite: **R01–R08**.

---

## 2. Harness assumptions

- In-memory or **SQLite IR log** + local artifact directory.  
- Fake tools: `pure.echo`, `read.time`, `write.idempotent`, `write.deploy` (NON_IDEMPOTENT).  
- Ability to **kill** process between IR operations (or inject crash).  
- Tokenizer for projection tests: pinned test tokenizer (e.g. char/4 or fixed tiktoken name in policy).

---

## 3. Tests

### R01 — seal_decision then kill → resume without re-infer

**Setup:**  
- Trajectory T, step 1.  
- `PROJECT_CONTEXT` written.  
- Model plan stubbed once: one tool `pure.echo` with `stable_call_id=c1`.  
- `DECISION` + `DECISION_SEAL` durable.  
- **Crash before any TOOL_CALL.**

**Action:**  
- Restart runtime; `resume(T)`.  
- Instrument model: **must not be called** on resume.

**Expect:**  
- No new `DECISION` for step 1.  
- `TOOL_CALL`/`TOOL_RESULT` for `c1` from sealed plan.  
- Eventually `COMMIT_STEP(1)`.  
- Model invoke count on resume path = **0**.

---

### R02 — NON_IDEMPOTENT tool not double-executed

**Setup:**  
- Sealed DECISION with `write.deploy` (`NON_IDEMPOTENT_WRITE`), `stable_call_id=d1`.  
- Tool adapter increments global `deploy_count` on each real exec.  
- Crash **after tool start, before durable TOOL_RESULT** (simulate mid-tool).

**Action:**  
- Resume without gate → must **not** auto exec.  
- Status `BLOCKED_NEEDS_GATE`.  
- Then gate `CONFIRM_SUCCESS` (or explicit path) once.

**Expect:**  
- `deploy_count == 1` after full recovery path that confirms success without re-exec.  
- If operator chooses path that re-executes, must be **EXPLICIT_RERUN** in a **new step** only (document); default path does not increment twice.

**Phase 1A demo mapping:** kill-mid-deploy script asserts deploy once.

---

### R03 — PURE tool may recompute

**Setup:**  
- Sealed plan with `pure.echo`.  
- Crash after TOOL_CALL started; no result yet.  
- PURE policy allows recompute.

**Action:** Resume.

**Expect:**  
- Tool may run again; final `TOOL_RESULT` present; step can commit.  
- No block-and-gate required.

---

### R04 — CONSTRAINT always in project under budget

**Tier:** **R03–R08 “later”** relative to Phase 1A (R01/R02 first).

**Runtime invariant (normative, not package-dependent):**  
v0.1 defines a **minimal built-in default projector policy**: pins are always included; `CONSTRAINT` nodes are never dropped; if constraints alone exceed the token budget → hard `BUDGET_IMPOSSIBLE` (never silent trim).  
Package `projector-policy.yaml` remains **optional**. Custom policies must uphold the same CONSTRAINT invariant to pass R04.  
Tests may use a fixed test tokenizer (e.g. `test/char4`) for budget math.

**Setup:**  
- Many filler THOUGHT/INPUT nodes.  
- One `CONSTRAINT` node: `"Do not push to main"`.  
- Call `project()` with **default built-in policy** (no package yaml required).

**Action:** `project()`.

**Expect:**  
- Bundle includes CONSTRAINT text.  
- `budget_tokens` not exceeded.  
- If constraints alone exceed budget → `BUDGET_IMPOSSIBLE`, not silent drop.

---

### R05 — export/import preserves seals

**Setup:**  
- Trajectory with DECISION_SEAL + STEP_SEAL for step 1.  
- One artifact in fat package.

**Action:**  
- `export(mode=fat)` → `sample.tir`  
- Fresh empty store `import(sample.tir)`  
- Recompute seal hashes and node_ids.

**Expect:**  
- Same `trajectory_id`, node_ids, seal_ids.  
- Artifact bytes match content_hash.  
- Resume/open succeeds.

---

### R06 — sandbox blocks NON_IDEMPOTENT

**Setup:**  
- Trajectory or child in `SANDBOX` mode (or policy flag if BRANCH deferred: `execution_mode=SANDBOX` on trajectory metadata).  
- Attempt seal DECISION containing `write.deploy`.

**Action:** Seal or exec.

**Expect:**  
- Reject at seal or exec with clear error.  
- `deploy_count` unchanged.

---

### R07 — graft artifacts without copying thoughts

**Setup:**  
- Trajectory A has THOUGHT + ARTIFACT_PUT(report).  
- Trajectory B grafts only artifact ids from A.

**Action:** Graft; project B.

**Expect:**  
- B can resolve artifact bytes.  
- B nodes do **not** include A’s THOUGHT payloads.  
- No automatic share of A’s DECISION text.

---

### R08 — redaction hides nodes from project

**Setup:**  
- THOUGHT with secret string `SECRET42`.  
- REDACTION targeting that node (or redacted export).

**Action:** `project()` on redacted view / after redaction.

**Expect:**  
- Bundle does not contain `SECRET42`.  
- Omission or redaction marker allowed.

---

## 4. Phase gates

| Gate | Required tests |
|------|----------------|
| Phase 1A start of coding | Spec tag `spec-v0.1` |
| Phase 1A done | **R01, R02** green + kill-mid-deploy demo script |
| Phase 1A complete suite | R01–R08 green |
| Later | Lease dual-worker, LTM pin tests, etc. |

---

## 5. Kill-mid-deploy demo script (acceptance narrative)

```text
1. Create trajectory; seal DECISION with write.deploy
2. Start deploy tool (sleep inside tool to allow kill)
3. Kill process
4. Restart; resume
5. Assert: BLOCKED_NEEDS_GATE or single completion path
6. Assert: deploy_count == 1 after confirmed recovery
7. Print success
```

Script lives with runtime skeleton after spec freeze (not before tag).

---

## 6. Freeze checklist

- [ ] R01–R08 descriptions accepted as runnable  
- [ ] Phase 1A = R01/R02 minimum accepted  
- [ ] Demo narrative accepted  

**Spec tag target:** `spec-v0.1`
