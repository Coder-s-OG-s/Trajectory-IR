# One step

Trajectory IR sits beside the agent you already run. One command builds the verify binary, points Claude Code at project-local hooks, and checks one pack.

```powershell
pwsh -File scripts\install_claude.ps1
pwsh -File scripts\install_claude.ps1 -Target C:\path\to\your\project
```

```bash
./scripts/install_claude.sh
./scripts/install_claude.sh --target /path/to/your/project
```

The shell script uses `TRAJIR_PYTHON`, then `.venv/bin/python`, then `.venv/Scripts/python.exe` when that file exists, then `python3`, then `python`. PowerShell uses `TRAJIR_PYTHON`, then the repo `.venv`, then `python`.

The installer smoke-checks its own pack. That smoke does not run the hook scripts. Verify a pack you exported. Install does not create `pack.tir`.

```bash
./go/trajir.exe verify path/to/pack.tir
```

`--target` writes the config into that project. The hook scripts stay in this install. The user profile is not edited. The same command writes Cursor, VS Code, Gemini CLI, and Codex project MCP config. Those MCP tools are `trajectory_status`, `trajectory_export_tir`, `trajectory_import_tir`, and `trajectory_verify_signature`. They do not seal the host's own tools.

If nothing is listening on `127.0.0.1:8787`, the installer starts `trajir-console` and waits until it accepts a connection. The console data directory is `<target>/.local-evidence/console-data`. If the port is taken, it leaves that process alone. If the new process dies, the installer says so and exits 1. Pass `--skip-console` to skip the start. There is no second console.

The first PreToolUse seals the decision and exits 0 so the host can run the tool. The hook command carries the owner word `project`. The same owner asking again while that call is still pending exits 2 for `NON_IDEMPOTENT_WRITE` and for a row with no effect class. That is a crash before PostToolUse. A different owner on that pending call exits 0. A second registration of this hook must use a different owner word. The same word looks like a retry and blocks the first run. After the result is recorded, a later PreToolUse for that `NON_IDEMPOTENT_WRITE` id exits 2. The hook does not call the tool and does not start DBOS. PostToolUse records the result. That write is best-effort and happens after the tool has run. If it fails, PostToolUse still exits 0 so the host is not asked to run the tool again. PostToolUseFailure records a failed tool and clears its pending row. If the host denies the tool after the seal, neither post hook runs. The pending row stays. There is no cleanup command. The next tool is a new step. A missing `TRAJIR_IDENTITY_FILE` exits 2 and names `scripts/install_claude.ps1` or `scripts/install_claude.sh`. If Python itself never starts, the host can still run the tool. Once the launcher process is up, an import error on PreToolUse exits 2.

The installer adds `.local-evidence/`, the project Claude and MCP config files, and `.trajir/*.py` to the target `.gitignore` when those lines are missing.

LangGraph still gates inside the tool wrap. The first body runs. A later resume does not run that gated body again.

```python
guard = TrajIRToolGuard(tenant_id="demo", trajectory_id="lg-live", work_dir=".local-evidence/lg-live")
tools = ToolNode(my_tools, wrap_tool_call=guard.wrap_tool_call)
```

Google ADK uses callbacks. They seal and record. They do not call the tool. A missing call id raises ValueError. This page does not claim a live ADK agent has been run.

```python
guard = AdkToolGuard(tenant_id="demo", trajectory_id="adk-live", work_dir=".local-evidence/adk-live")
agent = Agent(tools=my_tools, before_tool_callback=guard.before_tool_callback, after_tool_callback=guard.after_tool_callback)
```

Snippets for those two calls are written to `.trajir/` in the target project. Importing a snippet does not import LangGraph, ADK, or DBOS.

An unknown tool, and any tool named Bash, is `NON_IDEMPOTENT_WRITE`.

Token numbers on the console are `ceil(chars/4)` estimates. They are not a provider invoice. The adopter list is empty. This is not exactly-once, not a process sandbox, and not Sigstore. OpenTelemetry labels are copied only after the in-process seal.

The Article 12 field map is [Article 12 field map](article-12-field-map.md). It names the pack fields. It is not a compliance claim.

This page is published when GitHub Actions builds this docs folder. The link above stays inside this folder.
