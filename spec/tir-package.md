# tir-package.md — Trajectory IR portable package v0.1

**Status:** Phase 0 freeze candidate  
**Version:** 0.1  
**Extension:** **`.tir`** (Trajectory Intermediate Representation package)

---

## 1. Why not `.traj`

**SWE-agent** already uses **`.traj`** for trajectory logs in the same domain (agent runs).  
Using `.traj` for Trajectory IR would cause:

- Tooling/SEO/docs collision  
- “Which .traj?” confusion in the ecosystem  
- Migration pain if we rename after launch  

**v0.1 decision:** package extension = **`.tir`**  
(Alternative considered: `.trajir` — longer; prefer `.tir`.)

MIME suggestion: `application/vnd.trajectory-ir.tir+zip` or directory form without zip.

---

## 2. Package forms

| Form | Layout | Use |
|------|--------|-----|
| **Directory** | folder ending in `.tir/` or named `*.tir` dir | Dev |
| **Zip** | `name.tir` zip of same layout | Share / CI |

---

## 3. Layout (normative)

```text
manifest.json              # required
nodes.ndjson               # required — one node JSON per line, seq order
seals.json                 # required — array of seals (may be [])
artifacts-manifest.json    # required — map content_hash -> meta (+ uri if thin)
artifacts/                 # fat only — files named by content_hash
projector-policy.yaml      # optional but recommended
COMPAT.json                # required — min runtime / schema
SIGNATURE                  # optional (v0.1: presence only; format not frozen)
README.txt                 # optional human note
```

### SIGNATURE (v0.1)

Optional. **Not specified in detail for v0.1** (algorithm, payload coverage, key id).  
If present, importers may ignore or best-effort verify; **must not** reject a package solely for missing `SIGNATURE`.  
Full signature profile deferred to a later minor (e.g. detached ed25519 over `manifest.content_hash` + seals).

---

## 4. manifest.json

```json
{
  "format": "trajectory-ir-package",
  "format_version": "0.1",
  "extension": ".tir",
  "trajectory_id": "...",
  "schema_version": "0.1",
  "tenant_id": "...",
  "status": "OPEN",
  "created_at": "...",
  "package_mode": "fat" | "thin" | "redacted",
  "node_count": 0,
  "content_hash": "sha256 of nodes.ndjson bytes"
}
```

---

## 5. nodes.ndjson

- UTF-8, one canonical node object per line.  
- Strictly increasing `seq`.  
- Must match [node-kinds.md](./node-kinds.md).  
- Import verifies each `node_id` recomputes correctly.

---

## 6. seals.json

```json
[
  {
    "seal_id": "...",
    "seal_type": "DECISION_SEAL",
    "step_n": 1,
    "covered_node_ids": ["..."],
    "created_at": "..."
  }
]
```

Import must re-verify seal hashes when possible.

---

## 7. artifacts-manifest.json

```json
{
  "<content_hash>": {
    "mime": "application/json",
    "path": "openapi.yaml",
    "bytes": 1234,
    "uri": "s3://..." 
  }
}
```

| Mode | Artifacts |
|------|-----------|
| **fat** | `artifacts/<content_hash>` present; uri optional |
| **thin** | no `artifacts/` dir required; **uri required** (or resolver policy) |
| **redacted** | strip/omit sensitive nodes from nodes.ndjson; may drop secret artifacts |

### 7.1 Redacted mode rules (v0.1)

- Drop or replace `THOUGHT` payloads.  
- Apply `REDACTION` targets.  
- Strip env secrets from TOOL_CALL args if marked sensitive.  
- `manifest.package_mode = "redacted"`.

---

## 8. COMPAT.json

```json
{
  "min_runtime_version": "0.1.0",
  "schema_version": "0.1",
  "features": ["linear-decision", "block-and-gate"]
}
```

Importer **rejects** unknown major schema or missing features it cannot honor.

---

## 9. Import / export behavior

### Export
1. Freeze nodes + seals for selected trajectory (or snapshot).  
2. Choose mode fat/thin/redacted.  
3. Write layout; compute `manifest.content_hash`.  
4. Optional signature over manifest + content_hash.

### Import
1. Read COMPAT + manifest.  
2. Verify nodes.ndjson hash and node_ids.  
3. Verify seals.  
4. Resolve artifacts (fat copy in; thin fetch by uri).  
5. Trajectory status → **SUSPENDED/OPEN** per runtime; **do not** auto-acquire lease.  
6. Resume uses [seals-and-resume.md](./seals-and-resume.md).

---

## 10. Freeze checklist

- [ ] Extension **`.tir`** accepted (not `.traj`)  
- [ ] fat/thin/redacted accepted  
- [ ] manifest + ndjson + seals layout accepted  
- [ ] import verification steps accepted  

**Spec tag target:** `spec-v0.1`
