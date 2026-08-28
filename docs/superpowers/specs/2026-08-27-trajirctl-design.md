# trajirctl: an operator CLI for Trajectory IR

## 0. Purpose

Trajectory IR currently exposes operational reads/writes (status, `.tir`
export/import, signature verification) only through the MCP server
(`go/cmd/trajir-mcp`), which is designed for an AI agent to call, not a human
at a terminal. This spec designs `trajirctl`, a standalone command-line tool
that wraps the same underlying Go SDK calls for direct human/script use —
the kubectl/tctl-style tool referenced in prior discussion.

This spec covers two things:
1. A small amount of prep work in **this** repo (Trajectory IR) that
   `trajirctl` depends on.
2. The design of `trajirctl` itself, which will live in a **new, separate
   repository** and is not implemented as part of this repo.

## 1. Scope

**In scope for v1:**
- Local-workdir operations only: point `trajirctl` at a directory containing
  `nodes.sqlite` (+ `memo.sqlite`) and a tenant/trajectory id, same as the
  MCP tools require today.
- Commands: `status`, `export`, `import`, `verify`, `nodes list`, `nodes show`.
- Human-readable text output by default, `--json` for scripting.

**Explicitly deferred (not v1, not designed here):**
- Any command that talks to a remote/deployed backend (Postgres, Temporal,
  a hosted API). Trajectory IR has no server API for trajectory operations
  today to design a remote command against — when one exists, remote support
  is added as new commands, not a rewrite of these.
- A `Backend` interface abstracting local vs. remote. Rejected as premature
  (see §4).

## 2. Prep work in this repo (Trajectory IR)

Two small, additive changes land here before `trajirctl`'s v1 is buildable:

### 2.1 Extract shared workdir-resolution helpers

`go/trajir/mcp/tools.go` has two unexported helpers used by every MCP tool:
`requireBoundedWorkDir` (resolves + validates a workdir path stays within
`TRAJIR_MCP_ROOT`) and `workdirSQLitePaths` (derives `nodes.sqlite` /
`memo.sqlite` paths from a workdir). `trajirctl` needs the same logic and
should not reimplement it.

Action: move both functions into a new importable package,
`go/trajir/workdir`. Update `go/trajir/mcp/tools.go` to call the extracted
package instead of its own copies. No behavior change to `trajir-mcp`.

### 2.2 Tag the Go module for subdirectory semver resolution

The Go module lives at `go/` (module path
`github.com/Coder-s-OG-s/Trajectory-IR/go`). Per Go's convention for a
module in a subdirectory, a consumer can only depend on a proper semver
version if the tag is prefixed with the subdirectory: `go/vX.Y.Z`. Per
`docs/RELEASE.md`'s own version table, this repo has never cut such a tag —
existing tags (`v0.1.0`..`v0.2.1`) are root-level release tags, not module
version tags.

Action: after §2.1 lands on `main`, cut a new patch release following the
existing process in `docs/RELEASE.md` (version bump, CHANGELOG entry, tag).
Tag both the existing root convention and the subdirectory convention at the
same commit, e.g. `v0.2.2` and `go/v0.2.2`, so `trajirctl` can
`go get github.com/Coder-s-OG-s/Trajectory-IR/go@v0.2.2`.

Per `docs/RELEASE.md`: do not move an already-pushed tag. This tag is cut
once §2.1 is merged and `main` is green.

## 3. trajirctl repository structure

New repo (name/location TBD — GitHub org owned by the user), single Go
module:

```
trajirctl/
  go.mod              # requires github.com/Coder-s-OG-s/Trajectory-IR/go go/vX.Y.Z
  main.go              # subcommand dispatch (os.Args[1])
  cmd/
    status.go
    export.go
    import.go
    verify.go
    nodes.go            # list, show
  internal/
    workdir.go           # flag/env resolution wrapper around trajir/workdir
    output.go            # text vs --json rendering
  cmd_test.go            # builds a fixture nodes.sqlite via the SDK, runs each command
```

## 4. Approach: thin, concrete SDK wrapper (no backend abstraction)

Every command calls directly into `go/trajir/{client,log,tir}` and the new
`go/trajir/workdir` package — the same functions `trajir-mcp`'s tools call.
No `Backend` interface, no local/remote polymorphism.

Rejected alternative: introduce a `Backend` interface
(`LocalBackend`/`RemoteBackend`) now, so every command routes through it in
anticipation of future remote support. Rejected because Trajectory IR has no
remote trajectory-operations API today — designing an interface with only
one real implementation to validate it against means guessing its shape.
When remote support is actually built, the interface gets extracted then,
against a second real implementation, not speculatively now. This keeps v1
minimal and avoids carrying an abstraction that may not fit the eventual
remote API's actual constraints.

## 5. Command surface

```
trajirctl status   --workdir DIR --tenant ID --trajectory ID [--json]
trajirctl export   --workdir DIR --tenant ID --trajectory ID --dest PATH [--mode thin|fat] [--json]
trajirctl import   --workdir DIR --tenant ID --src PATH [--json]
trajirctl verify   --src PATH [--json]
trajirctl nodes list --workdir DIR --tenant ID --trajectory ID [--json]
trajirctl nodes show --workdir DIR --tenant ID --trajectory ID --id NODE_ID [--json]
```

`status`, `export`, `import`, `verify` are a 1:1 translation of the four
existing MCP tools (`trajectory_status`, `trajectory_export_tir`,
`trajectory_import_tir`, `trajectory_verify_signature`) onto a CLI surface.
`nodes list`/`nodes show` are new — the MCP surface deliberately doesn't
expose per-node browsing; this is the terminal-native equivalent of what was
done by hand with `sqlite3` during trajectory inspection (see prior K8s
crash/resume verification).

### Workdir resolution

Every command's `--workdir`, `--tenant`, `--trajectory` flags fall back to
`TRAJIR_WORKDIR` / `TRAJIR_TENANT` / `TRAJIR_TRAJECTORY` env vars when unset;
flag wins if both are present. `--workdir` defaults to `.` if neither flag
nor env var is set.

### CLI framework: stdlib `flag`, not Cobra

`go/cmd/crashagent` and `go/cmd/trajir-mcp` both use stdlib `flag` directly;
no Go binary in this project uses a CLI framework. `trajirctl`'s command set
is small and flat (no nested trees beyond `nodes list/show`), so Cobra's
main benefits (generated `--help` trees, shell completion) aren't worth a
new dependency for six commands. Each subcommand gets its own
`flag.NewFlagSet`, dispatched from `os.Args[1]` in `main.go`.

## 6. Data flow

1. `main.go` dispatches on `os.Args[1]` to the matching `run*` function.
2. `internal/workdir.go` resolves workdir/tenant/trajectory (flag → env →
   default) and calls `go/trajir/workdir` to validate the path and derive
   `nodes.sqlite`/`memo.sqlite` paths.
3. The command opens the trajectory via `client.OpenTrajectory` and calls
   straight into `go/trajir/{tir,log}` — `tir.Export`, `tir.Import`,
   `tir.VerifySignature`, `NodeLog.ListNodes`, etc. No logic is
   reimplemented; every command is a thin translation from flags to an
   existing SDK call.
4. `internal/output.go` renders the result either as human-readable text
   (default) or JSON (`--json`), using the same structured shape the MCP
   tools already return.

## 7. Error handling

SDK errors are wrapped with the command name and printed to stderr:
`trajirctl status: workdir not found: ...`. Exit code 1 on any error, 0 on
success. No panics escape `main`.

## 8. Output format

Text by default (readable in a terminal); `--json` emits the same
structured data machine-readably. Kept in v1 despite being outside the
strict "smallest possible v1" cut, because output format becomes a de facto
compatibility surface once anyone scripts against it — cheaper to include
now (the underlying data is already structured) than to retrofit once
text-parsing scripts exist.

## 9. Testing

`trajirctl`'s test suite is self-contained in the new repo: `cmd_test.go`
builds a fixture `nodes.sqlite` in a temp directory by calling the SDK
directly (open a trajectory, append a few nodes, seal one), then runs each
command's `run*` function against that fixture and asserts on output. No
dependency on this repo's Python test fixtures or `test/e2e` infrastructure.

## 10. Non-goals (this spec)

- Remote/deployed-backend commands (§1).
- A `Backend` abstraction (§4).
- Any change to `trajir-mcp`'s external behavior (the extraction in §2.1 is
  purely internal).
- Distribution mechanism (goreleaser, Homebrew tap, etc.) — not decided
  here, deferred to the new repo's own setup once it exists.

## 11. Open items for the new repo (not blocking this spec)

- Exact repo name and GitHub location.
- Distribution/install method.

These don't affect the prep work in this repo (§2) and can be decided when
the new repo is created.
