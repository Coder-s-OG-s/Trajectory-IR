# Trajectory IR v0.2.0

Phase **1B** release: **Go is the primary SDK** and default onboarding path.
Python remains the supported reference and parity port from Phase 1A.

## Highlights

- Go primary in the master README and CONTRIBUTING
- `go/QUICKSTART.md` and `go/examples/adoption_host`
- Go Postgres NodeLog and S3 CAS (including AWS SDK v2 / MinIO env wiring)
- Go `Resume` fails closed without history; plain tools leave IR history
- CI: deep gates, cross language `.tir` round trip, Go coverage floor **80%**
- Builds on the 0.1.x reliability and Phase B integration work

## Install

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
git checkout v0.2.0
pip install -e ".[dev]"
# optional: pip install -e ".[s3]" or ".[postgres]"
```

```bash
cd go
go test ./...
go run ./examples/adoption_host
```

When PyPI is published:

```bash
pip install trajectory-ir==0.2.0
```

## Not in 0.2.0

- Package digital signatures
- Fluid / k8s-fluid product packaging
- Multi tenant SaaS control plane
- Automated PyPI Trusted Publishing (manual upload still optional)
- Branch protection on private free GitHub plans (needs public or paid plan)

## Full changelog

See [CHANGELOG.md](../CHANGELOG.md) section `[0.2.0]`.
