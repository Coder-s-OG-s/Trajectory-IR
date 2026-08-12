# Trajectory IR v0.2.1

Phase 1C patch after **v0.2.0** (Go primary). No breaking IR API changes.

## Highlights

- **Branch protection on `main`** with required CI checks (public repo).
- **Live integration sandbox**: `docker-compose.live.yml` + smoke scripts
  (`scripts/run_live_matrix.sh` / `.ps1`).
- **Release workflow proof**: this tag should attach Python sdist + wheel via
  `.github/workflows/release.yml` (#159).

## Install

```bash
# from git tag
pip install "git+https://github.com/Coder-s-OG-s/Trajectory-IR.git@v0.2.1"

# or download wheel/sdist from the GitHub Release assets once the Release
# workflow finishes
```

## Verify release assets

```bash
gh run list --workflow=release.yml --limit 3
gh release view v0.2.1 --json assets,url
```

## Docs

- [CHANGELOG.md](../CHANGELOG.md) `[0.2.1]`
- [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md)
- [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md)
- [RELEASE.md](RELEASE.md)
