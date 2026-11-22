# Third-party dependency licenses

How Trajectory IR's dependency tree lines up with CNCF's allowed-license
policy, and how that's kept true going forward.

## Policy

The [CNCF Allowlist License Policy](https://github.com/cncf/foundation/blob/main/policies-guidance/allowed-third-party-license-policy.md)
lists the licenses a dependency may use without asking: Apache-2.0, 0BSD,
BSD-2-Clause, BSD-3-Clause, MIT, MIT-0, ISC, PSF-2.0, Python-2.0, PostgreSQL,
UPL-1.0, X11, Zlib and a few more. Anything else, including LGPL and MPL,
needs an exception from the CNCF Governing Board. Approved exceptions are
listed at [exceptions.cncf.io](https://exceptions.cncf.io/).

LGPL is not allowed by default. CNCF has approved it for specific packages
and uses, for example `paramiko` (LGPL-2.1) for oscal-compass. GPL, AGPL and
SSPL dependencies should not be added.

## Current state

Scanned the actual resolved dependency trees, not the ambient dev
environment (which pulls in unrelated packages never part of this project).

### Python

Clean-venv install of `pip install .[postgres,s3]` (the real runtime deps:
`dbos`, `rfc8785`, `psycopg[binary]`, `boto3`, plus transitives):

| Package | License |
|---|---|
| `psycopg`, `psycopg-binary` | **LGPL-3.0-only**, not on the allowlist, see below |
| everything else (`dbos`, `rfc8785`, `boto3`, `botocore`, `s3transfer`, `jmespath`, `python-dateutil`, `click`, `PyYAML`, `SQLAlchemy`, `greenlet`, `typing_extensions`, `tzdata`, `urllib3`, `six`, `colorama`, `websockets`) | Apache-2.0 / MIT / BSD / PSF-2.0 |

**`psycopg` and `psycopg-binary`:** LGPL-3.0-only. They come in only with
the optional `postgres` extra. The core install and the Go SDK do not use
them, and they are imported as unmodified libraries. CNCF has no approved
exception for `psycopg` today (checked against exceptions.cncf.io on
2026-11-22), so:

- the Sandbox application lists them under "dependencies with licenses not
  on the allowlist", with the reason above
- if the project is accepted, a maintainer files an exception request
  (the CNCF foundation repo has an issue template for it)
- the other way out is a Postgres driver under an allowlisted license for the
  Python `postgres` extra

CI leaves them out of the strict allow-only check with `--ignore-packages`
(`.github/workflows/security-scan.yml`), so the exclusion is visible rather
than silently passing.

### Go

Full resolved tree of `go/go.mod` via `go-licenses`: **100% permissive**
(Apache-2.0 / MIT / BSD-3-Clause / ISC). No GPL/AGPL/LGPL anywhere.

Two modules `go-licenses` flags `Unknown` are tool false-negatives, not real
gaps:

- `modernc.org/mathutil` — has a BSD-3-Clause `LICENSE` file (identical
  boilerplate to its sibling `modernc.org/*` packages, which the tool
  detects correctly); the tool's regex just misses this one file.
- `github.com/nexus-rpc/nexus-proto-annotations` — the `v0.1.0` module-proxy
  snapshot predates that repo adding a `LICENSE` file. Confirmed MIT via
  `gh api repos/nexus-rpc/nexus-proto-annotations -q .license`.

Both are excluded via `--ignore` in CI rather than left silently unverified.

## Keeping this true

`.github/workflows/security-scan.yml` runs two jobs on every PR:

- **Dependency license scan (Python)** — `pip-licenses --allow-only=...
  --ignore-packages psycopg psycopg-binary` against a clean install of the
  real runtime dependency tree.
- **Dependency license scan (Go)** — `go-licenses check ./...
  --disallowed_types=forbidden,restricted,reciprocal,unknown` against the full
  resolved `go.mod` tree. In go-licenses terms GPL and LGPL are `restricted`
  and MPL and EPL are `reciprocal`, so all of them fail the job.

A new Python dependency outside the allow-only list, or a new Go dependency
under a copyleft or unknown license, fails the PR instead of going
unnoticed.
