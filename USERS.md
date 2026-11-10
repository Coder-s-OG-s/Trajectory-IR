# Users

Trajectory IR is a library you keep next to an agent you already run. It writes a diary of what the agent decided, which tool it was about to call, and what came back. You can export that diary as a `.tir` file and check the file later with `trajir verify`.

The current library tag is v0.3.0. `pyproject.toml` is 0.3.0. This page is the setup and the daily use. The short command list is also in [README.md](README.md) under "Attach it to a project". The normative rules stay in [docs/MASTER_SPECIFICATION.md](docs/MASTER_SPECIFICATION.md).

You run Trajectory IR on your own machine, or on a Postgres and S3 you already operate. This repository does not host a cloud account for you.

## What you need

| Piece | When you need it |
| --- | --- |
| Git | Always |
| Go 1.26.10 | Building `go/trajir.exe`, `go/trajir-mcp.exe`, and `go/trajir-console.exe`. The module toolchain in `go/go.mod` is `go1.26.10`. |
| Python 3.11 or newer | The Python client, the Claude hooks, LangGraph, ADK callbacks, Postgres, and S3. |
| A project you already open in an agent | Only if you want the one step attach. |

Docker is optional. Use it when you want Postgres, MinIO, or a live Temporal server. The laptop path does not need Docker. The demos below do not need a paid model key.

The built binaries keep the `.exe` suffix on Linux and macOS too. `go/trajir` is the Go package directory, so the file is `go/trajir.exe` on every OS.

## Clone the repository

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
```

Stay in that checkout for the install scripts. If you want the diary in some other repository, pass that directory as the target in the attach step. The hook scripts stay in this checkout. Your home directory Claude config is not edited.

## See the Go library work

From `go/`:

```bash
cd go
go test ./...
```

A smaller first run, still from `go/`:

```bash
go run ./examples/adoption_host
go run ./examples/adoption_host -sandbox
go run ./examples/adoption_host -with-package
go run ./examples/kill_mid_deploy -workdir ./demo-data
```

`adoption_host` is a host loop with an optional thin package. `kill_mid_deploy` stops in the middle of a tool call and resumes the same work directory. The full client snippet, with imports, is in [go/QUICKSTART.md](go/QUICKSTART.md).

Go production crash recovery is Temporal, and it stays outside this library. A live check needs the stack in [docs/LIVE_INTEGRATION_DOCKER.md](docs/LIVE_INTEGRATION_DOCKER.md) and `TEMPORAL_HOSTPORT` (the quickstart uses `localhost:7233`). You do not need Temporal to seal a local run.

## Install the Python library

Python is the reference port. Until a wheel is published, install from this checkout. Publishing to PyPI is a maintainer step after a tag. See [docs/RELEASE.md](docs/RELEASE.md).

From the repository root, the directory that contains `pyproject.toml`. If you are still inside `go/`, run `cd ..` first.

```bash
python -m venv .venv
```

Windows PowerShell:

```powershell
.\.venv\Scripts\Activate.ps1
python -m pip install -U pip
python -m pip install -e ".[dev]"
```

bash:

```bash
source .venv/bin/activate
python -m pip install -U pip
python -m pip install -e ".[dev]"
```

`.[dev]` includes DBOS. A base install, without that extra, covers the client, the node log, projection, and `.tir` export. `pip install "trajectory-ir[dbos]"` is the same DBOS extra once a wheel exists. `open_trajectory` starts DBOS when that extra is installed. Pass `durable=False` to skip it, or `durable=True` to fail if the extra is missing. The Claude hook does not start DBOS.

Check the conformance tests if you want a green Python tree before you wire a host:

```bash
pytest conformance/ -q
```

## A short Python run

This writes a SQLite diary, seals one plan, runs a pure echo tool, and commits the step. It matches [QUICKSTART.md](QUICKSTART.md).

```python
from trajectory_ir.client.python.trajectory_client import (
    open_trajectory,
    project,
    seal_decision,
    exec_tool,
    commit_step,
)
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.tool import Tool


def echo(msg: str) -> str:
    return msg


traj = open_trajectory(tenant_id="demo", trajectory_id="qs-1", db_path="qs.sqlite")
project(traj, step_n=1, context={"goal": "hello"})
seal_decision(
    traj,
    step_n=1,
    plan={"tool_calls": [{"name": "echo", "args": {"msg": "hi"}}]},
)
tool = Tool(name="echo", fn=echo, effect_class=EffectClass.PURE)
result = exec_tool(
    traj,
    step_n=1,
    call={"args": {"msg": "hi"}},
    tool=tool,
    seq=2,
)
print(result.result)  # hi
commit_step(traj, step_n=1, seq=4)
```

Sandbox mode is a demo gate on effect class. Open with `mode="sandbox"` and a tool classed `NON_IDEMPOTENT_WRITE`, `AGENT_SPAWN`, or `SENSITIVE` is refused before the body. `PURE`, `READ_ONLY`, and `IDEMPOTENT_WRITE` still run. Sandbox mode does not isolate a process, and it does not read a bash script to decide if the script is safe.

Export that diary when you want a file you can move:

```python
from trajectory_ir.package import export_tir
from trajectory_ir.runtime.log import NodeLog

src = NodeLog("qs.sqlite")
export_tir(src, "qs-1", "run.tir", mode="thin", tenant_id="demo")
```

`mode="thin"` writes the hashes and leaves bulky bytes in a content address store. `mode="fat"` can carry artifact bytes in the package. `redacted=True` is a heuristic pass. Read the file before you send it to someone else. `import_tir` checks node hashes. There is no import that skips the check and still writes the log.

## Attach Trajectory IR to a project

Run this from the Trajectory IR checkout. It builds the three binaries, writes project local config, and smoke checks one library seal. It does not download Claude. It does not create `pack.tir`. The smoke check does not run the hook scripts, so a printed `INSTALL OK` means the library seal worked. It does not prove the hook command on your agent.

No target means "this checkout":

```powershell
pwsh -File scripts\install_claude.ps1
```

```bash
./scripts/install_claude.sh
```

Another repository:

```powershell
pwsh -File scripts\install_claude.ps1 -Target C:\path\to\your\project
```

```bash
./scripts/install_claude.sh --target /path/to/your/project
```

Skip starting the console:

```powershell
pwsh -File scripts\install_claude.ps1 -Target C:\path\to\your\project -SkipConsole
```

```bash
./scripts/install_claude.sh --target /path/to/your/project --skip-console
```

Which Python the hook will call:

* PowerShell uses `TRAJIR_PYTHON`, then `.venv\Scripts\python.exe` in this checkout, then `python` on `PATH`.
* The shell script uses `TRAJIR_PYTHON`, then `.venv/bin/python`, then `.venv/Scripts/python.exe` when that file exists, then `python3`, then `python`.

Set `TRAJIR_PYTHON` to the interpreter you trust before you run the script. That path is copied into the hook command.

The same command writes:

| File in the target | What it is |
| --- | --- |
| `.claude/settings.local.json` | Claude Code hooks: PreToolUse, PostToolUse, PostToolUseFailure |
| `.mcp.json` | Claude Code MCP server `trajectory-ir` |
| `.cursor/mcp.json` | Cursor MCP server |
| `.vscode/mcp.json` | VS Code MCP server |
| `.gemini/settings.json` | Gemini CLI MCP server |
| `.codex/config.toml` | Codex MCP server |
| `.local-evidence/claude-one-step.json` | Identity file: work directory, tenant `local`, trajectory `claude-one-step` |
| `.local-evidence/claude-agent/` | SQLite diary for those hooks |
| `.trajir/langgraph_attach.py` and `.trajir/adk_attach.py` | Snippets. Importing the file does not import LangGraph, ADK, or DBOS. |

Other server names in the JSON configs stay. The name written is `trajectory-ir`, and an old `trajectory-ir` block in those JSON files is replaced. Codex is the exception: if `.codex/config.toml` already contains `mcp_servers.trajectory-ir`, that file is left unchanged. Invalid JSON, including a comment or a trailing comma, is left as it is and the command exits 1 before the smoke check.

The installer appends these lines to the target `.gitignore` when they are missing:

```text
.local-evidence/
.claude/settings.local.json
.mcp.json
.cursor/mcp.json
.vscode/mcp.json
.gemini/settings.json
.codex/config.toml
.trajir/*.py
```

MCP command paths are absolute paths on the machine where you ran the installer. Move the checkout and run the installer again. Claude Code asks you to approve the project MCP server before that server starts.

`scripts/install_evidence_sidecar.ps1` is the smaller build. It compiles `go/trajir.exe` and prints the next commands. Use `install_claude.ps1` when you want hooks, MCP config, and the console start.

## What the Claude hooks do

The hook command is `integrations/attach/launch_hook.py` in this checkout. The last word of that command is the owner, `project`. The launcher only runs these three files from this install:

* `integrations/claude/pre_tool_use.py`
* `integrations/claude/post_tool_use.py`
* `integrations/claude/post_tool_use_failure.py`

Any other script path exits 2 on PreToolUse and is not executed.

PreToolUse seals the decision and exits 0 so Claude can run the tool. The hook does not call the tool. A second PreToolUse from the same owner `project`, while that call is still pending, exits 2 for `NON_IDEMPOTENT_WRITE` and for a row that has no effect class. That is the crash before PostToolUse. A different owner on the same pending call exits 0. If you register this hook twice, the second registration needs a different owner word. The same word looks like a retry and blocks the first run.

After PostToolUse records the result, a later PreToolUse for that same `NON_IDEMPOTENT_WRITE` id exits 2. A recorded row with no effect class exits 2 as well.

PostToolUse writes the result after the tool has run. If that diary write fails, PostToolUse still exits 0. The host is not asked to run the tool again. The tool call row is stored only after the tool has run.

PostToolUseFailure records a failed tool and clears the pending row.

If Claude denies the tool after the seal, neither post hook runs. The pending row stays in the target `.local-evidence` and the decision stays in the log. There is no cleanup command. Leave the row. The next tool is a new step.

If `TRAJIR_IDENTITY_FILE` points at a file that is not there, PreToolUse exits 2 and the message names `scripts/install_claude.ps1` or `scripts/install_claude.sh`. If that variable is unset, the work directory falls back to this install's `.local-evidence/claude-agent`. The installer sets the variable to the target identity file, so a normal attach uses `<target>/.local-evidence/claude-agent`.

If Python itself never starts, Claude can still run the tool. Once the launcher process is up, an import error on PreToolUse exits 2.

The hooks do not seal tools that belong to Cursor, VS Code, Gemini CLI, or Codex. Those hosts only receive the MCP config described next.

## MCP servers

`go/trajir-mcp.exe` speaks MCP on stdio until the client disconnects. The installer starts it with `TRAJIR_MCP_ROOT` set to the absolute target path. Every `work_dir`, `dest`, and `path` has to stay under that root. If you run the binary yourself and leave `TRAJIR_MCP_ROOT` unset, the process current directory is the root.

`nodes.sqlite` and `memo.sqlite` inside a work directory must not be symlinks.

| Tool | What you ask it for |
| --- | --- |
| `trajectory_status` | Node counts by kind, seal count, and paths. Arguments: `work_dir`, `tenant_id`, `trajectory_id`. |
| `trajectory_export_tir` | Write a `.tir` file. `dest` is required and must stay under the work directory or the workspace root. `mode` is `thin` (the default) or `fat`. |
| `trajectory_import_tir` | Load a `.tir` and check hashes. It does not write a node log. |
| `trajectory_verify_signature` | Check an optional `trajir-pkg-sig-v1` signature. An unsigned pack is accepted unless you set `require_signature`. |

For a diary written by the Claude hooks, the ids in `.local-evidence/claude-one-step.json` are tenant `local` and trajectory `claude-one-step`. `work_dir` is `.local-evidence/claude-agent` under the target. Ask the host to call `trajectory_export_tir` with those values and a `dest` inside the project, then verify the file with the command in "Check a pack".

These four tools do not wrap the host's own tools. They do not decide whether Claude, Cursor, or Codex may call bash. Sealing the tool call is the Claude hook, or the in-process LangGraph and ADK wraps.

A hand written MCP entry, if you are not using the installer, looks like this. Use the absolute path to the binary you built.

```json
{
  "mcpServers": {
    "trajectory-ir": {
      "command": "C:/path/to/Trajectory-IR/go/trajir-mcp.exe",
      "args": [],
      "env": {
        "TRAJIR_MCP_ROOT": "C:/path/to/your/project"
      }
    }
  }
}
```

Do not put signing keys in that file.

## Plugins

There is no plugin marketplace in this repository. The installer is the plugin step that exists today: it writes the project MCP config and the two `.trajir` snippets. Skill files and a separate host UI are not shipped here. [docs/INTEGRATIONS.md](docs/INTEGRATIONS.md) keeps host plugin packaging as a later repository of config only.

Copilot is not in the installer. If your editor is not Claude Code, Cursor, VS Code, Gemini CLI, or Codex, use the MCP JSON above or the Python and Go libraries.

## The console on your machine

The console listens on `127.0.0.1:8787` only. Open `http://127.0.0.1:8787/`.

If that port is free, the installer starts `go/trajir-console.exe` and waits until it accepts a connection. The data directory is `<target>/.local-evidence/console-data`. If something is already listening, the installer leaves that process alone. If the process it just started dies, the installer exits 1. One console is enough. Do not start a second one on another port and expect the docs to follow it.

Start it yourself:

```powershell
.\go\trajir-console.exe -data C:\path\to\project\.local-evidence\console-data -addr 127.0.0.1:8787
```

```bash
./go/trajir-console.exe -data /path/to/project/.local-evidence/console-data -addr 127.0.0.1:8787
```

| Flag | Environment variable | Meaning |
| --- | --- | --- |
| `-addr` | `TRAJIR_CONSOLE_ADDR` | Listen address. Default `127.0.0.1:8787`. |
| `-data` | `TRAJIR_CONSOLE_DATA` | Directory of NDJSON under `trajectories/`. Required. |
| `-token` | `TRAJIR_CONSOLE_TOKEN` | Optional Bearer token for the HTTP API. Empty means the local API does not check a token. |

The Claude hooks write SQLite. They do not post events to the console. An empty Evidence tab after attach is expected until some program emits. The LangGraph stub does that when you set the sink and run it:

```powershell
$env:TRAJIR_CONSOLE_SINK = "http"
$env:TRAJIR_CONSOLE_URL = "http://127.0.0.1:8787"
python integrations/langgraph/demo_stub.py
```

Then open `http://127.0.0.1:8787/?id=langgraph-demo`. `TRAJIR_CONSOLE_SINK=file` writes NDJSON under `TRAJIR_CONSOLE_DATA` instead of HTTP. Leave the sink unset and nothing is emitted. A sink error is logged. The node log and the package write still succeed.

Numbers on the console are `ceil(chars/4)` estimates. They are not the provider's `prompt_tokens` and they are not an invoice.

## LangGraph

LangGraph is the wrap that gates the tool body in-process. Install the extra in the Trajectory IR checkout:

```bash
python -m pip install -e ".[langgraph]"
```

```python
from langgraph.prebuilt import ToolNode
from integrations.langgraph import TrajIRToolGuard

guard = TrajIRToolGuard(
    tenant_id="demo",
    trajectory_id="lg-live",
    work_dir=".local-evidence/lg-live",
)
tools = ToolNode(my_tools, wrap_tool_call=guard.wrap_tool_call)
```

The first body runs. A later resume does not run that gated body again. When you want a pack:

```python
path = guard.export_tir(".local-evidence/lg-live.tir")
guard.close()
```

Then `./go/trajir.exe verify` on that path. Flags go before the pack path.

A smoke that does not need LangGraph installed:

```powershell
pwsh -File scripts\install_evidence_sidecar.ps1
python integrations/langgraph/demo_stub.py
.\go\trajir.exe verify .local-evidence\langgraph-demo.tir
```

The longer notes are in [integrations/langgraph/README.md](integrations/langgraph/README.md) and [docs/EVIDENCE_SIDECAR.md](docs/EVIDENCE_SIDECAR.md).

## Google ADK

ADK uses callbacks. This repository does not import the ADK library. You pass the callbacks into the agent you already constructed. The callbacks seal and record. They do not call the tool. A missing call id raises `ValueError`. No live ADK agent is part of the tests here.

```python
from integrations.adk import AdkToolGuard

guard = AdkToolGuard(
    tenant_id="demo",
    trajectory_id="adk-live",
    work_dir=".local-evidence/adk-live",
)
agent = Agent(
    tools=my_tools,
    before_tool_callback=guard.before_tool_callback,
    after_tool_callback=guard.after_tool_callback,
)
```

The same shape is written to `.trajir/adk_attach.py` in the target project. Call the function when you want it. Importing the snippet does not import ADK.

## The diary on disk

The usual laptop store is SQLite plus a directory of content-addressed bytes.

* The node log is `nodes.sqlite` in the work directory (`memo.sqlite` sits beside it when the client opens a memo).
* Artifact bytes go through `FileSystemCAS` if you are not using S3.

```python
from trajectory_ir.package import export_tir, load_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.storage import FileSystemCAS, put_artifact, rehydrate_artifacts

store = FileSystemCAS("./cas_root")
log = NodeLog("traj.sqlite")
ref = put_artifact(store, b"bytes from a tool", logical_path="out.bin")
export_tir(
    log,
    "my-traj",
    "run.tir",
    mode="thin",
    artifacts=[ref],
    tenant_id="demo",
    cas=store,
)
pkg = load_tir("run.tir")
rehydrated = rehydrate_artifacts(store, pkg.artifacts_manifest)
```

The ordered walkthrough, including what is optional, is [docs/E2E_POSTGRES_CAS_THIN.md](docs/E2E_POSTGRES_CAS_THIN.md).

## Postgres

Postgres replaces the SQLite node log when you want more than one process sharing the diary. You run Postgres yourself. Trajectory IR does not provision it.

```bash
export TRAJIR_DATABASE_URL=postgresql://trajir:trajir@localhost:5432/trajir
python -m pip install -e ".[dev,postgres]"
```

PowerShell:

```powershell
$env:TRAJIR_DATABASE_URL = "postgresql://trajir:trajir@localhost:5432/trajir"
python -m pip install -e ".[dev,postgres]"
```

```python
from trajectory_ir.drivers.postgres.log import open_postgres_node_log

log = open_postgres_node_log()
log.append("DECISION", 1, {"plan": {}}, "e2e-pg", "demo", seq=1)
log.close()
```

`open_postgres_node_log` reads `TRAJIR_DATABASE_URL`, then `DATABASE_URL`. Pass `dsn=` if you do not want the environment. `export_tir` accepts a `PostgresNodeLog` the same way it accepts SQLite when you pass the log and `tenant_id`.

The Docker recipe used in development is in [CONTRIBUTING.md](CONTRIBUTING.md).

## S3 and MinIO

S3 replaces the on disk content address store. MinIO is the local stand in. You bring the bucket. The library does not create an account.

```bash
export TRAJIR_S3_ENDPOINT_URL=http://127.0.0.1:9000
export TRAJIR_S3_BUCKET=trajir
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
python -m pip install -e ".[dev,s3]"
```

```python
from trajectory_ir.drivers.s3.cas import S3CAS, build_s3_client_from_env
from trajectory_ir.storage import put_artifact, rehydrate_artifacts

store = S3CAS(build_s3_client_from_env(), bucket="trajir")
ref = put_artifact(store, b"payload", logical_path="out.bin")
rehydrate_artifacts(store, [{"content_hash": ref.content_hash}])
```

When `TRAJIR_S3_ENDPOINT_URL` is set, the client uses path style addressing so MinIO works. `AWS_DEFAULT_REGION` is optional. MinIO setups often use `us-east-1`.

| Variable | Who reads it |
| --- | --- |
| `TRAJIR_DATABASE_URL` | `open_postgres_node_log` |
| `DATABASE_URL` | Fallback if `TRAJIR_DATABASE_URL` is unset |
| `TRAJIR_S3_ENDPOINT_URL` | `build_s3_client_from_env` |
| `TRAJIR_S3_BUCKET` | Bucket name you pass to `S3CAS` |
| `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` | S3 credentials |
| `TRAJIR_MCP_ROOT` | Bounds MCP paths |
| `TRAJIR_PYTHON` | Interpreter written into the hook command |
| `TRAJIR_IDENTITY_FILE` | Set by the hook launcher to the identity JSON |
| `TRAJIR_CONSOLE_ADDR`, `TRAJIR_CONSOLE_DATA`, `TRAJIR_CONSOLE_TOKEN` | The console process |
| `TRAJIR_CONSOLE_SINK`, `TRAJIR_CONSOLE_URL` | Programs that emit console events |

## Check a pack

From the Trajectory IR checkout, flags first, then the pack path:

```bash
./go/trajir.exe verify path/to/pack.tir
```

```powershell
.\go\trajir.exe verify path\to\pack.tir
```

You should see `OK` and the path. A bad hash prints `FAIL` and the findings, and the process exits 1. Usage errors exit 2.

```text
trajir verify [--require-signature] [--trust-key hex] [--trust-key-id id] [--json] <pack.tir>
```

`--json` prints the result object. `--require-signature` fails when the package has no `SIGNATURE` member. `--trust-key` is a hex ed25519 public key. Repeat the flag for more than one key. `--trust-key-id` is the signer id. Repeat it the same way.

Unsigned packs stay valid with the plain `verify` command. The signature format in this tree is `trajir-pkg-sig-v1`. This release does not verify packs with Sigstore.

Change one byte of a pack you just exported and run `verify` again. That is the check that the hash is doing the work. Do it on a copy.

## Effect classes

Every tool call is stored with one class. If the class is missing or the tool name is unknown, the class is `NON_IDEMPOTENT_WRITE`. A tool named Bash is `NON_IDEMPOTENT_WRITE`. The same is true for the other open world names (`python`, `sql`, browser). Trajectory IR does not parse the command string. An operator can set the class on the tool. Tagging bash as read only, then calling `ExecTool`, is refused unless you set the open world override and accept that you did that on purpose.

| Class | What to expect |
| --- | --- |
| `PURE` | Safe to run again. |
| `READ_ONLY` | Safe to run again. |
| `IDEMPOTENT_WRITE` | Safe to run again if the tool really is idempotent. |
| `NON_IDEMPOTENT_WRITE` | One automatic attempt. A retry after an interruption needs a person. |
| `AGENT_SPAWN` | Same gate as a non idempotent write. Spawning another agent is not safe to repeat blindly. |
| `SENSITIVE` | Same gate. A sensitive read that should stay re fetchable is `Tool.sensitive` on a base class, not the class `SENSITIVE` by itself. |

`NON_IDEMPOTENT_WRITE`, `AGENT_SPAWN`, and `SENSITIVE` are block-and-gate. The sealed plan is kept. The body is not fired again on its own. That policy is at most one automatic attempt. If the remote API can apply a write twice, you still have to send it an idempotency key. The key on the `TOOL_CALL` is a length prefixed sha256 of domain, tenant, trajectory, step, and seq. It is exposed as call metadata. It is not stuffed into Python kwargs. Your host has to forward that string.

A tool result the hash cannot accept, because an integer falls outside the safe JSON range (bigger than 9007199254740991, or smaller than the negative of that), is stored as an `integer_domain` error. The raw number is not written into the diary. That call is not run again.

## Resume, in one page

A sealed step is not planned again. Looking at the world again is allowed. A new plan is a new step. If the host declared what the step depends on, `CheckWorld` fails loud on drift instead of pretending the old plan still fits.

`PURE` and the other classes that are not gated may run the body again on resume. If a result was already recorded, the durable memo can still skip the body. DBOS (Python reference) and Temporal (Go production) are the engines that remember that memo. They are not started by `trajir verify`, by `trajir-mcp`, or by the Claude hook.

OpenTelemetry labels are copied only after the in-process seal. A collector that sees a span is not itself a sealed pack.

## Limits

Read this before you put Trajectory IR in a security review or a sales page.

* The adopter list in [ADOPTERS.md](ADOPTERS.md) is empty. Add a row with a pull request when a real user agrees to be named.
* The token figure is `ceil(chars/4)`. It is not a model bill and it is not a savings claim.
* Sandbox mode refuses three effect classes. It is not a process sandbox.
* Package verify checks hashes, and optionally `trajir-pkg-sig-v1`. It is not Sigstore.
* [docs/ARTICLE_12_FIELD_MAP.md](docs/ARTICLE_12_FIELD_MAP.md) names fields in the pack. It is not a compliance certificate.
* GitHub Pages for this repository is not enabled. The install notes live in the repo at [website/docs/one-step.md](website/docs/one-step.md). Do not send people to the github.io URL until Pages is turned on.
* Trajectory IR is not LangGraph, not a memory store, and not a hosted control plane. LangGraph, ADK, Temporal, DBOS, and Restate keep their own jobs.

## When the installer stops

| What you see | What it means |
| --- | --- |
| `target directory not found` | `-Target` or `--target` is not a directory. |
| `go build trajir failed` (or the mcp or console build) | Go is missing, or it is older than the toolchain in `go/go.mod`. |
| `config write failed` and a JSON error | A target config file has a comment, a trailing comma, or other JSON the parser will not read. Fix the file. The installer does not rewrite it. |
| `smoke verify failed` | The library seal after the config write failed. The hook scripts were not the thing under test. |
| Installer exits 1 after talking about the console | Port 8787 was free, the process started, then it died. Read the console log. Or rerun with `-SkipConsole` / `--skip-console`. |
| PreToolUse exits 2 and names the install script | `TRAJIR_IDENTITY_FILE` is set but the file is gone. Run the installer again. |
| PreToolUse exits 2 on the first real tool | The owner word `project` is registered twice, or a pending row for that call is still there from a tool Claude denied. |
| Claude ran the tool and the diary is empty | Python never started, so the hook never ran. Check `TRAJIR_PYTHON` and that the hook command in `.claude/settings.local.json` points at this checkout. |
| MCP tool says the path is outside the root | `work_dir`, `dest`, or `path` is not under `TRAJIR_MCP_ROOT`. |
| `OK` from verify, empty console | Expected. Hooks write SQLite. Set `TRAJIR_CONSOLE_SINK` on a program that emits, or export a `.tir` and verify that. |

Report a vulnerability in private, the way [SECURITY.md](SECURITY.md) describes. A public issue is fine for a broken install step.

## Where to read next

| Doc | Use it for |
| --- | --- |
| [README.md](README.md) | Project landing page |
| [QUICKSTART.md](QUICKSTART.md) | Go first, then the Python reference, including the env table |
| [go/QUICKSTART.md](go/QUICKSTART.md) | The Go client snippet and Temporal notes |
| [website/docs/one-step.md](website/docs/one-step.md) | The same attach, in the docs folder |
| [docs/INTEGRATIONS.md](docs/INTEGRATIONS.md) | MCP tools, confinement, and what the server will not do |
| [docs/E2E_POSTGRES_CAS_THIN.md](docs/E2E_POSTGRES_CAS_THIN.md) | SQLite, Postgres, file CAS, and S3 in order |
| [docs/CONSOLE_INGEST.md](docs/CONSOLE_INGEST.md) | How events get into the console |
| [docs/EVIDENCE_SIDECAR.md](docs/EVIDENCE_SIDECAR.md) | The LangGraph seal before the tool body |
| [docs/SCOPE_AND_NON_GOALS.md](docs/SCOPE_AND_NON_GOALS.md) | What the project refuses to become |
| [ADOPTERS.md](ADOPTERS.md) | Empty until someone consents |
| [SECURITY.md](SECURITY.md) | Private vulnerability reports |
| [CONTRIBUTING.md](CONTRIBUTING.md) | DCO, local Postgres, local MinIO |

License is Apache-2.0.
