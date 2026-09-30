# Console ingest (local)

How to run the append-only event store for the Trajectory console.

Spec: [CONSOLE_EVENTS.md](CONSOLE_EVENTS.md). Issue:
[#393](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/393).

## Layout

```text
$TRAJIR_CONSOLE_DATA/
  trajectories/
    <trajectory_id>.ndjson
  packages/
    <trajectory_id>/
      <name>.tir
```

Each line is one `console-events-v1` event object.

## Run

```bash
export TRAJIR_CONSOLE_DATA=/tmp/trajir-console
export TRAJIR_CONSOLE_TOKEN=dev-token   # optional; empty = open local API
cd go
go run ./cmd/trajir-console -addr 127.0.0.1:8787
```

## HTTP

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/v1/events` | Body = one event JSON; fail-closed on invalid envelope |
| `GET` | `/v1/trajectories` | List trajectory ids |
| `GET` | `/v1/savings` | Cross-trajectory tokens saved (est.). Sum of lifetime avoided |
| `GET` | `/v1/dashboard` | Home rollup: savings plus one card per run |
| `GET` | `/v1/trajectories/{id}/events` | Append-ordered events |
| `GET` | `/v1/trajectories/{id}/summary` | Seal / economy / transfer rollup |
| `GET` | `/v1/trajectories/{id}/packages` | Local `.tir` copies for that id |
| `GET` | `/v1/trajectories/{id}/packages/{name}` | Download one staged package |
| `POST` | `/v1/local/reveal` | Loopback only. Open Explorer/Finder on the data dir or a staged `.tir` |
| `POST` | `/v1/local/open-shell` | Loopback only. Open PowerShell (Windows) or a folder (other OS) in the data dir. No command string from the browser |
| `GET` | `/healthz` | Liveness (no auth) |
| `GET` | `/` | Operator UI shell (trajectory picker + panels) |
| `GET` | `/ui/*` | Static CSS/JS for the shell |

When `TRAJIR_CONSOLE_TOKEN` is set, send `Authorization: Bearer <token>` on
all `/v1/*` routes. The UI has a token field (sessionStorage) for local demos.
`POST /v1/local/reveal` and `POST /v1/local/open-shell` also require a loopback
client. They never take a shell command from the browser.

Open `http://127.0.0.1:8787/?id=<trajectory_id>` for a deep link.

## Library

Go package: `github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/console`

Hosts can call `Store.Append` directly (file sink) without starting HTTP.

## Attach a file sink

Set `TRAJIR_CONSOLE_SINK=file` and `TRAJIR_CONSOLE_DATA` to the directory above. Leave the sink env unset and nothing is emitted. A sink error is logged. The node log and the package write still succeed.

```go
sink := emit.FromEnv()
tr, err := client.OpenTrajectory(tenant, traj, client.Options{
    WorkDir: dir, ConsoleSink: sink,
})
```

`emit` is `github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit`. `client` is `go/trajir/client`.

Python reference, same env vars:

```python
from trajectory_ir.console_emit import from_env
traj = open_trajectory(tenant, traj, db_path, console_sink=from_env())
```

HTTP is `TRAJIR_CONSOLE_SINK=http`, `TRAJIR_CONSOLE_URL` (default `http://127.0.0.1:8787`), and optional `TRAJIR_CONSOLE_TOKEN`.
