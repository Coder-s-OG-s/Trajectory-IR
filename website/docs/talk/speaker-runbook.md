# Speaker runbook (KubeCon / KCD Ahmedabad)

Audience assumption: cloud-native / platform / AI infra engineers. Time box: **20–30 minutes** including Q&A.

## Pre-flight (day before)

- [ ] Laptop on power; Go toolchain works: `cd go && go run ./examples/adoption_host`
- [ ] Clean demo dirs removed so counters start fresh
- [ ] Terminal font large (24+); dark theme; hide secrets in shell history
- [ ] Backup: this docs site open on phone/hotspot with embedded fixtures
- [ ] Slide deck has the boundary table (engines vs IR vs host)
- [ ] Do **not** claim CNCF membership; say “Apache 2.0, preparing for CNCF Sandbox”

## Stage sequence (optimized)

| Minute | Beat | Action |
|---|---|---|
| 0–3 | Problem | Duplicate deploy / re-inference after crash; checkpoint lock-in |
| 3–6 | Pitch | Portable IR on top of Temporal/DBOS; seals + effects + `.tir` |
| 6–12 | Live A | [Kill mid deploy](../demos/kill-mid-deploy.md) |
| 12–16 | Live B | [Adoption host `-with-package`](../demos/adoption-host.md) |
| 16–18 | Optional | [Sandbox](../demos/sandbox.md) (skip if time-tight) |
| 18–22 | Architecture | Layers + non-goals |
| 22–30 | Q&A | Point to repo + this site |

## Live command cheat sheet

```bash
cd go

# A — crash mid tool
go run ./examples/kill_mid_deploy -workdir ./kmd -crash-during=tool_call
# kill when TOOL_CALL starts
go run ./examples/kill_mid_deploy -workdir ./kmd -resume

# B — portable package
go run ./examples/adoption_host -with-package

# C — sandbox
go run ./examples/adoption_host -sandbox
```

## If live demo fails

1. Switch to **fixture terminals** on this site (already captured stdout).
2. Narrate counters: `model_count=1`, `deploy_count=0`, `BLOCKED_NEEDS_GATE`.
3. Do not debug Temporal on stage.

## Talking points that land with CNCF audiences

- Complementary to durable execution, not competitive
- Spec-shaped: conformance R01–R08
- Go primary SDK; Python reference
- Portable evidence (`.tir`) for audit / multi-agent handoff
- Fail-closed effect classification

## Links to hand out

- Repo: `https://github.com/Coder-s-OG-s/Trajectory-IR`
- Go quickstart: `go/QUICKSTART.md`
- CNCF prep (honest): `docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md`
- This demo site (after Pages deploy): `https://coder-s-og-s.github.io/Trajectory-IR/`
