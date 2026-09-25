import fs from 'node:fs';
import path from 'node:path';

export interface RouteEntry {
  path: string;
  category: 'core' | 'docs' | 'legal' | 'de';
  slug: string;
}

function scanDir(dir: string, baseDir: string = dir): string[] {
  if (!fs.existsSync(dir)) return [];
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  const files: string[] = [];

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...scanDir(fullPath, baseDir));
    } else if (entry.isFile() && entry.name.endsWith('.md')) {
      const rel = path.relative(baseDir, fullPath).replace(/\\/g, '/');
      files.push(rel);
    }
  }

  return files;
}

function isDraft(filePath: string): boolean {
  try {
    const content = fs.readFileSync(filePath, 'utf-8');
    if (!content.startsWith('---')) return false;
    const end = content.indexOf('---', 3);
    if (end === -1) return false;
    const frontmatter = content.slice(3, end);
    return /draft:\s*true/i.test(frontmatter);
  } catch {
    return false;
  }
}

export function discoverRoutes(): RouteEntry[] {
  const docsDir = path.resolve(__dirname, '../../docs');
  const routes: RouteEntry[] = [
    { path: '/', category: 'core', slug: 'home' },
    { path: '/de/', category: 'de', slug: 'de-home' },
    { path: '/legal', category: 'legal', slug: 'legal' },
  ];

  if (!fs.existsSync(docsDir)) {
    return routes;
  }

  const allDocs = scanDir(docsDir);

  for (const file of allDocs) {
    const fullPath = path.join(docsDir, file);
    if (isDraft(fullPath)) continue;

    if (file.startsWith('development/internal/')) {
      continue;
    }

    if (file.startsWith('de/')) {
      const id = file.slice(3).replace(/\.md$/, '').replace(/\/index$/, '');
      routes.push({
        path: `/de/${id}/`,
        category: 'de',
        slug: `de-${id.replace(/\//g, '-')}`,
      });
    } else if (file === 'legal.md') {
      // already included as /legal
      continue;
    } else if (file.startsWith('legal/')) {
      const id = file.replace(/\.md$/, '').replace(/\/index$/, '');
      routes.push({
        path: `/${id}/`,
        category: 'legal',
        slug: id.replace(/\//g, '-'),
      });
    } else {
      const id = file.replace(/\.md$/, '').replace(/\/index$/, '');
      routes.push({
        path: `/docs/${id}/`,
        category: 'docs',
        slug: `docs-${id.replace(/\//g, '-')}`,
      });
    }
  }

  return routes;
}
