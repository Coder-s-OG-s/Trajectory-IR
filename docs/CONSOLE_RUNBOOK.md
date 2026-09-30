# Console runbook

Boot the local operator UI from a checked-in fixture. No network calls.
A maintainer who already has Go can do this in a few minutes.

Spec: [CONSOLE_EVENTS.md](CONSOLE_EVENTS.md). Ingest: [CONSOLE_INGEST.md](CONSOLE_INGEST.md).
Epic: [#391](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/391).

The fixture `go/trajir/console/testdata/console_demo.ndjson` is trajectory
`console-demo`. It has one seal create, one seal verify, one projection
(`raw_char_len` 400, `projected_char_len` 100, so the reader reports 75
estimated tokens avoided), one redaction count, and one `.tir` export followed
by an import of `demo.tir`. The demo script also copies
`testdata/sample_thin.tir` to `packages/console-demo/demo.tir` so Transfers can
download the file and Show in folder can open it. The UI copies summary fields.
It does not recompute hashes or token estimates.

## Prepare the data

Linux:

```bash
bash scripts/console_demo.sh
```

Windows (PowerShell 7, or Windows PowerShell 5):

```powershell
pwsh -File scripts/console_demo.ps1
powershell -File scripts/console_demo.ps1
```

The script copies the fixture into `$TRAJIR_CONSOLE_DATA/trajectories/console-demo.ndjson`
(default `<repo>/.console-data`) and prints the local URL. It does not start
the server.

## Run the console

```bash
cd go
go run ./cmd/trajir-console -addr 127.0.0.1:8787
```

Open `http://127.0.0.1:8787/?id=console-demo`.

1. Overview shows the event count and the seal / package rollup.
2. Seals shows `seal.created` then `seal.verified` with `ok=true`.
3. Transfers shows the export and the import of `demo.tir` (mode thin, redacted).
4. Economy shows the projection. Tokens avoided (est.) is 75. That figure is `ceil(chars/4)`, not a provider invoice.
5. Transfers lists the local `demo.tir` copy. Download and Show in folder only work on this machine.
6. Sidebar Show data folder / Open PowerShell here talk to `POST /v1/local/*` on loopback. The browser does not spawn a shell itself.

Stop the process with Ctrl+C. Nothing here phones home.

## Optional file sink

The demo above does not need a live host. When emit hooks are on the branch
you are running, a host main attaches a file sink with three lines. Set
`TRAJIR_CONSOLE_SINK=file` and `TRAJIR_CONSOLE_DATA` to the same directory the
script printed.

```go
sink := emit.FromEnv()
tr, err := client.OpenTrajectory(tenant, traj, client.Options{
    WorkDir: dir, ConsoleSink: sink,
})
```

`emit` is `github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit`. A sink error
is logged. The node log still commits.
