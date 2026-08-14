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

Host-specific packaging (Claude Code skill files, Cursor config bundles) will
live in a **separate repository**. This folder only documents the in-tree MCP
surface.

## Tools

See [docs/INTEGRATIONS.md](../../docs/INTEGRATIONS.md).
