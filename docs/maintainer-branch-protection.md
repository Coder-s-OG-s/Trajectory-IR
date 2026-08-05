# Maintainer note: branch protection on main

Do this after CI has produced green check names on `main` or on a PR.

## Recommended settings

GitHub → Settings → Branches → Branch protection rule for `main`:

1. Require a pull request before merging
2. Require approvals (at least 1 when more than one maintainer is active; 0 is OK for a solo maintainer if status checks are strict)
3. Require status checks to pass before merging
4. Require branches to be up to date before merging (recommended)
5. Do not allow force pushes or deleting `main`

## Status checks to require

Match the names shown in the Actions UI:

1. `DCO` (pull requests only; may appear after the first PR with the DCO job)
2. `Quality (Python 3.11)` — includes `pip-audit` via unit tests
3. `Quality (Python 3.12)` — includes `pip-audit` via unit tests
4. `Go` (includes `govulncheck`)

If GitHub shows a slightly different label, use the exact string from the check run.

Also enable org/repo **secret scanning** and **push protection** when available (Settings → Code security).

## Post-feature landing checklist

After large merges (packages, conformance, security):

1. Latest `main` CI is green.
2. Required checks still match the names above (rename jobs only with a protection update).
3. Close GitHub issues that already shipped (link the merge PR).
4. Skim [QUICKSTART.md](../QUICKSTART.md) and [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md) for fictional APIs.
5. Prefer Dependabot PRs reviewed weekly; do not bulk-merge without CI.

## Order

1. Merge a PR that defines or updates `.github/workflows/ci.yml`
2. Confirm a green run so check names exist
3. Enable the protection rule and select those checks
4. Confirm a test PR cannot merge with a failing DCO, Quality, or Go job

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
      "Go"
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

Adjust review count when the team grows.
