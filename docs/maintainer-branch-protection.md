# Maintainer note: branch protection on main

This is settings work after the CI workflow in `.github/workflows/ci.yml` has
run green at least once on the default branch or on a PR.

## Recommended settings (GitHub → Settings → Branches → Branch protection rule)

Apply to `main`:

1. Require a pull request before merging
2. Require approvals: at least 1 when more than one maintainer is active
3. Require status checks to pass before merging
4. Require branches to be up to date before merging (optional but preferred)
5. Do not allow bypassing the above except for emergency break-glass accounts

## Status checks to require

After CI has appeared in the checks list, require at least:

1. `DCO`
2. `Quality (Python 3.11)`
3. `Quality (Python 3.12)`

Job names come from `.github/workflows/ci.yml`. If GitHub shows a slightly
different label, match what the Actions UI shows.

## Order of operations

1. Merge the PR that adds `.github/workflows/ci.yml`
2. Open a small follow-up PR or re-run CI so the check names exist
3. Turn on the protection rule and select those checks
4. Confirm a test PR cannot merge with a failing DCO or quality job

Private vulnerability reports stay on the Security tab; they are not replaced
by branch protection.
