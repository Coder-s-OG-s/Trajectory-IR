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

`.github/workflows/docs-site.yml` is the file Actions runs. `website/docs-site.workflow.yml` is the same text, kept here because some OAuth tokens cannot push `.github/workflows/*` without the `workflow` scope. Keep the two copies the same. The live file sets top-level `contents: read`, pins the pip install with `website/requirements.lock`, and gives each git ref its own Pages concurrency group.

```bash
cp website/docs-site.workflow.yml .github/workflows/docs-site.yml
git add .github/workflows/docs-site.yml website/docs-site.workflow.yml
git commit -m "ci: enable docs site GitHub Pages workflow"
git push
```

Or refresh credentials first:

```bash
gh auth refresh -h github.com -s workflow
```

Then enable **Settings → Pages → Build and deployment → GitHub Actions**.

Expected URL shape: `https://coder-s-og-s.github.io/Trajectory-IR/`
