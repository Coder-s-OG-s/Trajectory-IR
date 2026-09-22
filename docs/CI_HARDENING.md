# CI/CD hardening (CNCF-aligned)

How Trajectory IR strengthens contributor and maintainer CI after Phase 1C.
Grounded in CNCF TAG Security supply-chain guidance and the CNCF
[GitHub Actions CI dependency recipe card](https://www.cncf.io/blog/2026/05/04/securing-github-actions-ci-dependencies-recipe-card/).

Milestone: **[Phase CI/CD harden](https://github.com/Coder-s-OG-s/Trajectory-IR/milestone/6)**

## Principles (from CNCF / OpenSSF)

| Principle | Practice here |
|-----------|----------------|
| Least privilege | Workflows default `contents: read`; release job (not workflow top-level) gets `contents: write` |
| Trusted sources | Prefer GitHub-owned actions; Dependabot weekly for `github-actions` |
| Keep fresh | Dependabot for pip, gomod, and GitHub Actions |
| Audit the kitchen | Scorecard, zizmor (advisory), actionlint, gitleaks, dependency license scan |
| Test before ship | Existing CI gates + Go race on core packages |
| Release integrity | Wheel/sdist + CycloneDX SBOMs attached on `v*` tags |

## Current workflows

| Workflow | File | Role |
|----------|------|------|
| **CI** | `.github/workflows/ci.yml` | DCO, Quality, Package smoke, pip-audit, Go (+ race), Conformance, Postgres, MinIO |
| **Release** | `.github/workflows/release.yml` | Build dist, SBOM, attach to GitHub Release |
| **Scorecard** | `.github/workflows/scorecard.yml` | OpenSSF Scorecard SARIF |
| **CodeQL** | `.github/workflows/codeql.yml` | Go + Python static analysis |
| **Security scan** | `.github/workflows/security-scan.yml` | gitleaks CLI (no paid license), actionlint, zizmor, dependency license scan (Python + Go) vs [CNCF allowlist](THIRD_PARTY_LICENSES.md) |

## Required status checks on `main`

Machine gates (classic branch protection) still require the **CI** job names
listed in [maintainer-branch-protection.md](maintainer-branch-protection.md).

**Optional to promote later** (once green and stable on every PR):

- `Secret scan (gitleaks)`
- `Workflow lint (actionlint)`
- Scorecard / CodeQL are often schedule + main push; require only if they run on every PR and stay green

## Contributor path (strong defaults)

```bash
# Python reference
pip install -e ".[dev]"
ruff check pkg drivers client test conformance examples
pytest test/unit -q
pytest test/e2e conformance/ -q

# Go primary
cd go
go test ./... -count=1
go test -race ./trajir/client/... ./trajir/log/... ./trajir/resume/... -count=1
govulncheck ./...
```

Local live stack (optional): [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md).

## Pinned install tools (Scorecard Pinned-Dependencies)

GitHub Actions steps are SHA-pinned (#167). CLI tools installed inside jobs are
version-pinned (and release tarballs checksummed where we download binaries):

| Tool | Where | Pin style |
|------|-------|-----------|
| gitleaks | `security-scan.yml` | version + sha256 of linux_x64 tarball |
| actionlint | `security-scan.yml` | `go install …@v1.7.7` |
| zizmor | `security-scan.yml` | `pip install zizmor==…` |
| pip-licenses | `security-scan.yml` | exact PyPI version |
| go-licenses | `security-scan.yml` | `go install github.com/google/go-licenses/v2@v2.0.1` |
| govulncheck | `ci.yml` | `go install …@v1.7.0` |
| dco-check / pip-audit / build / twine | `ci.yml` / `release.yml` | exact PyPI versions |
| syft | `release.yml` | version + sha256 of linux_amd64 tarball |

### Intentional exceptions

- `pip install -e ".[dev]"` / `".[postgres,s3]"` installs the **project tree** from
  `pyproject.toml`. Runtime deps move via Dependabot (`pip` ecosystem), not
  per-workflow hash digests. Full `requirements.txt` hash pinning for every
  transitive wheel is deferred until we adopt a lockfile workflow.
- `pip install --upgrade pip` stays unpinned: we want the installer current on
  the runner image; the packages we care about are pinned above.

Track remaining Scorecard work under
[OpenSSF security bar](https://github.com/Coder-s-OG-s/Trajectory-IR/milestone/10)
(#375 and follow-ons).

## Follow-up backlog (issues under Phase CI/CD harden)

1. ~~**SHA-pin GitHub Actions**~~ — done (#167): digests + version comments in workflows
2. ~~**Require** gitleaks + actionlint on `main`~~ — done (#168)
3. **SLSA provenance** / cosign sign release artifacts (when maintainers pick a key strategy)
4. **Fork PR approval** settings: require approval for first-time contributors (org setting)
5. **zizmor fail-closed** after reviewing residual findings
6. ~~**Pin workflow CLI installs**~~ — in progress (#375): versions + checksums above

Active OpenSSF work tracks under milestone
[OpenSSF security bar](https://github.com/Coder-s-OG-s/Trajectory-IR/milestone/10).
Project Security Insights metadata: root [`SECURITY_INSIGHTS.yml`](../SECURITY_INSIGHTS.yml).

## References

- [CNCF: Securing GitHub Actions CI dependencies (recipe card)](https://www.cncf.io/blog/2026/05/04/securing-github-actions-ci-dependencies-recipe-card/)
- [CNCF: Securing CI/CD for OSS — access control (Cilium series)](https://www.cncf.io/blog/2026/06/04/securing-ci-cd-for-an-open-source-project-controlling-who-runs-what/)
- [CNCF TAG Security — Software Supply Chain Best Practices](https://tag-security.cncf.io/community/working-groups/supply-chain-security/supply-chain-security-paper/sscsp/)
- [OpenSSF Scorecard](https://scorecard.dev/)
