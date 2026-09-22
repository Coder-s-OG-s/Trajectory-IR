# Console ingest (local)

How to run the append-only event store for the Trajectory console.

Spec: [CONSOLE_EVENTS.md](CONSOLE_EVENTS.md). Issue:
[#393](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/393).

## Layout

```text
$TRAJIR_CONSOLE_DATA/
  trajectories/
    <trajectory_id>.ndjson
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
| `GET` | `/v1/trajectories/{id}/events` | Append-ordered events |
| `GET` | `/v1/trajectories/{id}/summary` | Seal / economy / transfer rollup |
| `GET` | `/healthz` | Liveness (no auth) |
| `GET` | `/` | Operator UI shell (trajectory picker + panels) |
| `GET` | `/ui/*` | Static CSS/JS for the shell |

When `TRAJIR_CONSOLE_TOKEN` is set, send `Authorization: Bearer <token>` on
all `/v1/*` routes. The UI has a token field (sessionStorage) for local demos.

Open `http://127.0.0.1:8787/?id=<trajectory_id>` for a deep link.

## Library

Go package: `github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/console`

Hosts can call `Store.Append` directly (file sink) without starting HTTP.
SDK emit hooks are tracked separately ([#398](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/398)).
