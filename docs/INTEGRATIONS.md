# Integrations: MCP (primary)

Trajectory IR exposes agent hosts through a **Model Context Protocol (MCP)**
server. Host-specific plugins (Claude Code, Cursor configs) live in a **separate
repo** later and only package config; they do not reimplement IR semantics.

## Architecture

```text
AI host (Claude Code, Cursor, …)
        │  MCP stdio
        ▼
  trajir-mcp  (this repository, Go)
        │
        ▼
  trajir/client + trajir/tir + NodeLog
```

| Layer | Repository | Role |
|-------|------------|------|
| IR core + MCP server | **Trajectory-IR** (this repo) | Semantics, tools, releases |
| Host plugins / configs | Separate repo (later) | Install snippets, skills, UI only |

## Binary

```bash
cd go
go build -o trajir-mcp ./cmd/trajir-mcp
# or: go run ./cmd/trajir-mcp
```

The server speaks MCP over **stdio** until the client disconnects.

## Tools (v0.1.0)

| Tool | Purpose |
|------|---------|
| `trajectory_status` | Node counts by kind, seal count, paths for a workdir trajectory |
| `trajectory_export_tir` | Export thin (default) or fat `.tir` package |
| `trajectory_import_tir` | Load + hash-verify a `.tir` (no NodeLog write) |
| `trajectory_verify_signature` | Optional `trajir-pkg-sig-v1` verify; unsigned OK unless `require_signature` |

### Common arguments

- `work_dir` — directory holding `nodes.sqlite` (and `memo.sqlite` when opening a client)
- `tenant_id` / `trajectory_id` — IR identity
- `path` / `dest` — filesystem paths for packages

## Example host config (Claude Code / Cursor style)

```json
{
  "mcpServers": {
    "trajectory-ir": {
      "command": "/absolute/path/to/trajir-mcp",
      "args": []
    }
  }
}
```

Use an absolute path to the built binary. Do not put private signing keys in
config files; use environment variables when signing tools are added.

### Workspace confinement

| Variable | Meaning |
|----------|---------|
| `TRAJIR_MCP_ROOT` | Approved workspace root. All `work_dir`, `dest`, and `path` values must resolve under this directory. When unset, the process **current working directory** is the root. |

This bounds prompt-injected tool paths (CWE-73). Host configs should set
`TRAJIR_MCP_ROOT` to the project directory and start the server with that cwd.

`nodes.sqlite` and `memo.sqlite` under a work dir must not be **symlinks**
(CWE-59). Symlinked leaves that point outside the workspace are rejected before open.

```json
{
  "mcpServers": {
    "trajectory-ir": {
      "command": "/absolute/path/to/trajir-mcp",
      "args": [],
      "env": {
        "TRAJIR_MCP_ROOT": "/absolute/path/to/project"
      }
    }
  }
}
```

## Non-goals

- Agent graph orchestration (LangGraph-class)
- Replacing Temporal / DBOS crash engines
- Multi-tenant SaaS control plane
- Fluid productization

## Security notes

- Treat untrusted `.tir` paths carefully; import verifies node hashes/seals.
- Signature verify fails closed on tamper; unsigned packages remain valid by default.
- Prefer fixing signature P0 issues (#186, #188) before relying on signed packages in production hosts.

## Related

- Package format: root `README.md` §9 / §9.1
- Go client: `go/trajir/client`
- Implementation: `go/trajir/mcp`, `go/cmd/trajir-mcp`
