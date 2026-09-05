# Trajectory IR

**Portable, hash-verifiable intermediate representation for agent execution trajectories.**

Trajectory IR sits **on top of** durable execution engines. It does not replace Temporal, DBOS, or Restate. It seals model decisions, classifies tool effects, and exports a runtime-independent `.tir` package that auditors and peer agents can verify by content hash.

!!! tip "Live demos for talks and docs"
    Start here: **[Demos](demos/index.md)** — crash-safe resume, portable `.tir` export, and sandbox rejection. These are the same Go examples we run on stage.

## What you get

| Capability | Why it matters |
|---|---|
| Sealed decisions | Resume does not silently re-ask the model for a sealed step |
| Effect classes | Fail-closed mapping for tools (including MCP-aligned hints) |
| Block-and-gate | Non-idempotent tools do not double-fire after a mid-flight crash |
| `.tir` packages | Thin or fat portable evidence with hash verification |
| Dual SDK | **Go primary**, Python reference / parity |

## Stack at a glance

- **Languages:** Go (primary SDK), Python (reference)
- **Durable backends:** Temporal (Go production), DBOS (Python reference)
- **Storage:** SQLite / Postgres NodeLog, filesystem or S3 / MinIO CAS
- **Interop:** Model Context Protocol (MCP) stdio tools under `TRAJIR_MCP_ROOT`

## CNCF note (honest)

This project maintains a [CNCF Sandbox application outline](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md) and process pack (maintainers, governance, roadmap, DCO, security).

**We do not claim CNCF membership or “donated to CNCF” status** until the TOC votes Approved and the contribution agreement is signed. On stage and on this site we say: *open source, Apache 2.0, preparing for Sandbox*.

## Next steps

1. [Watch the demos](demos/index.md)
2. [Run the Go quickstart](getting-started.md)
3. [Speaker runbook](talk/speaker-runbook.md) for conference delivery
