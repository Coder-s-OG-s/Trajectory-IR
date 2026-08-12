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
| **Phase 1C harden and adopt** | Open | Gates when plan allows, release proof, live Docker matrix, adoption polish. Epic [#151](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/151). |
| **Future deferred product** | Open | Signatures, Fluid, SaaS, etc. Park only. |

## Rules (follow these)

1. **One milestone per issue and PR.** Set it when you open the issue, not at merge time.
2. **Do not mix scopes.** Phase 1C harden work stays on Phase 1C. Deferred product ideas go to Future, not Phase 1C.
3. **Exit criteria live in the milestone description.** Close the milestone only when those criteria are met (and the matching release or epic is done).
4. **Suggested work order right now (Phase 1C)**
   1. Maintainer merge policy while #146 blocked ([#152](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/152))
   2. Docker live matrix docs/compose ([#154](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/154))
   3. Status docs keep pace ([#155](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/155))
   4. First-success Go path audit ([#156](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/156))
   5. Next `v*` tag: prove Release attaches wheel/sdist ([#153](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/153))
   6. Branch protection when public or paid plan allows ([#146](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/146))
5. **Future milestone is a parking lot.** Implementing from Future needs a README scope bump (§14) and a new active milestone or epic first. Example: [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149) signatures.
6. **Labels still matter** (`go`, `phase-1c`, `SPEC-QUESTION`, …). Milestone answers *when/why*; labels answer *kind of work*.

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
- Phase 1C status: [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md)
- Phase 1C epic: [#151](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/151)
- Release process: [RELEASE.md](RELEASE.md)
- Live Docker stack: [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md)
