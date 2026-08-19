# CI/CD hardening (CNCF-aligned)

How Trajectory IR strengthens contributor and maintainer CI after Phase 1C.
Grounded in CNCF TAG Security supply-chain guidance and the CNCF
[GitHub Actions CI dependency recipe card](https://www.cncf.io/blog/2026/05/04/securing-github-actions-ci-dependencies-recipe-card/).

Milestone: **[Phase CI/CD harden](https://github.com/Coder-s-OG-s/Trajectory-IR/milestone/6)**

## Principles (from CNCF / OpenSSF)

| Principle | Practice here |
|-----------|----------------|
| Least privilege | Workflows default `contents: read`; release only gets `contents: write` + `id-token` |
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

## Follow-up backlog (issues under Phase CI/CD harden)

1. ~~**SHA-pin GitHub Actions**~~ — done (#167): digests + version comments in workflows
2. ~~**Require** gitleaks + actionlint on `main`~~ — done (#168)
3. **SLSA provenance** / cosign sign release artifacts (when maintainers pick a key strategy)
4. **Fork PR approval** settings: require approval for first-time contributors (org setting)
5. **zizmor fail-closed** after reviewing residual findings

## References

- [CNCF: Securing GitHub Actions CI dependencies (recipe card)](https://www.cncf.io/blog/2026/05/04/securing-github-actions-ci-dependencies-recipe-card/)
- [CNCF: Securing CI/CD for OSS — access control (Cilium series)](https://www.cncf.io/blog/2026/06/04/securing-ci-cd-for-an-open-source-project-controlling-who-runs-what/)
- [CNCF TAG Security — Software Supply Chain Best Practices](https://tag-security.cncf.io/community/working-groups/supply-chain-security/supply-chain-security-paper/sscsp/)
- [OpenSSF Scorecard](https://scorecard.dev/)
