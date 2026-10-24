# One step

Trajectory IR sits beside the agent you already run. One command builds the verify binary, points Claude Code at project-local hooks, and checks one pack.

```powershell
pwsh -File scripts\install_claude.ps1
```

```bash
bash scripts/install_claude.sh
```

Verify a pack you exported. Install does not create `pack.tir`.

```bash
./go/trajir.exe verify path/to/pack.tir
```

The shell script uses `TRAJIR_PYTHON`, then `.venv/bin/python` (or `.venv/Scripts/python.exe`), then `python3`, then `python`. PowerShell uses `TRAJIR_PYTHON`, then the repo `.venv`, then `python`.

The PreToolUse hook seals the decision. A seal error on this call exits 2 and the host skips that tool. The next call is a new process. PostToolUse records the result after the tool has run. That hook is best-effort: a diary error is swallowed and the process still exits 0. PostToolUseFailure records a failed tool and clears its pending row. If the host denies the tool after the seal, neither post hook runs, the pending row stays, and the next tool is a new step. The hook writes the log directly and does not start DBOS. On this path the tool-call row is stored after the host tool, so it records the call. The LangGraph wrap is the path that gates the tool body. `trajir verify` checks the pack offline.

A LangGraph process uses the wrap that is already on main:

```python
guard = TrajIRToolGuard(tenant_id="demo", trajectory_id="lg-live", work_dir=".local-evidence/lg-live")
tools = ToolNode(my_tools, wrap_tool_call=guard.wrap_tool_call)
```

A Google ADK agent uses the same seal, with callbacks instead of a hook:

```python
guard = AdkToolGuard(tenant_id="demo", trajectory_id="adk-live", work_dir=".local-evidence/adk-live")
agent = Agent(tools=my_tools, before_tool_callback=guard.before_tool_callback, after_tool_callback=guard.after_tool_callback)
```

An unknown tool, and any tool named Bash, is `NON_IDEMPOTENT_WRITE`. A crash after that call starts does not run it again from this process.

Token numbers on the console are `ceil(chars/4)` estimates. They are not a provider invoice. The adopter list is empty. This is not exactly-once, not a process sandbox, and not Sigstore. OpenTelemetry labels are copied only after the in-process seal.

The Article 12 field map is [Article 12 field map](article-12-field-map.md). It names the pack fields. It is not a compliance claim.
