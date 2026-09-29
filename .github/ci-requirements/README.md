# CI tool pins (hash-locked)

These files pin **workflow CLI tools only** for OpenSSF Scorecard
Pinned-Dependencies. They are not the project runtime lockfile.

Regenerate (Python 3.12):

```bash
pip install pip-tools
echo 'dco-check==0.5.1' > /tmp/in.txt
pip-compile --generate-hashes --allow-unsafe -o dco-check.txt /tmp/in.txt
```

Same pattern for `build`, `pip-audit` (+ `setuptools`), `zizmor`,
`pip-licenses`, and `release-build` (`build` + `twine`).

Do **not** put `pip install -e ".[dev]"` hashes here ? that tree is owned by
`pyproject.toml` + Dependabot (documented exception in docs/CI_HARDENING.md).
