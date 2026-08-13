# Maintainer note: branch protection on main

Do this after CI has produced green check names on `main` or on a PR.

## Status (issues #146 / #158, Phase 1C) — ENABLED

Applied 2026-08-13 on public repo `Coder-s-OG-s/Trajectory-IR` via classic branch
protection API. Re-check:

```bash
gh api repos/Coder-s-OG-s/Trajectory-IR/branches/main/protection \
  --jq '{strict: .required_status_checks.strict, contexts: .required_status_checks.contexts, force: .allow_force_pushes.enabled}'
```

Expected live settings:

| Setting | Value |
|---------|--------|
| Required PR reviews | yes (0 approving reviews for solo maintainer) |
| Dismiss stale reviews | yes |
| Required status checks | yes, **strict** (branch must be up to date with `main`) |
| Allow force pushes | **no** |
| Allow deletions | **no** |
| Enforce admins | false (admins may bypass in emergency; prefer not to) |

## Required status checks (live)

Match Actions job names exactly:

**CI workflow**

1. `DCO`
2. `Quality (Python 3.11)`
3. `Quality (Python 3.12)`
4. `Package smoke`
5. `Security (pip-audit)`
6. `Go`
7. `Conformance & E2E (Python 3.11)`
8. `Conformance & E2E (Python 3.12)`
9. `Integration (Postgres)`
10. `Integration (MinIO)`

**Security scan workflow** (#168)

11. `Secret scan (gitleaks)`
12. `Workflow lint (actionlint)`

If a job is renamed in `.github/workflows/*.yml`, update protection in the same PR.

### Action SHA pinning (#167)

Third-party and GitHub-owned Actions in workflows are pinned to full commit SHAs
with a version comment (e.g. `actions/checkout@3d3c42e5… # v7`). Dependabot
still updates the `github-actions` ecosystem weekly; review pin bumps carefully.

## Merge policy (historical while protection was blocked)

Section for #152 when `main` was ungated. **Machine gates are now on**; still
prefer squash merge, DCO on every commit, milestone on PRs, and no direct
product commits to `main`.

1. Do not merge unless required checks are green on the latest PR head.
2. Prefer **squash** merge; keep the branch **up to date** with `main`.
3. Require **Signed-off-by** / DCO on every commit.
4. **No force push** to `main` (blocked by protection).
5. Assign milestone **Phase 1C harden and adopt** (or Future) before merge.
6. After merge: confirm `main` is healthy; close shipped issues with the PR link.

## Recommended settings (checklist)

GitHub → Settings → Branches → Branch protection rule for `main`:

1. Require a pull request before merging
2. Require approvals (0 solo / 1+ when the team grows)
3. Require status checks to pass before merging (list above)
4. **Require branches to be up to date before merging** (`strict: true`)
5. Do not allow force pushes or deleting `main`

Also enable org/repo **secret scanning** and **push protection** when available
(Settings → Code security). **Dependabot alerts** / security updates should stay on.

## Coverage floors (workflow env)

Set in `.github/workflows/ci.yml` as `env:`:

| Variable | Default | Applies to |
|----------|---------|------------|
| `PYTHON_COV_FAIL_UNDER` | `80` | unit suite on `trajectory_ir` + `drivers` + `client` |
| `GO_COV_FAIL_UNDER` | `80` | `go/trajir/...` except `durable/temporal` via `scripts/check_go_coverage.sh` |

Raise only after a measured green baseline; do not drop floors without a PR.

History (issue #131): 50 → 70 (unit baseline ~70.2%), then **80** with:
- sqlmock unit coverage for `trajir/postgres`
- floor packages exclude `durable/temporal` (optional Temporal cluster;
  covered by `temporal_integration` tests, not the default PR unit gate)

Full `go test ./trajir/...` still runs on every PR; only the **coverage floor**
package set omits Temporal.

## Post-feature landing checklist

After large merges (packages, conformance, security):

1. Latest `main` CI is green (required checks above).
2. Required checks still match the names above (rename jobs only with a protection update).
3. Close GitHub issues that already shipped (link the merge PR).
4. Skim [QUICKSTART.md](../QUICKSTART.md), [go/QUICKSTART.md](../go/QUICKSTART.md), and [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md) for fictional APIs.
5. Prefer Dependabot PRs reviewed weekly; do not bulk-merge without CI.

## Order (first enable)

1. Merge a PR that defines or updates `.github/workflows/ci.yml`
2. Confirm a green run so check names exist
3. Enable the protection rule (done for #146 / #158)
4. Confirm a test PR cannot merge with a failing required check

## CLI (re-apply or update)

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
      "Conformance & E2E (Python 3.12)",
      "Integration (Postgres)",
      "Integration (MinIO)",
      "Secret scan (gitleaks)",
      "Workflow lint (actionlint)"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 0
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
```

`strict: true` is “require branches to be up to date before merging.” Raise
`required_approving_review_count` when the team grows.
