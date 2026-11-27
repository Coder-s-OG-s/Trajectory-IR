---
name: trajir-runs
description: List, export and verify the tool calls Trajectory IR recorded for this project. Use when the user asks what the agent did, wants evidence of a session, wants a .tir package, or wants to check one.
---

# Recorded runs

The Trajectory IR plugin seals every tool call before it runs and records its
result. Each Claude Code session is one trajectory named `claude-<session id>`,
stored under `.trajir/runs/claude/` in the project.

## List sessions

```bash
trajir runs
```

The newest session is usually the current one. The `STEPS` column counts
sealed tool calls.

## Export a session

```bash
trajir export --host claude --trajectory <trajectory id> -o <name>.tir
```

Use `--mode fat` to include artifact bytes. The export also appears on the
dashboard under the run's packages.

## Verify a package

```bash
trajir verify <name>.tir
```

`OK` means every node hash checks out and every tool call was sealed before
it ran. Any finding is printed one per line. Report findings as they are.
Don't explain them away.

## If trajir is missing

Tell the user to install a release binary from
https://github.com/Coder-s-OG-s/Trajectory-IR/releases and put it on PATH,
or to set `TRAJIR_BIN`. Until then the plugin blocks tool calls, because
it cannot record them.
