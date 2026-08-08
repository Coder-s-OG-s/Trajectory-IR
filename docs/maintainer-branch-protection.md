# Maintainer note: branch protection on main

Do this after CI has produced green check names on `main` or on a PR.

## GitHub plan note (important)

This repository is currently **private**. On GitHub’s free plan for private
repos, **classic branch protection** and **repository rulesets** return HTTP
403 (“Upgrade to GitHub Pro or make this repository public”).

To enable step 1 of post-v0.1.0 stabilization, pick one:

1. **Make the repo public** (free branch protection + free secret scanning for public repos), or  
2. **Upgrade the org/user to a plan that includes branch protection on private repos** (GitHub Pro / Team / Enterprise as applicable).

Until then, enforce process by policy: only merge green PRs, prefer squash, and use **Update branch** (repo setting `allow_update_branch` is enabled).

## Recommended settings

GitHub → Settings → Branches → Branch protection rule for `main` (after plan allows it):

1. Require a pull request before merging
2. Require approvals (at least 1 when more than one maintainer is active; 0 is OK for a solo maintainer if status checks are strict)
3. Require status checks to pass before merging
4. **Require branches to be up to date before merging** (strongly recommended; avoids the multi-PR conflict pile after `main` moves)
5. Do not allow force pushes or deleting `main`

## Status checks to require

Match the names shown in the Actions UI (Phase A, issue #81). Fast gate and
deep gate are both required once protection is on (issue #128 part 2 split
`Quality` so its e2e/conformance steps run as their own deep gate job):

Fast gate:

1. `DCO` (pull requests only; may appear after the first PR with the DCO job)
2. `Quality (Python 3.11)`: ruff, mypy, unit coverage floor
3. `Quality (Python 3.12)`: same as 3.11
4. `Package smoke`: `python -m build` + install wheel + import smoke
5. `Security (pip-audit)`: first-class dependency audit (not only a unit test)
6. `Go`: trajir coverage floor, full `go test ./...`, `govulncheck`

Deep gate:

7. `Conformance & E2E (Python 3.11)`: e2e crash/resume, full `conformance/` R01-R08
8. `Conformance & E2E (Python 3.12)`: same as 3.11

Optional after Phase B is proven stable (issue #85):

9. `Integration (Postgres)`
10. `Integration (MinIO)`

If GitHub shows a slightly different label, use the exact string from the check run.

Also enable org/repo **secret scanning** and **push protection** when available
(Settings → Code security). On **private** free-tier repos, secret scanning is
often unavailable without GitHub Advanced Security; **Dependabot alerts** and
**Dependabot security updates** can still be enabled (and were enabled for this
repo where the API allows).

## Coverage floors (workflow env)

Set in `.github/workflows/ci.yml` as `env:`:

| Variable | Default | Applies to |
|----------|---------|------------|
| `PYTHON_COV_FAIL_UNDER` | `80` | unit suite on `trajectory_ir` + `drivers` + `client` |
| `GO_COV_FAIL_UNDER` | `50` | `go/trajir/...` via `scripts/check_go_coverage.sh` |

Raise only after a measured green baseline; do not drop floors without a PR.

## Post-feature landing checklist

After large merges (packages, conformance, security):

1. Latest `main` CI is green (all six check families above).
2. Required checks still match the names above (rename jobs only with a protection update).
3. Close GitHub issues that already shipped (link the merge PR).
4. Skim [QUICKSTART.md](../QUICKSTART.md) and [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md) for fictional APIs.
5. Prefer Dependabot PRs reviewed weekly; do not bulk-merge without CI.

## Order

1. Merge a PR that defines or updates `.github/workflows/ci.yml`
2. Confirm a green run so check names exist (including `Package smoke` and `Security (pip-audit)`)
3. Enable the protection rule, select those checks, enable **up to date with main**
4. Confirm a test PR cannot merge with a failing DCO, Quality, Package smoke, Security, or Go job

## CLI (optional)

With admin rights on the repo:

```bash
gh api -X PUT repos/Coder-s-OG-s/Trajectory-IR/branches/main/protection \
  -H "Accept: application/vnd.github+json" \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "DCO",
      "Quality (Python 3.11)",
      "Quality (Python 3.12)",
      "Package smoke",
      "Security (pip-audit)",
      "Go",
      "Conformance & E2E (Python 3.11)",
      "Conformance & E2E (Python 3.12)"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
```

`strict: true` is “require branches to be up to date before merging.” Adjust review count when the team grows.
