# Demos

These are the **conference-ready** Trajectory IR demos. They use the **Go primary SDK** and match Phase 1B messaging.

| Demo | Claim it proves | Stage time | Command |
|---|---|---|---|
| [Kill mid deploy](kill-mid-deploy.md) | Seal + crash mid non-idempotent tool → honest gate, no silent re-deploy | 3–4 min | `go run ./examples/kill_mid_deploy ...` |
| [Adoption host + `.tir`](adoption-host.md) | Host loop + thin package + CAS rehydrate | 2 min | `go run ./examples/adoption_host -with-package` |
| [Sandbox mode](sandbox.md) | R06: reject real `NON_IDEMPOTENT_WRITE` in sandbox | 30–45 sec | `go run ./examples/adoption_host -sandbox` |

## Recommended talk order

```text
1. Problem slide (duplicate deploy / re-inference after crash)
2. Kill mid deploy (live or recorded)
3. Adoption host -with-package (show portable .tir)
4. Optional: sandbox reject
5. Boundary slide (Temporal owns durability; IR owns seals + .tir)
```

## Captured terminal output

Pages below embed **real captured stdout** from `go run` on this repository (fixtures under `website/fixtures/`). Refresh fixtures with:

```powershell
pwsh website/scripts/capture_demos.ps1
```

```bash
bash website/scripts/capture_demos.sh
```

## What we are not demoing on stage

- Full Temporal cluster boot (too heavy for a short talk; mention as production path)
- Browser / WASM playground (not in scope yet)
- Claiming CNCF Sandbox membership (prep only)
