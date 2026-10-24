# Trajectory IR

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/14075/badge)](https://www.bestpractices.dev/projects/14075)
[![CI](https://github.com/Coder-s-OG-s/Trajectory-IR/actions/workflows/ci.yml/badge.svg)](https://github.com/Coder-s-OG-s/Trajectory-IR/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-primary_SDK-00ADD8?logo=go&logoColor=white)](go/QUICKSTART.md)
[![Python](https://img.shields.io/badge/Python-reference_port-3776AB?logo=python&logoColor=white)](QUICKSTART.md)
[![Spec](https://img.shields.io/badge/spec-v0.2--draft-informational)](docs/MASTER_SPECIFICATION.md)

**Portable, hash-verifiable intermediate representation for agent execution trajectories.**

A **flight recorder** for agent runs: typed nodes, a sealed model plan, fail-closed effect classes, and a runtime-independent `.tir` package anyone can verify by content hash.

It sits **on top of** durable execution engines (Temporal, DBOS, Restate). It does **not** replace them, and it is **not** another agent framework. It does **not** freeze AWS, auctions, or users while you are crashed, and a client-side gate is **not** exactly-once at Stripe.

```text
  Agent host / framework          (Claude Code, LangGraph, ADK, MCP hosts, ...)
            │
            ▼
  ┌─────────────────────────────┐
  │       Trajectory IR         │  seals · effect classes · .tir · sandbox
  └─────────────────────────────┘
            │
            ▼
  Durable backend                 (Temporal · DBOS · Restate)
```

---

## Why it exists

Production agent stacks keep failing the same way. Crash replay is already Temporal/DBOS/Restate's job. The hole they do not fill is a **portable, hash-verifiable record of what the agent actually decided and invoked**, plus agent-shaped policy on top of that record.

| Failure | Who actually solves it | What Trajectory IR adds |
|---|---|---|
| **Crash mid-tool** | Durable backend memoization. **Exactly-once** still needs a server-side idempotency key the remote API honors. | Block-and-gate: at most one automatic attempt from this client. Unknown in-flight calls go to `BLOCKED_NEEDS_GATE` for a human, not a second LLM turn. The seal-derived key is recorded on `TOOL_CALL` so you can *forward* it. |
| **Naive resume** | Do not re-call the model for a sealed step (backend replay + IR seal). | Honest resume of the sealed plan (R01). That freezes **the plan**, not **the world**. If the cluster changed while you were dead, re-observe (`READ_ONLY`) or abort and start a **new** step. Hosts that declared a `WORLD_SNAPSHOT` at seal time can `CheckWorld` and get fail-loud `WORLD_DRIFT`. Do not silently re-infer the sealed one. |
| **Locked history** | Nobody else ships a runtime-independent unit. | Portable thin/fat `.tir` with hash-checked node IDs |
| **Unsafe demos** | Host policy. | Sandbox mode is a **demo/CI effect-class gate** before the tool body (R06). It is not a process sandbox, not an AST of `bash`, and not a security boundary for arbitrary code. |

**One-line pitch:** portable semantics for *what the agent actually did*, on top of engines that already solve crash safety.

### What "IR" means here

This is a **runtime trajectory IR**, not LLVM. You do not compile a prompt into `.tir` ahead of time. Hosts **lower** a live agent step into typed, content-addressed nodes; you can project, redact, graft, verify, and export that trace onto more than one backend. If you wanted a compiler, this is the wrong repo. If you wanted a vendor-neutral flight recorder for agent steps, this is the product.

> Full normative contract: **[docs/MASTER_SPECIFICATION.md](docs/MASTER_SPECIFICATION.md)** (`spec-v0.2-draft`).  
> Short scope card: **[docs/SCOPE_AND_NON_GOALS.md](docs/SCOPE_AND_NON_GOALS.md)**.

---

## Features

- **Sealed decisions** — freeze the model's plan (`DECISION`) before world-changing tools run. The world is not frozen with it.
- **Effect classes** — fail-closed mapping from MCP tool hints, plus `AGENT_SPAWN` and `SENSITIVE`. Open-world primitives (`bash`, `python`, `sql`, browser) stay `NON_IDEMPOTENT_WRITE` unless an operator sets the class on the tool. No command parser.
- **Honest resume** — a sealed step does not re-infer (R01). Re-observe is allowed; re-plan is a **new** step. Optional `WORLD_SNAPSHOT` + `CheckWorld` is fail-loud when the host declared what it depends on.
- **Block-and-gate** — non-idempotent tools are not blindly retried after interruption (R02). At-most-one automatic attempt, not exactly-once in the world.
- **Seal-derived idempotency keys** — length-prefixed `sha256` of domain, tenant, trajectory, step, and seq on every `TOOL_CALL`, exposed to the tool body as CallMeta so you can send `Idempotency-Key`. Never stuffed into Python kwargs. Hosts must forward that string to the remote API. The withdrawn colon key does not resume onto this hash.
- **`.tir` packages** — thin or fat export/import with content-addressed identity; optional `trajir-pkg-sig-v1` signatures
- **Sandbox mode** — demo/CI gate that rejects dangerous effect classes before the tool body. Not a security sandbox.
- **Dual SDK** — **Go primary** (Temporal production backend), **Python reference** (DBOS local profile)
- **Conformance suite** — R01–R11 runnable across languages
- **OpenSSF Best Practices** — project badge and continuous hardening

---

## Quick start (Go — recommended)

**Prerequisites:** Go 1.26.10, Git

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR/go
go test ./...
```

Minimal client step:

```go
tr, err := client.OpenTrajectory("demo", "qs-1", client.Options{WorkDir: dir})
// ...
tr.Project(1, map[string]any{"goal": "hello"})
tr.SealDecision(1, map[string]any{
    "tool_calls": []any{
        map[string]any{"name": "echo", "args": map[string]any{"msg": "hi"}},
    },
})
res, err := tr.ExecTool(1, 2, tool, map[string]any{"msg": "hi"})
fmt.Println(res.Result) // hi
tr.CommitStep(1, 4)
```

Full walkthrough, Temporal notes, and demos: **[go/QUICKSTART.md](go/QUICKSTART.md)**

### Python reference port (optional)

```bash
cd Trajectory-IR
python -m venv .venv
# Windows: .\.venv\Scripts\activate
# Unix:    source .venv/bin/activate
pip install -U pip
pip install -e ".[dev]"
pytest conformance/ -q
```

`.[dev]` includes DBOS. Outside the repo, a base install covers the client, node log, projection, and `.tir` export. `pip install "trajectory-ir[dbos]"` adds the DBOS backend that the default `make_run_step` hooks use.

More: **[QUICKSTART.md](QUICKSTART.md)** · kill-mid-deploy demo: [`examples/kill_mid_deploy/`](examples/kill_mid_deploy/)

---

## One step in front of Claude Code

One command in this checkout builds the verify binary, points Claude Code at project-local hooks, and checks one pack. It does not download Claude. It sets Trajectory IR up in front of the Claude Code session that already uses this worktree.

```powershell
pwsh -File scripts\install_claude.ps1
```

```bash
bash scripts/install_claude.sh
```

PowerShell uses `TRAJIR_PYTHON`, then `.venv\Scripts\python.exe`, then `python`. The shell script uses `TRAJIR_PYTHON`, then `.venv/bin/python` (or `.venv/Scripts/python.exe`), then `python3`, then `python`. That interpreter is what the hook command runs.

Verify a pack you exported. There is no `pack.tir` until something writes one:

```bash
./go/trajir.exe verify path/to/pack.tir
```

What that command already does:

1. Builds `go/trajir.exe` and `go/trajir-mcp.exe`. The names keep the `.exe` suffix on every OS, because `go/trajir` is the package directory.
2. Merges `PreToolUse`, `PostToolUse`, and `PostToolUseFailure` into `.claude/settings.local.json`, and merges the MCP server into `.mcp.json`. Existing JSON objects are kept. Invalid JSON, including a comment or a trailing comma, is left as it is and the command exits 1, so the smoke check does not run. Claude Code asks you to approve the project MCP server before that server starts.
3. Seals the decision in PreToolUse. A seal error on this call exits 2 and Claude skips that tool. The next call is a new process, so a missing tool id or a lock timeout does not block later tools. If the python in the hook command cannot import this repo, the hook crashes with exit 1, Claude treats that as non-blocking, and the tool runs unsealed. Fix the interpreter (`TRAJIR_PYTHON` or `.venv`), delete the `trajectory-ir` hook entries from `.claude/settings.local.json`, then run the installer again. When the seal succeeds, the tool runs and `PostToolUse` records the result. PostToolUse is best-effort: errors are swallowed and the hook exits 0. On this Claude path the tool-call row is stored only after the tool runs. PostToolUseFailure records a failed tool and clears the pending row. If Claude denies the tool after the seal, neither post hook runs. The pending row stays in `.local-evidence/claude-agent/hook_state.json` and the decision stays in the log. The next tool is a new step. Leave that row. There is no cleanup command.
4. Writes the log directly. This path does not start DBOS.
5. Seals and records one local pack through the library, then prints `INSTALL OK`. This does not run the hook scripts, so it does not prove the hook command works.

The user profile at `~/.claude` is not changed. The console on `127.0.0.1:8787` is not started by this script. Hooks stay inside this worktree.

Two in-process wraps are already on main, for hosts that call tools in the same process.

LangGraph is the path that gates the tool body:

```python
from integrations.langgraph import TrajIRToolGuard

guard = TrajIRToolGuard(tenant_id="demo", trajectory_id="lg-live", work_dir=".local-evidence/lg-live")
tools = ToolNode(my_tools, wrap_tool_call=guard.wrap_tool_call)
```

Google ADK uses the same seal, with callbacks. The callbacks do not call the tool:

```python
from integrations.adk import AdkToolGuard

guard = AdkToolGuard(tenant_id="demo", trajectory_id="adk-live", work_dir=".local-evidence/adk-live")
agent = Agent(
    tools=my_tools,
    before_tool_callback=guard.before_tool_callback,
    after_tool_callback=guard.after_tool_callback,
)
```

MCP (`trajectory_status`, `trajectory_export_tir`, `trajectory_import_tir`, `trajectory_verify_signature`) reports status and checks packs. It does not seal the host's own tools. An unknown tool, and any tool named Bash, stays `NON_IDEMPOTENT_WRITE`.

Token numbers on the console are `ceil(chars/4)` estimates, not a provider invoice. The adopter list is empty. This is not exactly-once, not a process sandbox, and not Sigstore.

The short page is [website/docs/one-step.md](website/docs/one-step.md). The Article 12 field map names pack fields. It is not a compliance claim: [docs/ARTICLE_12_FIELD_MAP.md](docs/ARTICLE_12_FIELD_MAP.md).

---

## Who is this for?

| Audience | Start here |
|---|---|
| **Students / newcomers** | This README → [go/QUICKSTART.md](go/QUICKSTART.md) → try sandbox + kill-mid-deploy demos |
| **Claude Code, this checkout** | [One step in front of Claude Code](#one-step-in-front-of-claude-code) |
| **Agent / platform engineers** | [docs/SCOPE_AND_NON_GOALS.md](docs/SCOPE_AND_NON_GOALS.md) → [docs/INTEGRATIONS.md](docs/INTEGRATIONS.md) → wire seals into your host loop |
| **Security / compliance** | Seals + `.tir` (hash-verifiable export); report issues via [SECURITY.md](SECURITY.md). Sandbox is a demo gate, not a security boundary. |
| **Contributors** | [CONTRIBUTING.md](CONTRIBUTING.md) (DCO required) → Phase 1B/1C: **Go first** |
| **AI coding agents** | [docs/MASTER_SPECIFICATION.md](docs/MASTER_SPECIFICATION.md) + [AI_POLICY.md](AI_POLICY.md) — implement the spec, do not invent behavior |
| **Talks / demos** | [website/](website/README.md) MkDocs site and speaker runbook |

---

## How a run looks

```mermaid
flowchart LR
  A[PROJECT_CONTEXT] --> B[DECISION seal]
  B --> C[TOOL_CALL]
  C --> D[TOOL_RESULT]
  D --> E[COMMIT_STEP]
```

1. Host projects context for the step  
2. Model plan is **sealed** before tools with side effects  
3. Tools execute under an effect class (fail closed if unknown)  
4. Step commits; trajectory can be exported as `.tir`

Same agent, two modes: **live** does the job; **sandbox** refuses dangerous *classified* effects and still keeps the sealed plan. Tag `bash` as read-only and `ExecTool` will refuse unless you set `AllowOpenWorldOverride` and own that lie.

---

## What this is / is not

| Trajectory IR **is** | Trajectory IR **is not** |
|---|---|
| A portable runtime IR + `.tir` package for agent trajectories | LLVM, bytecode, or a compiler for prompts |
| Seals, effect classes, resume *semantics*, hash-verifiable export | A replacement for Temporal / DBOS / Restate (adapters only) |
| A thin layer over pluggable durable backends | An agent orchestration framework (not LangGraph) |
| At-most-one automatic retry policy + a key you can forward | Exactly-once remote writes (that is the *server's* idempotency key) |
| A demo/CI effect-class gate (R06) | A process sandbox or `bash` AST analyzer |
| Open source libraries (Apache-2.0) | A hosted multi-tenant SaaS control plane, or a long-term memory product |

---

## Project status

| Item | Status |
|---|---|
| Spec | `spec-v0.2-draft` — [master specification](docs/MASTER_SPECIFICATION.md) |
| Phase | **1C harden and adopt** (Go primary; Python parity) — see [docs/ROADMAP.md](docs/ROADMAP.md) |
| Library | Latest tag is **v0.2.2**. `pyproject.toml` is **0.3.0** for the import move, and that version is not tagged. The one-step Claude attach is not its own release |
| OpenSSF Best Practices | [Project 14075](https://www.bestpractices.dev/projects/14075) |
| CNCF Sandbox | Preparing — [outline](docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md). **Not** claiming CNCF membership until TOC approval |
| Adopters | Honest empty list — [ADOPTERS.md](ADOPTERS.md) (add yourself via PR when you consent) |

---

## Documentation map

| Doc | Purpose |
|---|---|
| [docs/MASTER_SPECIFICATION.md](docs/MASTER_SPECIFICATION.md) | **Normative** architecture, data model, protocols |
| [go/QUICKSTART.md](go/QUICKSTART.md) | Fastest path to a working Go client |
| [QUICKSTART.md](QUICKSTART.md) | Repo-wide quickstart (Go + Python) |
| [docs/SCOPE_AND_NON_GOALS.md](docs/SCOPE_AND_NON_GOALS.md) | In / out of scope |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Public roadmap |
| [docs/INTEGRATIONS.md](docs/INTEGRATIONS.md) | Host and backend integration notes |
| [docs/RELEASE.md](docs/RELEASE.md) | Release process |
| [website/README.md](website/README.md) | Demo / docs site (MkDocs) |
| [CHANGELOG.md](CHANGELOG.md) | What shipped |
| [website/docs/one-step.md](website/docs/one-step.md) | One-step Claude Code attach in this checkout |

---

## Contributing

We welcome students, first-time OSS contributors, and experienced systems engineers.

1. Read **[CONTRIBUTING.md](CONTRIBUTING.md)** — every commit needs `Signed-off-by` (DCO)
2. Follow **[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)**
3. Prefer **Go** for new Phase 1B/1C features (`go/trajir`); keep Python green for parity
4. Do **not** reimplement durable execution (retry / lease / crash engines) — adapters only
5. Spec questions → open a **Spec question** issue; do not invent undefined behavior

```bash
git commit -s -m "feat: short description"
```

Governance: [GOVERNANCE.md](GOVERNANCE.md) · Maintainers: [MAINTAINERS.md](MAINTAINERS.md) · AI-assisted work: [AI_POLICY.md](AI_POLICY.md)

### Good first directions

- Improve docs and quickstarts for newcomers
- Add or harden conformance / unit tests
- Fix sandbox, redaction, or packaging edge cases
- Dual-language parity when an issue asks for it
- Real adopter stories in [ADOPTERS.md](ADOPTERS.md) (with consent only)

---

## Security

Please report vulnerabilities privately per **[SECURITY.md](SECURITY.md)**.  
Do not open public issues for undisclosed security bugs.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE).  
Third-party notices: [docs/THIRD_PARTY_LICENSES.md](docs/THIRD_PARTY_LICENSES.md)

---

## Acknowledgments

Trajectory IR builds on ideas and prior art from durable execution (Temporal, DBOS, Restate), MCP tool annotations, and the CNCF TAG Infrastructure discussion of agentic AI storage needs. See §3.1 and §18 of the [master specification](docs/MASTER_SPECIFICATION.md).

---

*Questions? Open a GitHub Discussion or Issue. Spec wins over chat — when in doubt, read [docs/MASTER_SPECIFICATION.md](docs/MASTER_SPECIFICATION.md).*
