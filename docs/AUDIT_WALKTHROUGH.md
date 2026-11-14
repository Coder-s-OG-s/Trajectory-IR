# Audit walkthrough

Ten minutes from a clone to an agent record that someone else can check
offline, and that fails loudly when it has been changed.

What you end with: a `.tir` package that says what the agent planned, which
tools it called with which effect class, what came back, and a content hash
on every row. `trajir verify` checks it without your database, your agent or
any network.

This is evidence for review. It is not a compliance certificate. The question
to field map is [ARTICLE_12_FIELD_MAP.md](ARTICLE_12_FIELD_MAP.md).

## 1. Set up

You need Git, Go 1.26 and Python 3.11+.

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
python -m venv .venv
# Windows: .venv\Scripts\activate    Unix: source .venv/bin/activate
pip install -e .
cd go && go build -o ../bin/ ./cmd/trajir && cd ..
```

That writes `bin/trajir` (`bin	rajir.exe` on Windows). Use that path below.

## 2. Record a run

The LangGraph stub runs one sealed step with two tools through the same
wrapper a real LangGraph `ToolNode` uses. No model and no API key.

```bash
python integrations/langgraph/demo_stub.py
```

It writes `.local-evidence/langgraph-demo.tir`. For your own LangGraph agent,
the wrapper is in [EVIDENCE_SIDECAR.md](EVIDENCE_SIDECAR.md#langgraph-toolnode-wrap).

## 3. Verify it

```bash
bin/trajir verify .local-evidence/langgraph-demo.tir
```

```
OK .local-evidence/langgraph-demo.tir
```

Exit code 0. `-json` prints the same result as JSON.

## 4. See what an auditor gets

A `.tir` is a zip. `nodes.ndjson` has one row per event. Save this as
`show_pack.py`:

```python
import json, sys, zipfile

with zipfile.ZipFile(sys.argv[1]) as z:
    for line in z.read("nodes.ndjson").decode().splitlines():
        n = json.loads(line)
        print(n["step_n"], n["seq"], n["kind"], json.dumps(n["payload"])[:120])
```

```bash
python show_pack.py .local-evidence/langgraph-demo.tir
```

```
1 0 PROJECT_CONTEXT {"goal": "evidence sidecar stub", "host": "langgraph-stub"}
1 1 DECISION {"plan": {"tool_calls": [{"args": {"msg": "hello-evidence"}, "name": "echo"}, ...
1 2 TOOL_CALL {"args": {"msg": "hello-evidence"}, "idempotency_key": "c86e...", "tool": "echo"}
1 3 TOOL_RESULT {"result": "hello-evidence"}
1 4 TOOL_CALL {"args": {"service": "api", "version": "1.0.0"}, "idempotency_key": "51e3...", "tool": "ship_release"}
1 5 TOOL_RESULT {"result": {"shipped": "api:1.0.0"}}
1 6 COMMIT_STEP {}
```

Reading it:

| Row | Tells you |
|---|---|
| `DECISION` | The whole plan, sealed before any tool ran |
| `TOOL_CALL` | Which tool, with which args, and the idempotency key the host can forward |
| `TOOL_RESULT` | What came back |
| `COMMIT_STEP` | The step was closed |

Every row also has `tenant_id`, `trajectory_id`, `ts` and an `id` that is the
hash of its content. `ship_release` was declared `NON_IDEMPOTENT_WRITE`, so
after a crash it would not be retried automatically.

## 5. Change it and verify again

Save this as `tamper.py`. It rewrites one value inside the package, the way
someone editing a record after the fact would:

```python
import sys, zipfile

src, dst, old, new = sys.argv[1:5]
with zipfile.ZipFile(src) as zin, zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zout:
    for item in zin.infolist():
        data = zin.read(item.filename)
        if item.filename == "nodes.ndjson":
            data = data.replace(old.encode(), new.encode())
        zout.writestr(item, data)
```

```bash
python tamper.py .local-evidence/langgraph-demo.tir .local-evidence/edited.tir api:1.0.0 api:9.9.9
bin/trajir verify .local-evidence/edited.tir
```

```
FAIL .local-evidence/edited.tir
  LOAD_FAILED: tir verification: node id mismatch kind=TOOL_RESULT seq=5 recorded=... expected=...
```

Exit code 1, and it names the row. Flipping a random byte in the file also
fails, usually as a zip read error.

## 6. Sign it, so a rewrite can't pass

Step 5 catches an edit that leaves the old hashes in place. Someone who
rewrites the whole package can recompute every hash, and an unsigned package
built that way verifies as OK. If the record has to stand up against the
person who produced it, sign it and verify against a key you trust.

Save as `sign_pack.py`:

```python
import sys
from nacl.signing import SigningKey
from trajectory_ir.package.signature import sign_package

sk = SigningKey.generate()
pub = bytes(sk.verify_key)
sign_package(sys.argv[1], bytes(sk) + pub)
print(pub.hex())
```

```bash
cp .local-evidence/langgraph-demo.tir .local-evidence/signed.tir
python sign_pack.py .local-evidence/signed.tir      # prints the public key
bin/trajir verify -require-signature -trust-key <public key> .local-evidence/signed.tir
```

| Case | Result |
|---|---|
| Signed, trusted key | `OK` |
| Unsigned, `-require-signature` | `FAIL SIGNATURE_REQUIRED: package is unsigned` |
| Signed, different `-trust-key` | `FAIL SIGNATURE_REQUIRED: public key not in trust store` |
| Signed, then edited | `FAIL LOAD_FAILED: node id mismatch` |

In real use the key lives with whoever signs releases of the record, not in a
throwaway script. The signature format is `trajir-pkg-sig-v1` (Ed25519), see
[MASTER_SPECIFICATION.md](MASTER_SPECIFICATION.md).

## 7. When a tool fails

What gets recorded depends on the host today.

- **Claude Code hooks.** A failed tool is written as an `ABORT` row with
  `HOST_REPORTED_FAILURE`, the tool name and the error text. This repo's own
  sessions run with the hooks, and their logs show these rows.
- **LangGraph wrap.** A tool that raises leaves its `TOOL_CALL` with no
  `TOOL_RESULT` and no `ABORT`, the error is not recorded, and the step still
  gets a `COMMIT_STEP`. That is a bug, tracked in
  [#561](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/561). Until it is
  fixed, a `TOOL_CALL` with no result means "did not finish", not "succeeded".

## Using Claude Code instead

The one-step attach in [EVIDENCE_SIDECAR.md](EVIDENCE_SIDECAR.md#one-step-claude-attach-local)
sets up the hooks and the `trajectory-ir` MCP server for a worktree. After a
session, export with the MCP tool `trajectory_export_tir`. Pass `dest` as a
path relative to `work_dir`; a path that repeats `work_dir` currently fails
([#535](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/535)). Then run
`trajir verify` on the result as in step 3.

## What this does not prove

- That the remote side of a tool call happened. The record is what this host
  saw.
- Anything before the seal. A `DECISION` sealed in process before the tool ran
  is evidence. A log that arrives after the fact is not.
- That nothing was left out, unless the producer signed it.
