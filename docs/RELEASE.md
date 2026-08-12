# Release process (maintainers)

How to cut a GitHub release and optionally publish to PyPI. **Do not** push tags
from unreviewed automation. Package owner credentials never belong in the repo.

## Automated dist on `v*` tags (#147 / #153)

Workflow: [`.github/workflows/release.yml`](../.github/workflows/release.yml)

On every push of a tag matching `v*`:

1. Build sdist + wheel (`python -m build`)
2. `twine check dist/*`
3. Upload Actions artifacts (`python-dist`)
4. Attach `dist/*` to the GitHub Release for that tag (`softprops/action-gh-release`)

### Verify after a tag push (#153 / #159)

```bash
# Workflow must be green
gh run list --workflow=release.yml --limit 5

# Release must list wheel + sdist assets (not empty)
gh release view vX.Y.Z --json assets,url
```

**History:** `v0.2.0` was cut as a Phase 1B library tag without release.yml
assets (empty asset list). **`v0.2.1`** is the Phase 1C proof cut for #159:
green `Release` workflow + non-empty wheel/sdist.

If the workflow is green but assets are missing, check that a GitHub Release
object exists for the tag (the action creates/updates it) and that
`permissions: contents: write` remains on the workflow.

## Preconditions

1. `main` is green (Quality + Go green on the merge commit / latest PR).
2. CHANGELOG has a version section that matches what you intend to ship.
3. Working tree clean; `git pull origin main`.
4. You have rights to push tags and (for PyPI) upload the project.
5. After the tag push: confirm **Release** workflow green and assets attached.

## Version numbers

| Surface | Where | Notes |
|---------|--------|--------|
| Python package | `pyproject.toml` → `project.version` | Current line: `0.2.1` (Phase 1C patch) |
| Git tag | `v0.2.0` Phase 1B; **`v0.2.1`** Phase 1C harden | Prefer `v` prefix |
| Go module | `go/go.mod` | No separate semver tag yet; consumers use commit or a later policy |

### Choosing 0.1.0 vs 0.1.1

`v0.1.0` is already tagged. Prefer **`v0.1.1`** for everything that landed after
that tag (Phase B CI, reliability fixes, optional adoption demo / E2E docs).

1. Keep `v0.1.0` as the first library surface snapshot.
2. Ship **`0.1.1`** when CHANGELOG `[Unreleased]` is accurate, optional fold-in
   PRs are merged or deferred, `pyproject.toml` version is bumped, and CI is green.
3. Draft notes: [RELEASE_NOTES_0.1.1.md](RELEASE_NOTES_0.1.1.md).

Do not move an already pushed tag. Prefer a new patch version.

## Steps for a GitHub release (example v0.2.0)

```bash
git checkout main
git pull origin main

# After version bump in pyproject.toml and CHANGELOG [0.2.0] section:
git tag -a v0.2.0 -m "Trajectory IR v0.2.0 Phase 1B Go primary SDK"

git push origin v0.2.0
```

Create or update a **GitHub Release** for the tag:

1. Title: `v0.2.0` (or the version you are cutting)
2. Body: paste or adapt [RELEASE_NOTES_0.2.0.md](RELEASE_NOTES_0.2.0.md)
   (older notes: [RELEASE_NOTES_0.1.1.md](RELEASE_NOTES_0.1.1.md), [RELEASE_NOTES_0.1.0.md](RELEASE_NOTES_0.1.0.md))
3. Prefer letting **release.yml** attach wheel + sdist automatically on tag push.
   Source zip remains automatic from GitHub.
4. Confirm assets: `gh release view vX.Y.Z --json assets`

### Historical: v0.1.0

```bash
git tag -a v0.1.0 -m "Trajectory IR v0.1.0 — Phase 1A library surface (R01–R08, dual .tir)"
git push origin v0.1.0
```

## Publish to PyPI (optional but tracked)

PyPI is **not** required for the git tag to be a valid release. When you are ready
to make `pip install trajectory-ir` work for outsiders:

### One time setup

1. Create or use a PyPI project named consistently with `pyproject.toml` (`trajectory-ir`).
2. Prefer **Trusted Publishing** (GitHub Actions OIDC) or a PyPI API token stored
   only in the package owner’s secret store. Never commit tokens.
3. Confirm the package name is still available (or you own it).

### Build and upload (manual)

From a clean checkout of the **tagged** commit:

```bash
git checkout v0.1.0
python -m venv .venv-release
# Windows: .\.venv-release\Scripts\activate
# Unix:    source .venv-release/bin/activate
pip install -U pip build twine
python -m build
# Inspect dist/ — sdist and wheel for trajectory-ir 0.1.0
twine check dist/*
# Upload only with owner credentials:
# twine upload dist/*
```

### Smoke after upload

```bash
python -m venv .venv-smoke
# activate venv
pip install trajectory-ir==0.1.0
python -c "import trajectory_ir; print('ok')"
# Optional: clone still needed for conformance suite paths; at minimum import works.
```

### Automation later

Trusted Publishing from GitHub Actions can replace manual twine. That is a follow
up workflow change, not required for the first upload.

## After the release

1. Confirm the GitHub Release page renders.
2. If something critical was missed, ship `0.1.1` rather than moving the tag.
3. When PyPI is live, update QUICKSTART install section to show `pip install trajectory-ir`
   as the primary path and keep the git clone path for contributors.
4. Open or continue issues under a clean Unreleased CHANGELOG section.

## Checklist

1. [ ] CHANGELOG section accurate for the version you tag
2. [ ] `pyproject.toml` version matches the tag (without the `v` prefix)
3. [ ] Annotated tag pushed
4. [ ] GitHub Release published
5. [ ] (Optional) PyPI wheel uploaded and smoke install works
6. [ ] Announce with install from git or PyPI link
