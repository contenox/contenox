import type { APIRoute } from 'astro';
import { getCollection } from 'astro:content';

// The previous contenox.com generations were indexed (and scraped) with
// `.html`-suffixed URLs. Emit a real redirect page for every such URL so
// legacy links resolve with a 200 + canonical instead of a 404.
// Entries whose id ends in `index` are skipped: their `.html` path is the
// file Astro already writes for the directory route.

type Target = { legacy: string; target: string };

export async function getStaticPaths() {
  const paths: { params: { legacy: string }; props: { target: string } }[] = [];
  const add = (legacy: string, target: string) => {
    paths.push({ params: { legacy }, props: { target } });
  };

  add('docs.html', '/docs/');
  add('cookbook.html', '/docs/use-cases/');
  add('stories.html', '/docs/use-cases/');
  add('de.html', '/de/');
  add('features.html', '/docs/guide/quickstart/');
  add('legal.html', '/legal/');
  for (const retired of [
    'pricing', 'services', 'cloud', 'login', 'signup', 'forgot-password',
    'reset-password', 'invite', 'admin', 'bob', 'pilot', 'about',
  ]) {
    add(`${retired}.html`, '/');
  }

  // Docs retired with the terminal-first V1 (Beam web UI, VS Code extension,
  // modeld local inference). Their pages are gone; the old URLs land on the
  // retired-blueprints index that records where each surface went.
  const retiredDocs = [
    'guide/beam',
    'integrations/editors/vscode-vscodium',
    'integrations/providers/modeld',
    'integrations/providers/modeld-architecture',
    'integrations/providers/local-models',
    'development/api_spec_generation',
    'development/beam-serve-auth',
    'development/modeld-llama-backend',
    'development/modeld-local-inference-landscape',
    'development/modeld-release-runbook',
    'development/modeld-source-build',
  ];
  for (const id of retiredDocs) {
    add(`docs/${id}.html`, '/docs/development/blueprints/retired/readme/');
  }

  const skip = (id: string) =>
    id === 'index' || id.endsWith('/index') || retiredDocs.includes(id);
  for (const entry of await getCollection('docs')) {
    if (!skip(entry.id)) add(`docs/${entry.id}.html`, `/docs/${entry.id}/`);
  }
  return paths;
}

export const GET: APIRoute<Target> = ({ props }) => {
  const target = new URL(props.target, 'https://contenox.com');
  const html = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Redirecting to ${target}</title>
<meta http-equiv="refresh" content="0;url=${props.target}">
<link rel="canonical" href="${target}">
<meta name="robots" content="noindex">
</head>
<body>
<p>This page has moved to <a href="${props.target}">${target}</a>.</p>
</body>
</html>
`;
  return new Response(html, { headers: { 'Content-Type': 'text/html; charset=utf-8' } });
};
