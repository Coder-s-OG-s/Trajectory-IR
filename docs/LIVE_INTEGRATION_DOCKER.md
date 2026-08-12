# Live integration Docker sandbox

Vacant local stack for **Postgres NodeLog**, **MinIO S3 CAS**, and **Temporal**
(Go durable). Offline unit and conformance tests do **not** need this.

Compose file (repo root): [`docker-compose.live.yml`](../docker-compose.live.yml)

Tracked under Phase 1C: [#154](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/154).

## Prerequisites

- Docker Desktop (or Docker Engine) with a **running Linux engine**
  - Confirm: `docker version` shows a **Server** version (not only Client)
  - On Windows, if the Server section is missing, wait until `wsl -l -v`
    shows `docker-desktop` as **Running**
- Go 1.25.x and Python 3.11+ (for running the live tests against the stack)
- Free host ports: `5432`, `9000`, `9001`, `7233`

## Start

From the repository root:

```bash
docker compose -f docker-compose.live.yml up -d
docker compose -f docker-compose.live.yml ps
```

Wait until services are healthy (`docker compose ... ps` shows healthy for
postgres and minio). Compose already healthchecks MinIO and runs `minio-init`
only after MinIO is healthy; bucket create fails closed if `mc` errors.

```bash
# Postgres
docker exec trajir-live-postgres pg_isready -U trajir -d trajir

# MinIO (from the host)
curl -sf http://127.0.0.1:9000/minio/health/live

# Bucket init one-shot should have exited 0
docker compose -f docker-compose.live.yml ps -a minio-init
```

Temporal may take longer on first pull. Frontend: `localhost:7233`.

## Environment

Export these for live tests (Unix shell):

```bash
export TRAJIR_DATABASE_URL=postgresql://trajir:trajir@127.0.0.1:5432/trajir
export TRAJIR_S3_ENDPOINT_URL=http://127.0.0.1:9000
export TRAJIR_S3_BUCKET=trajir
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export AWS_DEFAULT_REGION=us-east-1
export TEMPORAL_HOSTPORT=localhost:7233
```

PowerShell:

```powershell
$env:TRAJIR_DATABASE_URL = "postgresql://trajir:trajir@127.0.0.1:5432/trajir"
$env:TRAJIR_S3_ENDPOINT_URL = "http://127.0.0.1:9000"
$env:TRAJIR_S3_BUCKET = "trajir"
$env:AWS_ACCESS_KEY_ID = "minioadmin"
$env:AWS_SECRET_ACCESS_KEY = "minioadmin"
$env:AWS_DEFAULT_REGION = "us-east-1"
$env:TEMPORAL_HOSTPORT = "localhost:7233"
```

## Live matrix commands

### Python (reference port)

```bash
pip install -e ".[dev,postgres,s3]"
pytest test/integration/test_postgres_live.py -q
pytest test/integration/test_s3_minio_live.py -q
```

Optional single live NodeLog case under unit:

```bash
pytest test/unit/test_postgres_node_log.py -q -k live
```

### Go (primary)

```bash
cd go
go test ./trajir/postgres/... -count=1 -v
go test ./trajir/cas -run TestLiveS3StoreFromEnvRoundTrip -count=1 -v
go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
```

## Teardown

```bash
docker compose -f docker-compose.live.yml down -v
```

## Partial stack

Postgres + MinIO only (skip Temporal image):

```bash
docker compose -f docker-compose.live.yml up -d postgres minio minio-init
```

## Related

- [CONTRIBUTING.md](../CONTRIBUTING.md) (one-off `docker run` recipes)
- [E2E_POSTGRES_CAS_THIN.md](E2E_POSTGRES_CAS_THIN.md)
- [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md)
- [go/QUICKSTART.md](../go/QUICKSTART.md)
