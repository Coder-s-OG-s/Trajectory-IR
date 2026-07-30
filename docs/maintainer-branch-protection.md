# Maintainer note: branch protection on main

Do this after the CI workflow has run green at least once on main or on a PR.

## Recommended settings

GitHub → Settings → Branches → Branch protection rule for `main`:

1. Require a pull request before merging
2. Require approvals (at least 1 when more than one maintainer is active)
3. Require status checks to pass before merging
4. Optionally require branches to be up to date before merging

## Status checks to require

Match the names shown in the Actions UI after CI runs. Expect at least:

1. `DCO`
2. `Quality (Python 3.11)`
3. `Quality (Python 3.12)`

## Order

1. Merge the PR that defines `.github/workflows/ci.yml`
2. Confirm a green run so check names exist
3. Enable the protection rule and select those checks
4. Confirm a test PR cannot merge with a failing DCO or quality job
