---
name: trajir-dashboard
description: Open the Trajectory IR dashboard (trajir console) to see recorded agent sessions, seals, tool calls, exports and audits. Use when the user asks to see the dashboard, the console, or what was recorded.
---

# Dashboard

Every session's tool calls are sent to the dashboard data folder
(`~/.trajir/console-data` unless `TRAJIR_CONSOLE_DATA` says otherwise), for
every agent and project on this machine.

1. Check whether it is already up: `curl -s http://127.0.0.1:8787/healthz`.
2. If something is already listening, use that one. Do not start a second console.
3. If it is not up, start it in the background: `trajir console`. It serves
   `http://127.0.0.1:8787/` on this machine only.
4. Give the user the link. For one session use
   `http://127.0.0.1:8787/?id=<trajectory id>`. `trajir runs` lists the ids.

The dashboard re-reads its data on every request, so new tool calls show up
while it runs. Search for `grok-cli` to list only Grok CLI runs.

If the user set `TRAJIR_CONSOLE_SINK=off`, nothing is sent and the dashboard
stays empty. That is their choice. Say so rather than changing it.
