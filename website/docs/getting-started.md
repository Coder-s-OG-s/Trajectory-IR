# Getting started

Phase 1B default path is **Go**. Full detail lives in the repo:

- [go/QUICKSTART.md](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/go/QUICKSTART.md)
- Root [QUICKSTART.md](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/QUICKSTART.md) (Go first, Python reference below)

## Prerequisites

- Go 1.22+ (repo CI tracks current stable)
- Git clone of [Coder-s-OG-s/Trajectory-IR](https://github.com/Coder-s-OG-s/Trajectory-IR)

## First success (under 15 minutes)

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR/go
go run ./examples/adoption_host -with-package
```

Then run the hero crash story:

```bash
go run ./examples/kill_mid_deploy \
  -workdir ./kill_mid_deploy-data \
  -crash-during=tool_call
# kill when TOOL_CALL starts
go run ./examples/kill_mid_deploy \
  -workdir ./kill_mid_deploy-data \
  -resume
```

## Python reference

Python remains the Phase 1A reference / parity port (`examples/`, `client/python/`). Prefer Go for new demos and drivers unless you are explicitly working on Python parity.
