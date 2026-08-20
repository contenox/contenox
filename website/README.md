# contenox.com

Static site for contenox.com, built with Astro. The site owns no content:
every page under `/docs/` renders markdown from this repo's `docs/` tree
(see `src/content.config.ts`). Editing a doc there is editing the website.

```bash
task website:deps      # npm ci
task website:dev       # local dev server with live reload
task website:build     # static output -> website/dist
task website:preview   # build + serve the built output
```

`docs/` is the site's whole content source: `guide/`, `specification/`,
`integrations/`, `reference/`, `use-cases/` and `rnd/` publish; `development/`
(contributor docs) is also published but kept out of the SEO/sitemap surface.
Internal working notes never live in `docs/` — they go to the gitignored
`.notes/` at the repo root.

Conventions:

- Frontmatter (`title`, `description`) is optional; pages without it fall back
  to their first heading. Set `draft: true` to keep a doc out of the build.
- `public/` is served at the site root; `public/install.sh` must stay
  URL-stable (`https://contenox.com/install.sh` is referenced by docs and the
  install instructions). Only small, URL-stable assets (install.sh, logos,
  favicons, the OG image) live here.
- Heavy media (demo gifs, screenshots, diagrams) is hosted on the website S3
  bucket. Docs markdown still references it root-relatively (`/hero.gif`);
  `src/lib/remark-md-links.mjs` rewrites those image paths to the bucket URL
  at build time — add new filenames to its `S3_MEDIA` set after uploading.
- contenox.com is deployed from this tree by the upstream repository's CI on
  every release that touches `docs/`, `schema/` or `website/`, built from
  `website/Dockerfile`. Nothing in this repository deploys it.
