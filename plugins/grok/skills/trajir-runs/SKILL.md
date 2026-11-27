---
name: trajir-runs
description: List, export and verify the tool calls Trajectory IR recorded for this project. Use when the user asks what the agent did, wants evidence of a session, wants a .tir package, or wants to check one.
---

# Recorded runs

The Trajectory IR plugin seals every Grok CLI tool call before it runs and records its result. Each session is one trajectory named `grok-<session id>`, stored under `.trajir/runs/grok/` in the project.

A subagent does not get its own SessionStart. Tool calls still seal when PreToolUse runs. If that call carries a different session id, it is its own trajectory.

## List sessions

```bash
trajir runs
```

The newest session is usually the current one. The `STEPS` column counts sealed tool calls.

## Export a session

```bash
trajir export --host grok --trajectory <trajectory id> -o <name>.tir
```

Use `--mode fat` to include artifact bytes. The export also appears on the dashboard under the run's packages.

## Verify a package

```bash
trajir verify <name>.tir
```

`OK` means every node hash checks out and every tool call was sealed before it ran. Any finding is printed one per line. Report findings as they are. Don't explain them away.

## If trajir is missing

Tell the user to install a release binary from
https://github.com/Coder-s-OG-s/Trajectory-IR/releases and put `trajir` on PATH.
The pre hook is fail closed, so a missing `trajir` blocks the tool call. It does not run unrecorded.
