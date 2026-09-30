# Trajectory IR

**Portable, hash-verifiable intermediate representation for agent execution trajectories.**

Trajectory IR sits **on top of** durable execution engines. It does not replace Temporal, DBOS, or Restate. It is a **runtime flight recorder**: typed nodes, sealed decisions, fail-closed effect classes, and a `.tir` package auditors can verify by content hash. A seal freezes the plan, not the world. A client-side gate is at-most-one automatic attempt, not exactly-once at the far API.

!!! tip "Live demos for talks and docs"
    Start here: **[Demos](demos/index.md)** — crash-safe resume, portable `.tir` export, and sandbox rejection. These are the same Go examples we run on stage.

## What you get

| Capability | Why it matters |
|---|---|
| Sealed decisions | Resume does not silently re-ask the model for a sealed step. The world may still have moved. |
| Effect classes | Fail-closed mapping for tools (including MCP-aligned hints). `bash`/`python`/`sql` stay dangerous by name. |
| Block-and-gate | At-most-one automatic retry of a non-idempotent tool. Not exactly-once in the world. |
| Idempotency key | Hashed seal-derived key on `TOOL_CALL`, exposed to the tool body; host forwards `Idempotency-Key` to the remote API. |
| `.tir` packages | Thin or fat portable evidence with hash verification. This is the product. |
| Dual SDK | **Go primary**, Python reference / parity |

## Stack at a glance

- **Languages:** Go (primary SDK), Python (reference)
- **Durable backends:** Temporal (Go production), DBOS (Python reference)
- **Storage:** SQLite / Postgres NodeLog, filesystem or S3 / MinIO CAS
- **Interop:** Model Context Protocol (MCP) stdio tools under `TRAJIR_MCP_ROOT`

## CNCF note (honest)

This project maintains a [CNCF Sandbox application outline](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md) and process pack (maintainers, governance, roadmap, DCO, security).

**We do not claim CNCF membership or “donated to CNCF” status** until the TOC votes Approved and the contribution agreement is signed. On stage and on this site we say: *open source, Apache 2.0, preparing for Sandbox*.

## Normative spec

The authoritative contract lives in the repo at
[`docs/MASTER_SPECIFICATION.md`](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/docs/MASTER_SPECIFICATION.md).
The root [README](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/README.md) is the landing page for users and contributors.

## Next steps

1. [Watch the demos](demos/index.md)
2. [Run the Go quickstart](getting-started.md)
3. [Speaker runbook](talk/speaker-runbook.md) for conference delivery
