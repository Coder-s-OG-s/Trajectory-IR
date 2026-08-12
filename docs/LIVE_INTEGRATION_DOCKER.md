# Live integration Docker sandbox

Vacant local stack for **Postgres NodeLog**, **MinIO S3 CAS**, and **Temporal**
(Go durable). Offline unit and conformance tests do **not** need this.

Compose file (repo root): [`docker-compose.live.yml`](../docker-compose.live.yml)

Tracked under Phase 1C: [#154](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/154),
smoke automation [#160](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/160).

## Prerequisites

- Docker Desktop (or Docker Engine) with a **running Linux engine**
  - Confirm: `docker version` shows a **Server** version (not only Client)
  - On Windows, if the Server section is missing, wait until `wsl -l -v`
    shows `docker-desktop` as **Running**, then re-check `docker version`
- Go 1.25.x and Python 3.11+ with `pip install -e ".[dev,postgres,s3]"`
- Free host ports: `5432`, `9000`, `9001`, `7233` (7233 only if Temporal is up)

## One command smoke (preferred)

From the repository root, with Docker **Server** healthy:

```bash
# Unix
chmod +x scripts/run_live_matrix.sh
./scripts/run_live_matrix.sh              # Postgres + MinIO + live tests
./scripts/run_live_matrix.sh --with-temporal   # also Temporal integration
./scripts/run_live_matrix.sh --up-only         # start stack only
```

```powershell
# Windows PowerShell
.\scripts\run_live_matrix.ps1
.\scripts\run_live_matrix.ps1 -WithTemporal
.\scripts\run_live_matrix.ps1 -UpOnly
```

The script:

1. Starts compose services (postgres + minio; Temporal optional)
2. Waits until Postgres and MinIO answer health probes
3. Creates the `trajir` bucket via Python (`drivers.s3`) if missing
4. Runs Python and Go live tests

## Manual start

```bash
docker compose -f docker-compose.live.yml up -d postgres minio
# full stack including Temporal:
# docker compose -f docker-compose.live.yml up -d
docker compose -f docker-compose.live.yml ps
```

Wait until services are healthy:

```bash
docker exec trajir-live-postgres pg_isready -U trajir -d trajir
curl -sf http://127.0.0.1:9000/minio/health/live
```

MinIO healthcheck is defined in compose (`curl` against `/minio/health/live`).
The bucket is **not** created by a one-shot `mc` container (tags on Docker Hub
have disappeared mid-stream); create it with the smoke script or:

```bash
export TRAJIR_S3_ENDPOINT_URL=http://127.0.0.1:9000
export TRAJIR_S3_BUCKET=trajir
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
python -c "from drivers.s3.cas import build_s3_client_from_env; c=build_s3_client_from_env(); c.create_bucket(Bucket='trajir')"
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

## Live matrix commands (manual)

### Python (reference port)

```bash
pip install -e ".[dev,postgres,s3]"
pytest test/integration/test_postgres_live.py -q
pytest test/integration/test_s3_minio_live.py -q
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

Postgres + MinIO only (default for the smoke script):

```bash
docker compose -f docker-compose.live.yml up -d postgres minio
```

## Windows Docker Desktop note

If `docker compose` fails with a missing `dockerDesktopLinuxEngine` pipe, the
Linux engine dropped. Restart Docker Desktop and wait until
`docker version` prints a **Server** version before re-running the smoke script.

## Related

- [CONTRIBUTING.md](../CONTRIBUTING.md)
- [E2E_POSTGRES_CAS_THIN.md](E2E_POSTGRES_CAS_THIN.md)
- [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md)
- [go/QUICKSTART.md](../go/QUICKSTART.md)
