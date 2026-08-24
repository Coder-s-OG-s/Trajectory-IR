# Go demo: kill mid deploy

Shows honest seal and resume for Trajectory IR in Go (same story as the Python demo under `examples/kill_mid_deploy/`).

Uses `trajir/client` with LocalSQLite for the IR log and durable step memos.

## What you should see

1. Model seals a deploy plan (DECISION).
2. `deploy_server` starts (NON_IDEMPOTENT_WRITE).
3. You kill the process mid flight.
4. On `-resume`, the model is not asked again after seal, and a mid tool crash does not silently re-run deploy (block and gate).

## Setup

From the repo root:

```bash
cd go
```

## Crash during deploy (R02 style)

Terminal 1:

```bash
go run ./examples/kill_mid_deploy -workdir ./kill_mid_deploy-data -crash-during=tool_call
```

When you see `TOOL_CALL: deploy_server started`, kill the process:

```bash
# Unix / Git Bash
kill -9 <pid>

# Windows PowerShell (another window)
# find PID from the go run process tree, then:
Stop-Process -Id <pid> -Force
```

Terminal 1 again (or a new one):

```bash
go run ./examples/kill_mid_deploy -workdir ./kill_mid_deploy-data -resume
```

Expect a `BLOCKED_NEEDS_GATE` line and `deploy_count=0` (effect never completed; gate refuses a blind retry).

## Crash after decision seal (R01 style)

```bash
go run ./examples/kill_mid_deploy -workdir ./kill_mid_deploy-data2 -crash-after=decision_sealed
# kill when you see DECISION sealed
go run ./examples/kill_mid_deploy -workdir ./kill_mid_deploy-data2 -resume
```

Expect `model_count=1` across both runs and a completed deploy on resume (`deploy_count=1`).

## Clean workdir

```bash
# Unix
rm -rf ./kill_mid_deploy-data

# PowerShell
Remove-Item -Recurse -Force .\kill_mid_deploy-data
```

## Related

- CI style tests: `go test ./conformance`
- Fixture binary also used by tests: `go/cmd/crashagent`
