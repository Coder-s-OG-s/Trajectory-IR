# One step

Trajectory IR sits beside the agent you already run. One command builds the verify binary, points Claude Code at project-local hooks, and checks one pack.

```powershell
pwsh -File scripts\install_claude.ps1
```

```bash
./go/trajir.exe verify pack.tir
```

The PreToolUse hook seals the decision. The host then runs the tool. The PostToolUse hook records the result. `trajir verify` checks the pack offline.

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

The Article 12 field map is [ARTICLE_12_FIELD_MAP.md](../../docs/ARTICLE_12_FIELD_MAP.md). It names the pack fields. It is not a compliance claim.
