# Go IR core

**Primary SDK for Phase 1B** (epic #113). Full Trajectory IR stack under `trajir/`.
Python remains the supported **reference and parity port** from Phase 1A.

## Packages

| Path | Role |
|------|------|
| `trajir/nodes` | Node kinds, RFC 8785 payload hash, node id |
| `trajir/log` | SQLite append only NodeLog |
| `trajir/postgres` | PostgreSQL NodeLog (parity with `drivers.postgres`; needs DSN) |
| `trajir/effects` | Effect classes and fail closed MCP mapping |
| `trajir/durable` | Pluggable step memoization backend |
| `trajir/durable/temporal` | Temporal Backend + worker registration |
| `trajir/resume` | Block and gate, and RunStep seal path for one agent step |
| `trajir/client` | Thin SDK: open, project, seal, exec, commit, resume, RunStep |
| `trajir/cas` | Filesystem CAS and S3 compatible CAS (sharded layout; thin rehydrate; AWS SDK v2 via `NewS3StoreFromEnv`) |
| `trajir/tir` | Portable `.tir` package export / import (thin and fat) |
| `trajir/mcp` | Model Context Protocol server tools (status, export/import, verify) |
| `cmd/trajir-mcp` | stdio MCP binary for Claude Code / Cursor style hosts |
| `trajir/projector` | Default context projector (R04; size metric = RFC 8785 / JCS bytes) |

Integrations contract: [docs/INTEGRATIONS.md](../docs/INTEGRATIONS.md).

## Durable backend decision (issues #16 and #24)

| | |
|--|--|
| **Coding default** | `LocalSQLite` (file) and `Memory` (tests) in `trajir/durable` |
| **Production target** | Temporal (`trajir/durable/temporal`). Restate welcome later. |
| **Why** | Matches master README's durable-backend principle (§3.1): never hand roll crash/retry/lease logic, consume an external engine instead. Temporal is the master spec's recognized production backend for Go (§3.1, §5, §12.0); DBOS remains the Python default. Local memo stays the default for contributors; Temporal persists step memos when a cluster and worker are available. |

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

## Demo: kill mid deploy

Human runnable story (seal, kill, resume):

```bash
cd go
go run ./examples/kill-mid-deploy -workdir ./kill-mid-deploy-data -crash-during=tool_call
# kill when deploy starts, then:
go run ./examples/kill-mid-deploy -workdir ./kill-mid-deploy-data -resume
```

See [examples/kill-mid-deploy/README.md](examples/kill-mid-deploy/README.md).

| Go | Python |
|----|--------|
| `OpenTrajectory` | `open_trajectory` |
| `Resume` | `resume` |
| `Project` | `project` |
| `SealDecision` | `seal_decision` |
| `ExecTool` | `exec_tool` |
| `CommitStep` | `commit_step` |
| `RunStep` | full step via runtime (convenience) |

## Portable packages (`.tir`)

Matches the Python reference in `pkg/trajectory_ir/package/tir.py` (README §9).

```go
import (
    nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
    "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

// Export thin package
path, err := tir.Export(nl, "t-export", "out.tir", tir.ExportOptions{Mode: tir.ModeThin})

// Import (always verifies node ids; idempotent append)
pkg, err := tir.Import(path, nl)

// Load without writing to a log
pkg, err = tir.Load(path)
```

| Go | Python |
|----|--------|
| `tir.Export` | `export_tir` |
| `tir.Import` | `import_tir` |
| `tir.Load` | `load_tir` |
| `tir.LoadUnverified` | `load_tir_unverified` |

Fat mode uses the same CAS layout: `artifacts/cas/<2-char-shard>/<sha256>`.
Package signatures stay null/unimplemented. Redacted export is Python-only for now.

Cross-language fixture: `testdata/sample_thin.tir` (regenerate with `python scripts/gen_tir_fixture.py`).

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

