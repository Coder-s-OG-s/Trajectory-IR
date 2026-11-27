# Performance

What it costs to record an agent step and to move a trajectory as a `.tir`
package, on the default local path (SQLite NodeLog, LocalSQLite backend, thin
packages). These are numbers from one machine. Rerun them on yours before you
rely on them.

## How to rerun

From `go/`:

```bash
go test ./trajir/client -run '^$' -bench Step -benchmem -benchtime=1000x -count=4
go test ./trajir/tir    -run '^$' -bench .    -benchmem -count=3
```

Postgres NodeLog, with the live stack from
[LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md) running:

```bash
export TRAJIR_DATABASE_URL=postgresql://trajir:trajir@127.0.0.1:5432/trajir
go test ./trajir/postgres -run '^$' -bench . -benchmem -benchtime=1000x -count=4
```

Without `TRAJIR_DATABASE_URL` the Postgres benchmarks skip. With `1000x` the
1,000 step read takes a few minutes per count.

Use a fixed `-benchtime` for `BenchmarkStep`. Every step syncs SQLite to
disk, and with Go's automatic iteration ramp the result swung between 8 ms and
26 ms per step across three runs on the machine below. With `1000x` it stayed
between 4.5 and 5.0 ms.

The benchmarks live in `go/trajir/client/bench_test.go`,
`go/trajir/tir/bench_test.go` and `go/trajir/postgres/bench_test.go`.

Close Docker Desktop and other heavy programs before the SQLite runs. With
the Docker VM running on the machine below, export and verify came out up to
three times slower and swung a lot between runs.

## What one step is

Project context, seal a decision, run one `PURE` tool, commit. The export,
import and verify benchmarks use about 2 KB of varied text per step (random
words and hex from a fixed seed) so package sizes are not flattered by a
repeated string.

## Results

Measured 2026-11-22 on `main` at `96bee7b`, Go 1.26.11, Docker stopped.

Machine: Intel Core i7-13650HX, 16 GB RAM, SSD, Windows 11. Median of the
runs.

| Benchmark | 100 steps | 1,000 steps |
|---|---|---|
| Step (one full step, any length) | 4.7 ms | 4.7 ms |
| Export (thin) | 25 ms | 167 ms |
| Package size | 96 KB | 938 KB |
| Verify (what `trajir verify` runs) | 12 ms | 89 ms |
| Import | 28 ms | 223 ms |

Step cost does not grow with trajectory length: 100 and 1,000 step runs give
the same per step time. Next to a model call that takes seconds, recording is
small.

Export and verify scale roughly linearly.

Import writes the whole package in one transaction since
[#554](https://github.com/Coder-s-OG-s/Trajectory-IR/pull/554). Before that
it committed each node separately and took 480 ms for 100 steps and 4.6 s
for 1,000 on this machine (`0454488`).

### Agent hooks

Measured 2026-11-24 on the same machine, Go 1.26.11, dashboard sink off.

`BenchmarkHookPrePost` in `go/cmd/trajir` runs one tool call the way a host
does, the pre hook and then the post hook, in process, on a warm work dir:
**46 ms per call, so about 23 ms per hook**. That covers reading the payload,
the lock, the state file and the SQLite writes.

```bash
cd go && go test -run '^$' -bench HookPrePost -benchtime 200x ./cmd/trajir/
```

A real host starts `trajir` as a new process for each hook, and on Windows
that start costs more than the hook work. Timed from Git Bash, 20 runs each:

| Process | Per run |
|---|---|
| `true` (no-op) | 55 ms |
| Empty Go binary | 72 ms |
| `trajir version` (14.8 MB release build) | 241 ms |
| `trajir hook pre` (release build) | 302 ms |

The cost follows binary size and is not Go init: `GODEBUG=inittrace=1`
shows a few milliseconds in total, Defender real-time scanning was off, and a
build holding only the hook path (10.8 MB) was no faster. Linux and macOS
were not measured. A tool call that takes the model seconds to plan has room
for this, but a lighter hook process is open work.

### Postgres NodeLog

Measured 2026-11-22 at `96bee7b` against `postgres:16.6` from
`docker-compose.live.yml`, in Docker Desktop on the same Windows machine.
Median of four runs, `-benchtime=1000x`.

| Benchmark | Result |
|---|---|
| Append, one node | 4.7 ms |
| The five nodes of one step | 21.5 ms |
| Read a 100 step trajectory (`ListNodes`, 500 rows) | 5.6 ms |
| Read a 1,000 step trajectory (5,000 rows) | 49 ms |

One of the four 1,000 step read runs is left out. It reported 29 s per read
because the machine went to sleep during it.

These are NodeLog calls only. The Go client and `tir.Import` still run on the
SQLite log, so there is no Postgres step, export or import number yet. Each
Go `Append` is its own transaction (begin, savepoint, insert, slot check,
commit), so one node is about five round trips and a step about 25. The
connection goes through Docker Desktop's VM, which adds latency to each round
trip, so expect different numbers from a Postgres on a Linux host. Do not
compare these directly with the SQLite table.

## Not measured yet

- Postgres step, export and import, and S3 CAS. They need the live Docker
  stack from [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md).
- Postgres on a Linux host instead of Docker Desktop.
- Temporal backend.
- Linux and macOS. SQLite sync cost differs a lot between filesystems.
- Fat packages with artifact bytes.
- Concurrent writers to one log.
- The Python reference port.
