# Go quickstart (primary path)

First success path without Python. Matches APIs under `go/trajir` on `main`.
The current release is v0.3.0.

## Prerequisites

- Go **1.26.11** (see `go/go.mod`)
- Git

Optional later: Docker Postgres/MinIO/Temporal for live drivers
([docs/LIVE_INTEGRATION_DOCKER.md](../docs/LIVE_INTEGRATION_DOCKER.md)).

## 1. Clone and test

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR/go
go test ./...
```

## 2. Minimal client step

```go
package main

import (
	"fmt"
	"os"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

func main() {
	dir, err := os.MkdirTemp("", "trajir-qs-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	tr, err := client.OpenTrajectory("demo", "qs-1", client.Options{WorkDir: dir})
	if err != nil {
		panic(err)
	}
	defer tr.Close()

	if _, err := tr.Project(1, map[string]any{"goal": "hello"}); err != nil {
		panic(err)
	}
	plan := map[string]any{
		"tool_calls": []any{
			map[string]any{"name": "echo", "args": map[string]any{"msg": "hi"}},
		},
	}
	if _, err := tr.SealDecision(1, plan); err != nil {
		panic(err)
	}

	tool := resume.Tool{
		Name:   "echo",
		Effect: effects.PURE,
		Fn: func(args map[string]any) (any, error) {
			return args["msg"], nil
		},
	}
	res, err := tr.ExecTool(1, 2, tool, map[string]any{"msg": "hi"})
	if err != nil {
		panic(err)
	}
	fmt.Println(res.Result) // hi
	if err := tr.CommitStep(1, 4); err != nil {
		panic(err)
	}
}
```

Run from `go/`. The snippet is a `package main`, so it runs as its own
package inside the module: save it as `qs/main.go` (keep the `package main`
line) and run that package.

```bash
# from go/, after saving the snippet above as go/qs/main.go
go run ./qs
```

To run the same snippet from a module of your own, see
[Use it in your own project](#use-it-in-your-own-project) below.

### Use it in your own project

`go get` alone is not enough: it leaves `go.sum` without entries for this
module's dependencies, and `go run .` fails with `missing go.sum entry`
(sqlite, jcs, pgx) until you run `go mod tidy`.

```bash
mkdir trajir-qs && cd trajir-qs
go mod init example.com/trajir-qs
go get github.com/Coder-s-OG-s/Trajectory-IR/go@latest
# save the snippet from section 2 as main.go in this directory
go mod tidy
go run .
```

## 3. Demos

From `go/` (no paid model API):

```bash
go run ./examples/adoption_host
go run ./examples/adoption_host -sandbox
go run ./examples/adoption_host -with-package
go run ./examples/kill_mid_deploy -workdir ./demo-data
```

| Demo | Command | Intent |
|------|---------|--------|
| Adoption host | `go run ./examples/adoption_host` | Host seal loop + optional CAS / thin `.tir` |
| Kill mid deploy | `go run ./examples/kill_mid_deploy -workdir ./demo-data` | Crash safety / sealed plan |

## 4. Durable backends

| Backend | Role |
|---------|------|
| LocalSQLite / Memory | Coding and tests (test fakes) |
| Temporal | Production for Go |

```bash
# optional live Temporal check (stack: docs/LIVE_INTEGRATION_DOCKER.md)
# TEMPORAL_HOSTPORT=localhost:7233
go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
```

## 5. Live drivers (optional)

```bash
# from repo root
docker compose -f docker-compose.live.yml up -d
# then export TRAJIR_DATABASE_URL / MinIO env / TEMPORAL_HOSTPORT
# see docs/LIVE_INTEGRATION_DOCKER.md
```

## 6. Next

- Package map: [README.md](README.md)
- Phase 1C status: [../docs/PHASE_1C_STATUS.md](../docs/PHASE_1C_STATUS.md)
- Phase 1B epic (closed): [#113](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/113)
- Root docs (includes Python reference path): [../QUICKSTART.md](../QUICKSTART.md)
