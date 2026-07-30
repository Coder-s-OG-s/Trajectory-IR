# Go IR core

Second language implementation of Trajectory IR primitives. Python remains the Phase 1A reference runtime.

## Packages

| Path | Role |
|------|------|
| `trajir/nodes` | Node kinds, RFC 8785 payload hash, node id |
| `trajir/log` | SQLite append only NodeLog |
| `trajir/effects` | Effect classes and fail closed MCP mapping |
| `trajir/durable` | Pluggable step memoization backend |
| `trajir/durable/temporal` | Temporal Backend + worker registration |
| `trajir/resume` | Block and gate, and RunStep seal path for one agent step |
| `trajir/client` | Thin SDK: open, project, seal, exec, commit, resume, RunStep |

## Durable backend decision (issues #16 and #24)

| | |
|--|--|
| **Coding default** | `LocalSQLite` (file) and `Memory` (tests) in `trajir/durable` |
| **Production target** | Temporal (`trajir/durable/temporal`). Restate welcome later. |
| **Why** | Matches master README: do not hand roll crash engines. Local memo stays the default for contributors; Temporal persists step memos when a cluster and worker are available. |

Model inference and tools must go through `durable.Step` / `Infer` / `Tool`. Block and gate still relies on the NodeLog for NON_IDEMPOTENT_WRITE.

### Temporal (optional)

Env (defaults match local Temporal dev server):

| Variable | Default |
|----------|---------|
| `TEMPORAL_HOSTPORT` | `localhost:7233` |
| `TEMPORAL_NAMESPACE` | `default` |
| `TEMPORAL_TASK_QUEUE` | `trajectory-ir` |

Run a worker process that calls `temporal.NewWorker` and `Run`. Use `temporal.Dial` or `temporal.NewBackend` as a `durable.Backend` with `durable.Infer` / `durable.Tool` / `resume.RunStep`.

Default `go test ./...` does **not** need Temporal. Optional live check:

```bash
# with Temporal listening on localhost:7233 and a worker on trajectory-ir
go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
```

## Client usage

```go
tr, err := client.OpenTrajectory("demo", "t1", client.Options{WorkDir: dir})
// ...
results, err := tr.RunStep(ctx, 1, model, tools, map[string]any{"k": "v"})
// reopen same workdir
tr2, err := client.Resume("demo", "t1", client.Options{WorkDir: dir})
```

| Go | Python |
|----|--------|
| `OpenTrajectory` | `open_trajectory` |
| `Resume` | `resume` |
| `Project` | `project` |
| `SealDecision` | `seal_decision` |
| `ExecTool` | `exec_tool` |
| `CommitStep` | `commit_step` |
| `RunStep` | full step via runtime (convenience) |

## Test

```bash
cd go
go test ./...
```

Crash resume conformance (R01/R02 style) lives under `conformance/`.

1. In-process tests always run (panic after seal, TOOL_CALL pre-seed for gate).
2. Subprocess tests build `cmd/crashagent`, hard-kill at markers, then resume.
   They skip if the host blocks running the binary (some Windows policies).

```bash
cd go
go test ./conformance -count=1 -v
```
