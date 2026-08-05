# Release process (maintainers)

How to cut a GitHub release after the release-prep PR is merged. **Do not** push tags from unreviewed automation.

## Preconditions

1. `main` is green (DCO not required on push to main if merges only; Quality + Go green on the merge commit / latest PR).
2. Release-prep PR merged (CHANGELOG has a version section; this file is up to date).
3. Working tree clean; `git pull origin main`.

## Version numbers

| Surface | Where | Notes |
|---------|--------|--------|
| Python package | `pyproject.toml` → `project.version` | Currently `0.1.0` |
| Git tag | `v0.1.0` | Prefer `v` prefix |
| Go module | `go/go.mod` | No separate semver tag yet; consumers use commit or future `go/v0.1.0` policy if we split versioning later |

## Steps for v0.1.0

```bash
git checkout main
git pull origin main

# Annotated tag (example date; use actual merge day if different)
git tag -a v0.1.0 -m "Trajectory IR v0.1.0 — Phase 1A library surface (R01–R08, dual .tir)"

git push origin v0.1.0
```

Create a **GitHub Release** for `v0.1.0`:

1. Title: `v0.1.0`
2. Body: paste or adapt [RELEASE_NOTES_0.1.0.md](RELEASE_NOTES_0.1.0.md)
3. Attach nothing required (source zip is automatic)

Optional PyPI (when ready; not required to call the tag a release):

```bash
pip install build twine
python -m build
# twine upload dist/*   # only with package owner credentials
```

## After the release

1. Confirm the GitHub Release page renders.
2. If something critical was missed, ship `0.1.1` rather than moving the tag.
3. Open issues for next work (client integration, FS CAS, etc.) under a clean Unreleased section.

## Checklist

- [ ] CHANGELOG `[0.1.0]` section accurate
- [ ] Tag pushed
- [ ] GitHub Release published
- [ ] Announce (Discord/org) with install-from-git or PyPI link
