# End to end: NodeLog + CAS + thin `.tir`

Walkthrough of the multi process / portable artifact path that v0.1.0 already
ships: **NodeLog** (SQLite or Postgres), **CAS** (filesystem or S3 compatible),
and **thin package** export / rehydrate.

All snippets use APIs that exist on `main`. Nothing here invents client methods.

## What you need vs what is optional

| Piece | Required for this guide? | Notes |
|-------|--------------------------|--------|
| Python 3.11+ editable install | **Yes** | `pip install -e ".[dev]"` |
| Local filesystem CAS | **Yes** for the primary path | No Docker |
| SQLite `NodeLog` | **Yes** for the primary path | Default IR log |
| Docker Postgres | Optional | Multi process NodeLog |
| Docker MinIO / S3 | Optional | Remote CAS; same CAS protocol |
| Paid model API | No | Not used |

If you only have a laptop and no Docker, complete **Path A** (SQLite + FS CAS +
thin package). Path B (Postgres) and Path C (S3) are optional upgrades.

Cross links:

- Integration service recipes: [CONTRIBUTING.md](../CONTRIBUTING.md) (Postgres + MinIO)
- Phase inventory: [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md)
- Minimal client loop: [QUICKSTART.md](../QUICKSTART.md)
- Host owned demo (when present): `examples/host_loop/`, `examples/adoption_host/`

## Environment variables

| Variable | Used by | Required when |
|----------|---------|----------------|
| `TRAJIR_DATABASE_URL` | `open_postgres_node_log()` | Path B (Postgres) |
| `DATABASE_URL` | fallback for Postgres DSN | if `TRAJIR_DATABASE_URL` unset |
| `TRAJIR_S3_ENDPOINT_URL` | `build_s3_client_from_env` | Path C (MinIO/S3) |
| `TRAJIR_S3_BUCKET` | S3 CAS bucket name | Path C |
| `AWS_ACCESS_KEY_ID` | S3 credentials | Path C |
| `AWS_SECRET_ACCESS_KEY` | S3 credentials | Path C |
| `AWS_DEFAULT_REGION` | optional region | Path C (often `us-east-1` for MinIO) |

---

## Path A (primary): SQLite NodeLog + filesystem CAS + thin package

No Docker. From the repository root after install:

```bash
pip install -e ".[dev]"
```

### A1. Write a short IR history

```python
from pathlib import Path

from trajectory_ir.runtime.log import NodeLog

work = Path("./e2e_work")
work.mkdir(exist_ok=True)
db_path = work / "nodes.sqlite"
log = NodeLog(str(db_path))

trajectory_id = "e2e-thin-1"
tenant_id = "demo"

log.append(
    "PROJECT_CONTEXT",
    1,
    {"goal": "export a thin package"},
    trajectory_id,
    tenant_id,
    seq=0,
)
log.append(
    "DECISION",
    1,
    {"plan": {"tool_calls": []}},
    trajectory_id,
    tenant_id,
    seq=1,
)
log.append(
    "COMMIT_STEP",
    1,
    {},
    trajectory_id,
    tenant_id,
    seq=2,
)
log.close()
print("nodes written to", db_path)
```

You can also drive the same history through the public client
(`open_trajectory` / `project` / `seal_decision` / `commit_step`) as in
`examples/host_loop/` or the minimal flow in [QUICKSTART.md](../QUICKSTART.md).

### A2. Put artifact bytes into a filesystem CAS

```python
from pathlib import Path

from trajectory_ir.storage import FileSystemCAS, put_artifact

cas_root = Path("./e2e_work/cas")
store = FileSystemCAS(cas_root)
payload = b"report bytes from a tool or host process\n"
ref = put_artifact(store, payload, logical_path="outputs/report.bin")
print(ref.content_hash, ref.uri, ref.size)
assert store.has(ref.content_hash)
```

### A3. Export a thin `.tir` with CAS verification

```python
from pathlib import Path

from trajectory_ir.package import export_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.storage import FileSystemCAS, put_artifact

work = Path("./e2e_work")
db_path = work / "nodes.sqlite"
cas_root = work / "cas"
store = FileSystemCAS(cas_root)
ref = put_artifact(store, b"report bytes from a tool or host process\n", logical_path="outputs/report.bin")

log = NodeLog(str(db_path))
dest = work / "run.tir"
export_tir(
    log,
    "e2e-thin-1",
    dest,
    mode="thin",
    artifacts=[ref],
    tenant_id="demo",
    cas=store,  # fail closed if the hash is missing from CAS
)
log.close()
print("exported", dest)
```

Thin mode stores **hashes** (and optional URIs) in the package, not the blob
bytes. Fat mode embeds bytes; that is a different path (see unit tests under
`test/unit/test_tir_package.py`).

### A4. Load and rehydrate; show bytes match

```python
from pathlib import Path

from trajectory_ir.package import load_tir
from trajectory_ir.storage import FileSystemCAS, rehydrate_artifacts

work = Path("./e2e_work")
store = FileSystemCAS(work / "cas")
pkg = load_tir(work / "run.tir")

assert pkg.manifest["mode"] == "thin"
assert pkg.artifact_bytes == {}  # thin: no embedded blobs
print("node_count", pkg.manifest["node_count"])
print("artifacts", pkg.artifacts_manifest)

got = rehydrate_artifacts(store, pkg.artifacts_manifest)
for content_hash, data in got.items():
    print(content_hash[:12], "->", len(data), "bytes")
    assert data == b"report bytes from a tool or host process\n"
print("rehydrate ok")
```

### A5. One shot script (copy paste)

```python
"""e2e_thin_fs.py — SQLite + FileSystemCAS + thin .tir (no Docker)."""
from pathlib import Path

from trajectory_ir.package import export_tir, load_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.storage import FileSystemCAS, put_artifact, rehydrate_artifacts

work = Path("./e2e_work")
work.mkdir(exist_ok=True)
db_path = work / "nodes.sqlite"
cas_root = work / "cas"
traj, tenant = "e2e-thin-1", "demo"
payload = b"report bytes from a tool or host process\n"

log = NodeLog(str(db_path))
log.append("PROJECT_CONTEXT", 1, {"goal": "export a thin package"}, traj, tenant, seq=0)
log.append("DECISION", 1, {"plan": {"tool_calls": []}}, traj, tenant, seq=1)
log.append("COMMIT_STEP", 1, {}, traj, tenant, seq=2)

store = FileSystemCAS(cas_root)
ref = put_artifact(store, payload, logical_path="outputs/report.bin")
dest = work / "run.tir"
export_tir(log, traj, dest, mode="thin", artifacts=[ref], tenant_id=tenant, cas=store)
log.close()

pkg = load_tir(dest)
got = rehydrate_artifacts(store, pkg.artifacts_manifest)
assert got[ref.content_hash] == payload
print("ok", dest, "nodes=", pkg.manifest["node_count"], "hash=", ref.content_hash[:16])
```

```bash
python e2e_thin_fs.py
```

---

## Path B (optional): Postgres NodeLog

Use when you want a multi process friendly IR log. The **public client** still
defaults to SQLite paths; for Postgres you use `PostgresNodeLog` the same way
seal / gate code already uses the NodeLog interface.

### B1. Start Postgres (Docker)

From [CONTRIBUTING.md](../CONTRIBUTING.md):

```bash
docker run -d --name trajir-pg \
  -e POSTGRES_USER=trajir -e POSTGRES_PASSWORD=trajir -e POSTGRES_DB=trajir \
  -p 5432:5432 postgres:16.6
export TRAJIR_DATABASE_URL=postgresql://trajir:trajir@localhost:5432/trajir
pip install -e ".[dev,postgres]"
```

### B2. Append nodes on Postgres

```python
from drivers.postgres.log import open_postgres_node_log

log = open_postgres_node_log()  # reads TRAJIR_DATABASE_URL
traj, tenant = "e2e-pg-1", "demo"
log.append("DECISION", 1, {"plan": {"tool_calls": []}}, traj, tenant, seq=1)
assert log.has(traj, tenant, 1, "DECISION")
rows = log.list_nodes(traj, tenant_id=tenant)
print("count", len(rows), "id", rows[0]["id"][:16])
log.close()
```

### B3. Thin export from Postgres

`export_tir` accepts any log that implements the NodeLog read surface used by
export (list/filter nodes). `PostgresNodeLog` matches SQLite semantics:

```python
from pathlib import Path

from drivers.postgres.log import open_postgres_node_log
from trajectory_ir.package import export_tir, load_tir
from trajectory_ir.storage import FileSystemCAS, put_artifact, rehydrate_artifacts

traj, tenant = "e2e-pg-1", "demo"
log = open_postgres_node_log()
store = FileSystemCAS("./e2e_work/cas")
ref = put_artifact(store, b"from postgres backed run\n", logical_path="outputs/pg.bin")
dest = Path("./e2e_work/pg-run.tir")
export_tir(log, traj, dest, mode="thin", artifacts=[ref], tenant_id=tenant, cas=store)
log.close()

pkg = load_tir(dest)
assert rehydrate_artifacts(store, pkg.artifacts_manifest)[ref.content_hash] == b"from postgres backed run\n"
print("postgres thin export ok", dest)
```

### B4. Live integration test (CI parity)

```bash
pytest test/integration/test_postgres_live.py -q
```

Without `TRAJIR_DATABASE_URL`, that file skips. Unit fakes under
`test/unit/test_postgres_node_log.py` still cover offline behavior.

---

## Path C (optional): S3 compatible CAS (MinIO)

Same CAS protocol as `FileSystemCAS`. Thin packages rehydrate from any store
that verifies hashes on put/get.

### C1. Start MinIO (Docker)

```bash
docker run -d --name trajir-minio -p 9000:9000 \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z server /data
export TRAJIR_S3_ENDPOINT_URL=http://127.0.0.1:9000
export TRAJIR_S3_BUCKET=trajir
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
pip install -e ".[dev,s3]"
python -c "from drivers.s3.cas import build_s3_client_from_env; c=build_s3_client_from_env(); c.create_bucket(Bucket='trajir')"
```

### C2. Put + rehydrate against S3CAS

```python
from drivers.s3.cas import S3CAS, build_s3_client_from_env
from trajectory_ir.storage import put_artifact, rehydrate_artifacts

client = build_s3_client_from_env()
store = S3CAS(client, bucket="trajir")
ref = put_artifact(store, b"s3 payload\n", logical_path="outputs/s3.bin")
got = rehydrate_artifacts(
    store,
    [{"content_hash": ref.content_hash, "logical_path": ref.logical_path}],
)
assert got[ref.content_hash] == b"s3 payload\n"
print("s3 rehydrate ok", ref.content_hash[:16])
```

### C3. Live integration test

```bash
pytest test/integration/test_s3_minio_live.py -q
```

---

## Combining the pieces

A realistic host process often looks like:

1. Append / seal into **NodeLog** (SQLite for laptop, Postgres for multi process).
2. Store tool or host owned blobs in **CAS** (filesystem or S3).
3. `export_tir(..., mode="thin", artifacts=[...], cas=store)`.
4. Elsewhere: `load_tir` + `rehydrate_artifacts(store, pkg.artifacts_manifest)`.

The IR log and the CAS are separate by design (README storage split). Thin
packages are portable **references**; rehydrate needs access to a store that
still has those hashes.

## What this guide deliberately skips

1. New NodeLog or CAS features
2. Changing CI Phase B jobs
3. Multi tenant SaaS control planes
4. Spec backend wording for Temporal (issue #67)
5. Rewriting the master README

## Related tests (do not invent APIs; copy patterns)

| Path | Test |
|------|------|
| Thin + FS CAS | `test/unit/test_cas_artifact_export.py` |
| FS CAS unit | `test/unit/test_fs_cas.py` |
| Atomic export | `test/unit/test_tir_export_atomic_write.py` |
| Live Postgres | `test/integration/test_postgres_live.py` |
| Live MinIO | `test/integration/test_s3_minio_live.py` |
