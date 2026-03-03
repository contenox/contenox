# Website deployment (contenox.com)

## How it works

The live site **https://contenox.com** is built and deployed from this repo via GitHub Actions.

| Step | What happens |
|------|----------------|
| **Trigger** | Push to `main` |
| **Workflow** | [.github/workflows/docs.yml](../.github/workflows/docs.yml) |
| **Build** | 1. VitePress in `website-docs/` → output to `website/docs/`<br>2. `make docs-html` copies OpenAPI spec + openapi.html into `website/docs/`<br>3. `website/.gitignore` is removed so the built docs are included |
| **Deploy** | [peaceiris/actions-gh-pages](https://github.com/peaceiris/actions-gh-pages) pushes the **entire `./website` directory** to the external repo |
| **Target** | `contenox/contenox.github.io` (branch `main`) |
| **Domain** | `website/CNAME` contains `contenox.com` → GitHub Pages serves the site there |

So the **source of truth** for the live site is the `website/` directory in **this** repo (contenox/contenox). Whatever is in `website/` when the workflow runs is what gets pushed to the Pages repo.

## Why the site can look “old”

- The workflow deploys the current contents of `website/` on every push to `main`. If the live site shows outdated or “archived” content, it’s because that content is still in `website/` (and optionally `website-docs/`) in this repo.
- To update the live site: change the files under `website/` (and/or the VitePress sources in `website-docs/`), commit, and push to `main`. The next run of the workflow will publish the new content.

## Requirements

- **Secret:** `DEPLOY_KEY` must be set in this repo’s GitHub Actions secrets. It should be an SSH deploy key with **write** access to `contenox/contenox.github.io` (same key can be added as deploy key there).
- **Node:** workflow uses Node 24 for the VitePress build.
- **Go:** used for `make docs-html` (OpenAPI generation).

## Local preview

```bash
make website-install
make website-dev          # VitePress dev server (docs)
make vitepress-build      # build into website/docs
make docs-html            # add OpenAPI files under website/docs
# Then open website/ with a static server if you want to test the full site
```

## Other workflows

- **.github/workflows/dockerpublish.yml** also builds the docs and deploys to `contenox/contenox.github.io` as part of its job (same steps). So both `docs.yml` and `dockerpublish.yml` can update the website when they run.

## Repo rename note

If the GitHub Pages repo was renamed (e.g. from `contenox/contenox.github.io` to something else), update `external_repository` in both:

- `.github/workflows/docs.yml`
- `.github/workflows/dockerpublish.yml`
