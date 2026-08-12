# Milestones (how we plan work)

GitHub **Milestones** are the planning spine for Trajectory IR. Issues and PRs
should always land on exactly one open milestone (or a closed historical
milestone when filing retrospective notes).

Board: [Milestones](https://github.com/Coder-s-OG-s/Trajectory-IR/milestones)

## Active and historical milestones

| Milestone | State | Purpose |
|-----------|--------|---------|
| **Phase 1A library baseline (v0.1.0)** | Closed | First library tag. Historical only. |
| **v0.1.1 post 0.1.0 patch** | Closed | Absorbed into history; public cut advanced to v0.2.0. |
| **Phase 1B Go primary SDK** | Closed | Complete; released as **v0.2.0**. |
| **Phase 1C harden and adopt** | Closed | Complete at **v0.2.1** (protection, live stack docs, release assets). |
| **Phase CI/CD harden** | Open | Scorecard, CodeQL, security scan, SBOM, race, CODEOWNERS. See [CI_HARDENING.md](CI_HARDENING.md). |
| **CNCF Sandbox prep** | Open | Process readiness only (not product features). See [CNCF_SANDBOX_APPLICATION_OUTLINE.md](CNCF_SANDBOX_APPLICATION_OUTLINE.md). |
| **Future deferred product** | Open | Signatures, Fluid, SaaS, etc. Park only. |

## Rules (follow these)

1. **One milestone per issue and PR.** Set it when you open the issue, not at merge time.
2. **Do not mix scopes.** Active phase work stays on the active milestone. Deferred product ideas go to Future.
3. **Exit criteria live in the milestone description.** Close the milestone only when those criteria are met.
4. **Suggested work order right now**
   1. **Phase CI/CD harden** — supply-chain / contributor CI ([CI_HARDENING.md](CI_HARDENING.md))
   2. **CNCF Sandbox prep** — MAINTAINERS, governance, roadmap, application pack ([CNCF_SANDBOX_APPLICATION_OUTLINE.md](CNCF_SANDBOX_APPLICATION_OUTLINE.md)); **do not apply** until critical checklist is green
   3. Future product only after README scope bump (§14)
5. **Future milestone is a parking lot.** Example: [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149) signatures.
6. **Labels still matter** (`go`, `ci`, `SPEC-QUESTION`, …). Milestone answers *when/why*; labels answer *kind of work*.

## How to assign (maintainers and contributors)

```bash
# Issue
gh issue edit NNN --milestone "CNCF Sandbox prep"

# Pull request
gh pr edit NNN --milestone "CNCF Sandbox prep"
```

In the GitHub UI: issue/PR sidebar → Milestone.

## Related docs

- Master spec phases: root `README.md` §5
- Phase 1A inventory: [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md)
- Phase 1B program: epic [#113](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/113)
- Phase 1C status: [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md)
- CI hardening: [CI_HARDENING.md](CI_HARDENING.md)
- CNCF Sandbox prep: [CNCF_SANDBOX_APPLICATION_OUTLINE.md](CNCF_SANDBOX_APPLICATION_OUTLINE.md)
- Release process: [RELEASE.md](RELEASE.md)
