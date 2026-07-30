# Go IR core

Second language implementation of Trajectory IR primitives. Python remains the Phase 1A reference runtime.

## Packages

| Path | Role |
|------|------|
| `trajir/nodes` | Node kinds, RFC 8785 payload hash, node id |
| `trajir/log` | SQLite append only NodeLog |
| `trajir/effects` | Effect classes and fail closed MCP mapping |
| `trajir/durable` | Pluggable step memoization backend |
| `trajir/resume` | Block and gate for NON_IDEMPOTENT_WRITE tools |

## Durable backend decision (issue #16)

| | |
|--|--|
| **Coding default** | `LocalSQLite` (file) and `Memory` (tests) in `trajir/durable` |
| **Production target** | Temporal adapter (follow up). Restate welcome later. |
| **Why** | Matches master README: do not hand roll crash engines. Local memo is enough to prove Infer/Tool skip on re-entry and to build seal and gate next, without standing up Temporal in every contributor environment. |

Model inference and tools must go through `durable.Step` / `Infer` / `Tool`. Block and gate still relies on the NodeLog for NON_IDEMPOTENT_WRITE.

## Test

```bash
cd go
go test ./...
```
