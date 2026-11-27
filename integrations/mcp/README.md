# Trajectory IR MCP server

Implementation lives in the Go module:

| Path | Role |
|------|------|
| [`go/cmd/trajir-mcp`](../../go/cmd/trajir-mcp) | Binary entrypoint (stdio) |
| [`go/trajir/mcp`](../../go/trajir/mcp) | Server + tools |
| [`docs/INTEGRATIONS.md`](../../docs/INTEGRATIONS.md) | Contract and non-goals |

## Quick start

```bash
cd go
go build -o trajir-mcp ./cmd/trajir-mcp
./trajir-mcp   # waits on stdio for an MCP host
```

## Host plugins

Host packaging lives in this repo: the Claude Code plugin is under
[`plugins/claude-code`](../../plugins/claude-code), and `trajir install <agent>`
writes hooks and this MCP server for Codex, Gemini CLI, Cursor and Copilot CLI.
See [docs/AGENT_PLUGINS.md](../../docs/AGENT_PLUGINS.md). `trajir mcp` runs the
same server as `trajir-mcp`.

## Tools

See [docs/INTEGRATIONS.md](../../docs/INTEGRATIONS.md).
