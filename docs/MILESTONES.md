# Milestones (how we plan work)

GitHub **Milestones** are the planning spine for Trajectory IR. Issues and PRs
should always land on exactly one open milestone (or the closed Phase 1A
history bucket when you are only filing retrospective notes).

Board: [Milestones](https://github.com/Coder-s-OG-s/Trajectory-IR/milestones)

## Active and historical milestones

| Milestone | State | Purpose |
|-----------|--------|---------|
| **Phase 1A library baseline (v0.1.0)** | Closed | First library tag. Historical only. |
| **v0.1.1 post 0.1.0 patch** | Closed | Absorbed into history; public cut advanced to v0.2.0. |
| **Phase 1B Go primary SDK** | Closed | Complete; released as **v0.2.0**. |
| **Phase 1C harden and adopt** | Open | Gates when plan allows, release automation, polish. |
| **Future deferred product** | Open | Signatures, Fluid, SaaS, etc. Park only. |

## Rules (follow these)

1. **One milestone per issue and PR.** Set it when you open the issue, not at merge time.
2. **Do not mix scopes.** Phase 1C harden work stays on Phase 1C. Deferred product ideas go to Future, not Phase 1C.
3. **Exit criteria live in the milestone description.** Close the milestone only when those criteria are met (and the matching release or epic is done).
4. **Suggested work order right now**
   1. **Phase 1C**: branch protection when plan allows (#146), tag release workflow (#147), status docs (#148).
   2. Keep product non goals under **Future deferred product**.
5. **Future milestone is a parking lot.** Implementing from Future needs a README scope bump (§14) and a new active milestone or epic first.
6. **Labels still matter** (`go`, `phase-1b`, `SPEC-QUESTION`, …). Milestone answers *when/why*; labels answer *kind of work*.

## How to assign (maintainers and contributors)

```bash
# Issue
gh issue edit NNN --milestone "Phase 1C harden and adopt"

# Pull request
gh pr edit NNN --milestone "Phase 1C harden and adopt"
```

In the GitHub UI: issue/PR sidebar → Milestone.

## Related docs

- Master spec phases: root `README.md` §5
- Phase 1A inventory: [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md)
- Phase 1B program: epic [#113](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/113)
- Release process: [RELEASE.md](RELEASE.md)
