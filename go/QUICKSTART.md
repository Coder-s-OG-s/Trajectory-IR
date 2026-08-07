# Go quickstart (Phase 1B primary path)

First success path without Python. Matches APIs under `go/trajir` on `main`.

## Prerequisites

- Go **1.25.x** (see `go/go.mod`)
- Git

Optional later: Docker Postgres/MinIO, Temporal for production durable backends.

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

Run from `go/`:

```bash
# save as /tmp/qs.go or a small main under examples, then:
go test ./trajir/client/... -count=1
```

## 3. Demos

| Demo | Command | Intent |
|------|---------|--------|
| Kill mid deploy | `go run ./examples/kill-mid-deploy -workdir ./demo-data` | Crash safety |
| Adoption host | `go run ./examples/adoption_host` (when merged) | Host seal loop + optional CAS |

## 4. Durable backends

| Backend | Role |
|---------|------|
| LocalSQLite / Memory | Coding and tests (test fakes) |
| Temporal | Production for Go |

```bash
# optional live Temporal check
# TEMPORAL_HOSTPORT=localhost:7233
go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
```

## 5. Next

- Package map: [README.md](README.md)
- Phase 1B epic: [#113](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/113)
- Root docs (includes Python reference path): [../QUICKSTART.md](../QUICKSTART.md)
