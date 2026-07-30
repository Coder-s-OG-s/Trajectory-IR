# effect-classes.md — Trajectory IR v0.1

**Status:** Phase 0 freeze candidate  
**Version:** 0.1  
**Depends on:** [node-kinds.md](./node-kinds.md)

---

## 1. Purpose

Define the **six effect classes** for tools and how they map from **MCP annotations** (and similar registries).  
Effect class drives **resume**, **idempotency**, and **sandbox** policy.

---

## 2. Six classes (normative)

| Class | Meaning | Auto-replay on resume? |
|-------|---------|-------------------------|
| `PURE` | No external world change (local pure compute) | Yes (recompute OK) |
| `READ_ONLY` | External read only (GET/list) | Policy: re-fetch or cache; re-fetch = **new node**, original preserved (see seals-and-resume) |
| `IDEMPOTENT_WRITE` | Write safe with same idempotency key | Yes, **same key only** |
| `NON_IDEMPOTENT_WRITE` | Mutating / destructive / not safely retryable | **No auto re-exec** |
| `AGENT_SPAWN` | Start/call another agent | Policy-gated; no blind retry |
| `SENSITIVE` | Touches secrets/PII (may combine with above) | Extra ACL + redaction |

**Note:** A tool has **one primary** `effect_class` for control flow. `SENSITIVE` may be a **flag** `sensitive: true` on the call in addition to primary class if implementation prefers; v0.1 allows either:
- primary class = `SENSITIVE` for secret-only tools, or  
- primary class = e.g. `READ_ONLY` + `sensitive: true`.

**Recommended v0.1:** single enum field `effect_class` including `SENSITIVE`; if a write is both sensitive and non-idempotent, use **`NON_IDEMPOTENT_WRITE`** and set `sensitive: true`.

---

## 3. Tool registry entry (minimal)

```yaml
tool_name: cloud.deploy
schema_version: "1.0"
effect_class: NON_IDEMPOTENT_WRITE
sensitive: false
idempotency: required    # required | optional | none
compensation: cloud.deploy.rollback   # optional
timeout_ms: 3600000
lease_heartbeat: required  # long tools
mcp_name: optional         # original MCP tool name
```

Every `TOOL_CALL` node **must** copy `effect_class` (and `sensitive` if set) from the registry at seal time (frozen into DECISION plan).

---

## 4. MCP annotation → effect_class mapping

MCP / tool metadata varies by server. Normalize at **registration** time.

| Signal from MCP / tool metadata | Mapped `effect_class` |
|---------------------------------|------------------------|
| Explicit read-only / `readOnly` / GET-like | `READ_ONLY` |
| Explicit idempotent + mutating | `IDEMPOTENT_WRITE` |
| Explicit destructive / non-idempotent mutate | `NON_IDEMPOTENT_WRITE` |
| Local pure / no side effects declared | `PURE` |
| Spawns agent / A2A / sub-agent | `AGENT_SPAWN` |
| Touches credentials, tokens, PII | set `sensitive: true` (+ class as above) |
| **Silent / missing / ambiguous annotations** | **`NON_IDEMPOTENT_WRITE` (fail-closed)** |

### 4.1 Fail-closed rule (normative)

> If MCP (or any tool source) does **not** clearly classify the tool, register it as **`NON_IDEMPOTENT_WRITE`**.  
> Do **not** default to PURE or READ_ONLY.

Operators may later promote a tool to a safer class with an explicit registry override and audit log.

---

## 5. Policy by class

| Class | Seal required before exec? | Idempotency key | Crash mid-tool |
|-------|----------------------------|-----------------|----------------|
| `PURE` | After DECISION | Optional | Recompute OK |
| `READ_ONLY` | After DECISION | Optional | Re-fetch or use cache policy |
| `IDEMPOTENT_WRITE` | After DECISION | **Required** | Replay same key |
| `NON_IDEMPOTENT_WRITE` | After DECISION | Required if tool supports; else gate | **Block-and-gate** (see seals-and-resume) |
| `AGENT_SPAWN` | After DECISION | Policy | Block-and-gate unless marked safe |
| sensitive | Same as primary | Same | Extra audit |

### 5.1 Idempotency key format

```text
idempotency_key = trajectory_id + ":" + str(step_n) + ":" + stable_call_id
```

`stable_call_id` comes from **sealed DECISION.plan.tools[]**, not from a fresh model free-form id after re-infer.

---

## 6. Sandbox / branch (forward-compatible)

Even if BRANCH is v0.2, policy table for later:

| Mode | Allowed classes |
|------|-----------------|
| `MAIN` | Per registry |
| `SANDBOX` | `PURE`, `READ_ONLY`, mocked writes only |
| `WHATIF` | No `NON_IDEMPOTENT_WRITE`, no real `AGENT_SPAWN` |

---

## 7. Freeze checklist

- [ ] Six classes accepted  
- [ ] Silent MCP = NON_IDEMPOTENT fail-closed accepted  
- [ ] Idempotency key format accepted  
- [ ] Sensitive handling accepted  

**Spec tag target:** `spec-v0.1`
