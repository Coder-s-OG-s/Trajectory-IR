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

Use a fixed `-benchtime` for `BenchmarkStep`. Every step syncs SQLite to
disk, and with Go's automatic iteration ramp the result swung between 8 ms and
26 ms per step across three runs on the machine below. With `1000x` it stayed
between 4.5 and 5.0 ms.

The benchmarks live in `go/trajir/client/bench_test.go` and
`go/trajir/tir/bench_test.go`.

## What one step is

Project context, seal a decision, run one `PURE` tool, commit. The export,
import and verify benchmarks use about 2 KB of varied text per step (random
words and hex from a fixed seed) so package sizes are not flattered by a
repeated string.

## Results

Measured 2026-11-15 on `main` at `0454488`, Go 1.26.11.

Machine: Intel Core i7-13650HX, 16 GB RAM, SSD, Windows 11. Median of the
runs.

| Benchmark | 100 steps | 1,000 steps |
|---|---|---|
| Step (one full step, any length) | 4.6 ms | 4.6 ms |
| Export (thin) | 21 ms | 156 ms |
| Package size | 96 KB | 938 KB |
| Verify (what `trajir verify` runs) | 9.3 ms | 80 ms |
| Import | 480 ms | 4.6 s |

Step cost does not grow with trajectory length: 100 and 1,000 step runs give
the same per step time. Next to a model call that takes seconds, recording is
small.

Export and verify scale roughly linearly.

Import is the slow part on `main`. It commits each node separately, about
1 ms per node on this machine (five nodes per step). Pull request
[#554](https://github.com/Coder-s-OG-s/Trajectory-IR/pull/554) writes the
whole package in one transaction. The same import benchmark on that branch
(`85f2cbd`) gave 25 ms for 100 steps and 218 ms for 1,000. Update this table
when it merges.

## Not measured yet

- Postgres NodeLog and S3 CAS. They need the live Docker stack from
  [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md).
- Temporal backend.
- Linux and macOS. SQLite sync cost differs a lot between filesystems.
- Fat packages with artifact bytes.
- Concurrent writers to one log.
- The Python reference port.
