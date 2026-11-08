# Trajectory IR documentation / demo site

MkDocs Material site for conference demos and public documentation.

## Local preview

```bash
cd website
python -m pip install -r requirements.txt
mkdocs serve
```

Open `http://127.0.0.1:8000/`.

## Refresh demo fixtures

From repo root (requires Go):

```powershell
pwsh website/scripts/capture_demos.ps1
```

```bash
bash website/scripts/capture_demos.sh
```

## Deploy

The workflow builds the site when `website/` changes, and on a manual run. That build is the check. The upload step and the deploy job stay off unless the repository variable `ENABLE_PAGES` is exactly `true`. This repo does not set that variable, and GitHub Pages is not turned on, so a push to main does not publish a public site.

To publish, set `ENABLE_PAGES` to `true` and turn on Settings, Pages, Build and deployment, GitHub Actions. `website/mkdocs.yml` and `website/docs/talk/speaker-runbook.md` still name `https://coder-s-og-s.github.io/Trajectory-IR/`, and that address stays empty until both of those are done.

`.github/workflows/docs-site.yml` is the file Actions runs. `website/docs-site.workflow.yml` is the same text, kept here because some tokens cannot push `.github/workflows/*` without the `workflow` scope. Keep the two copies the same. Use a credential that has that scope, or push over SSH. The live file sets top-level `contents: read`, pins the pip install with `website/requirements.lock`, and gives each git ref its own concurrency group.

```bash
cp website/docs-site.workflow.yml .github/workflows/docs-site.yml
```
