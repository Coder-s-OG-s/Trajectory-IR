# Third-party dependency licenses

How Trajectory IR's dependency tree lines up with CNCF's allowed-license
policy, and how that's kept true going forward.

## Policy

CNCF (following ASF-style license categories) treats permissive licenses
(Apache-2.0, MIT, BSD-2/3-Clause, ISC, PSF-2.0) as freely usable, weak-copyleft
licenses (LGPL, MPL) as acceptable when the dependency is used as an
unmodified, dynamically-linked/imported library rather than compiled into a
combined work, and strong-copyleft or restrictive licenses (GPL, AGPL, SSPL)
as disallowed.

## Current state

Scanned the actual resolved dependency trees, not the ambient dev
environment (which pulls in unrelated packages never part of this project).

### Python

Clean-venv install of `pip install .[postgres,s3]` (the real runtime deps:
`dbos`, `rfc8785`, `psycopg[binary]`, `boto3`, plus transitives):

| Package | License |
|---|---|
| `psycopg`, `psycopg-binary` | **LGPL-3.0-only** — documented exception, see below |
| everything else (`dbos`, `rfc8785`, `boto3`, `botocore`, `s3transfer`, `jmespath`, `python-dateutil`, `click`, `PyYAML`, `SQLAlchemy`, `greenlet`, `typing_extensions`, `tzdata`, `urllib3`, `six`, `colorama`, `websockets`) | Apache-2.0 / MIT / BSD / PSF-2.0 |

**`psycopg`/`psycopg-binary` exception:** LGPL-3.0-only. `postgres` is an
optional extra, not a core dependency; it's imported as an unmodified
library, never statically linked into a single binary. This is the standard
basis on which CNCF/ASF-style policy treats LGPL as acceptable. Explicitly
excluded from the strict allow-only gate via `--ignore-packages` in CI
(`.github/workflows/security-scan.yml`) rather than silently passing.

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
  --disallowed_types=forbidden` against the full resolved `go.mod` tree.

A future dependency (or transitive dependency) landing under GPL, AGPL, SSPL,
or anything else outside the allowlist fails the PR instead of going
unnoticed.
