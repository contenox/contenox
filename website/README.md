# contenox.com

Static site for contenox.com, built with Astro. The site owns no content:
every page under `/docs/`, `/cookbook/`, and `/stories/` renders markdown from
this repo's `docs/` tree (see `src/content.config.ts`). Editing a doc there is
editing the website.

```bash
make deps-website      # npm ci
make dev-website       # local dev server with live reload
make build-website     # static output -> website/dist
make preview-website   # build + serve the built output
```

`docs/` is the site's whole content source: `guide/`, `chains/`, `tools/`,
`reference/`, `cookbook/`, `stories/` publish today; `development/` (contributor
docs) and `development/blueprints/` (design records) are public and can join the publish
set by extending the collection patterns. Internal working notes never live in
`docs/` — they go to the gitignored `.notes/` at the repo root.

Conventions:

- Frontmatter (`title`, `description`) is optional; pages without it fall back
  to their first heading. Set `draft: true` to keep a doc out of the build.
- `public/` is served at the site root; `public/install.sh` must stay
  URL-stable (`https://contenox.com/install.sh` is referenced by docs and the
  install instructions).
- Deployment (CI push of `dist/` to a GitHub Pages repo) is not wired yet;
  builds are local-only.
