# Agent plugins: record every tool call

Trajectory IR can record every tool call a coding agent makes: Claude Code,
Codex, Gemini CLI, Cursor and Copilot CLI. Each call is sealed before it runs
and its result is recorded after, one trajectory per agent session. Every
session shows up on the dashboard, and you can export it as a `.tir` that
anyone can check with `trajir verify`.

Everything runs on your machine. Nothing is uploaded.

## 1. Install trajir

Download the archive for your OS from the
[releases page](https://github.com/Coder-s-OG-s/Trajectory-IR/releases), check
it against `SHA256SUMS`, and put `trajir` (or `trajir.exe`) on your `PATH`.
Each archive also carries a build provenance attestation:

```bash
gh attestation verify trajir_<tag>_linux_amd64.tar.gz --repo Coder-s-OG-s/Trajectory-IR
```

With Go 1.26 or newer you can build it instead:

```bash
go install github.com/Coder-s-OG-s/Trajectory-IR/go/cmd/trajir@latest
```

Check it with `trajir version`.

## 2. Attach your agent

| Agent | How | Command |
|---|---|---|
| Claude Code | Plugin | `/plugin marketplace add Coder-s-OG-s/Trajectory-IR`, then `/plugin install trajectory-ir@trajectory-ir` |
| Codex | Project hooks | `trajir install codex`, then trust the hooks in `/hooks` |
| Gemini CLI | Project settings | `trajir install gemini` |
| Cursor | Project hooks | `trajir install cursor` |
| Copilot CLI | Repo hooks | `trajir install copilot` |

`trajir install <agent>` writes into the current project. Pass `--target DIR`
for another one, `--user` for your user-level config, and `--dry-run` to see
the files first. It merges into existing files and keeps your own hooks. A
file that is not a JSON object stops the install before anything is written.
Claude Code can also use `trajir install claude` if you would rather not use
the plugin.

The hooks run `trajir hook <phase> --host <agent> --owner <scope>`. The owner
(`plugin`, `project` or `user`) keeps two registrations of the same agent from
blocking each other: only the same owner asking again for an unfinished call
counts as a replay. When `trajir` is on your
`PATH`, the config says `trajir`. Otherwise it holds the absolute path of the
binary you ran, so don't commit that file.

> [!NOTE]
> Gemini CLI's docs say it was replaced by Antigravity CLI on 2026-06-18. The
> Gemini hooks still work where Gemini CLI does. There is no Gemini extension
> bundle.

## 3. Use the agent

Nothing changes in how you work. For each tool call:

1. **Before it runs**, the hook writes `PROJECT_CONTEXT` and a sealed
   `DECISION` naming the tool and its arguments.
2. **After it runs**, the hook writes `TOOL_CALL`, then `TOOL_RESULT` (or
   `ABORT` with `HOST_REPORTED_FAILURE` when the agent reports a failure), then
   `COMMIT_STEP`.

If the seal cannot be written (`trajir` missing, a work dir that is not
writable, a bad `effects.json`), **the tool call is blocked**, and the agent is
told why. An unrecorded tool call is exactly what this plugin exists to stop.
A failure after the tool ran only prints a warning, so the agent is never asked
to run something again.

If the same tool call comes back after a crash, before its result was
recorded, it is blocked when its effect class is gated. That is the
block-and-gate rule (R02). A person decides.

### Where runs live

```text
<project>/.trajir/
  .gitignore          "*", written the first time, so runs are not committed
  runs/<agent>/
    nodes.sqlite      the NodeLog
    hook_state.*.json which calls are sealed but not finished yet
  effects.json        optional, yours (below)
```

The trajectory id is `<agent>-<session id>`, for example `claude-8f2c...`.

| Variable | Default | Meaning |
|---|---|---|
| `TRAJIR_WORK_DIR` | `<project>/.trajir/runs/<agent>` | Where the NodeLog goes |
| `TRAJIR_TENANT_ID` | `local` | Tenant id |
| `TRAJIR_TRAJECTORY_ID` | `<agent>-<session>` | Pin one trajectory id |
| `TRAJIR_BIN` | `trajir` on `PATH` | Binary the Claude Code plugin runs |
| `TRAJIR_CONSOLE_SINK` | file | `off` sends nothing to the dashboard. `http` posts to `TRAJIR_CONSOLE_URL` |
| `TRAJIR_CONSOLE_DATA` | `~/.trajir/console-data` | Dashboard data folder |

### Effect classes

Each call gets an effect class, which decides what happens on a replay. It
never blocks a first run. Each agent's read-only built-ins (Claude Code
`Read`, `Grep`, `Glob`, `WebFetch`...; Gemini `read_file`, `glob`...; Cursor
`Read`, `Grep`) are `READ_ONLY`. MCP tools use their annotations. Everything
else, including shell and file writes, is `NON_IDEMPOTENT_WRITE`.

Override per project in `.trajir/effects.json`:

```json
{ "tools": { "mcp__deploy__preview": "IDEMPOTENT_WRITE", "Read": "NON_IDEMPOTENT_WRITE" } }
```

Open-world tools (`Bash`, `shell`, `python`, `sql`, `browser`...) cannot be
made safer here. Trying is an error that blocks tool calls until you fix the
file.

`effects.json` belongs to the project, so a repo you clone can ship one. It
can only change replay gating for tools that are not open-world. It never
skips a seal. Read it before you trust a cloned repo's replay behavior.

### Agents without a call id

Gemini CLI and Copilot CLI send no tool call id, and Codex may leave it out
after the call. Their post hook is matched to the oldest open call with the
same tool name and arguments. A repeat of such a call after a crash cannot be
told apart from a new call, so it is recorded as a new step and not blocked.
With a project and a user install both on, the second hook joins the call the
first one sealed, so each call is still recorded once.

Codex has no failure hook. A failed command is recorded as its result.

## 4. See it on the dashboard

```bash
trajir console
```

Open <http://127.0.0.1:8787/>. Every session from every agent and project is
there, with its agent shown next to it. Type `codex` or `claude-code` in the
run search to list one agent's runs. The Evidence tab shows the
seal-before-execute check, each tool call with its effect class and
idempotency key, and audit results. New calls show up while it runs.

The Claude Code plugin adds a `trajir-dashboard` skill, so you can ask Claude
to open it.

## 5. Export and verify

```bash
trajir runs                                     # sessions in this project
trajir export --host claude --trajectory claude-8f2c... -o session.tir
trajir verify session.tir
```

Both export and verify show up on the dashboard next to the run, and the
exported package is copied into the dashboard's packages panel.

## Uninstall

- Claude Code: `/plugin uninstall trajectory-ir`.
- Others: delete the `trajir` entries from `.codex/hooks.json`,
  `.gemini/settings.json`, `.cursor/hooks.json`, or delete
  `.github/hooks/trajir.json`. The MCP entry is named `trajectory-ir`.
- Recorded runs stay in `.trajir/` until you delete the folder.

## Manual smoke check (before a release)

For each agent, in a scratch git repo with a fresh terminal:

1. Attach it (step 2) and start a session.
2. Ask it to read a file and run `echo ok`.
3. `trajir runs` lists one session with 2 steps.
4. `trajir export ...` then `trajir verify` prints `OK`.
5. `trajir console` shows the run under that agent with 2 seals.
6. Rename the binary, start a session, and ask for a tool call. It is
   blocked, and the message says how to install `trajir`.
