# Demo: Adoption host + thin `.tir`

Shows a complete host-owned step using only the **public Go client**, then exports a **thin `.tir` package** with CAS and rehydrates the artifact.

Source: [`go/examples/adoption_host`](https://github.com/Coder-s-OG-s/Trajectory-IR/tree/main/go/examples/adoption_host)

## Story in one minute

1. `OpenTrajectory` → `Project` → stub model → `SealDecision`
2. Execute `build_manifest` (**PURE**) and `ship_release` (**NON_IDEMPOTENT_WRITE**)
3. `CommitStep`
4. With `-with-package`: put bytes in CAS, export thin `.tir`, rehydrate by content hash

This is the **portability** beat for CNCF / cloud-native audiences: the audit artifact leaves the runtime.

## Run it yourself

From `go/`:

```bash
go run ./examples/adoption_host
go run ./examples/adoption_host -with-package
```

## Captured output (fixture)

```text
--8<-- "adoption_host_package.txt"
```

!!! info "Read the node kinds"
    `PROJECT_CONTEXT → DECISION → TOOL_CALL/RESULT → TOOL_CALL/RESULT → COMMIT_STEP` is the sealed step shape. The `.tir` preserves those identities for another runtime to verify.

## Why thin packages matter

| Mode | What travels |
|---|---|
| Thin | Manifest, nodes, seals, artifact **URIs / hashes** (CAS must hold bytes) |
| Fat | Same metadata plus embedded `artifacts/cas/...` bytes |

Default on stage: **thin + CAS** — closer to how operators share evidence without shipping giant archives.
