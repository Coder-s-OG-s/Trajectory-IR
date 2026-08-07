# Go adoption host demo

Host owned seal loop using only the public Go client. Stub model (no paid API).
Optional filesystem CAS and thin `.tir` export with rehydrate check.

## What it proves

1. `OpenTrajectory` → `Project` → stub model → `SealDecision`
2. `ExecTool` for PURE (`build_manifest`) and gated NON_IDEMPOTENT_WRITE (`ship_release`)
3. `CommitStep`
4. Optional `-with-package`: CAS put, thin export, rehydrate
5. Optional `-sandbox`: non idempotent tools rejected

## How this differs

| Example | Intent |
|---------|--------|
| `examples/adoption_host` (this) | Adoption: host loop + optional package |
| `examples/kill-mid-deploy` | Crash safety / durable resume |

## Run

From `go/`:

```bash
go run ./examples/adoption_host
go run ./examples/adoption_host -sandbox
go run ./examples/adoption_host -with-package
```

Tests:

```bash
go test ./examples/adoption_host -count=1
```
