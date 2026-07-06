import { visit } from 'unist-util-visit';
import path from 'node:path';

// Rewrites relative links in content:
//  - `*.md` links become their rendered routes. A file at docs/a/b.md renders
//    at /docs/a/b/ (one extra path segment), so a file-relative link needs one
//    leading `../` to stay correct from the page URL. Path segments are
//    lowercased to match the glob loader's slugs (README.md -> readme/).
//  - other relative links (source files, configs) cannot exist on the static
//    site; they are rewritten to GitHub blob URLs resolved against the source
//    markdown file's repo path.
export default function remarkMdLinks() {
  return (tree, file) => {
    const abs = file?.history?.[0] ?? '';
    const marker = `${path.sep}docs${path.sep}`;
    const idx = abs.lastIndexOf(marker);
    const repoDir = idx === -1 ? null : path.posix.dirname('docs/' + abs.slice(idx + marker.length).split(path.sep).join('/'));

    visit(tree, 'link', (node) => {
      const url = node.url ?? '';
      if (/^(https?:)?\/\//.test(url) || url.startsWith('/') || url.startsWith('#') || url.startsWith('mailto:')) return;
      if (/\.md(#|$)/.test(url)) {
        const [p, anchor] = url.split('#');
        node.url = '../' + p.toLowerCase().replace(/\.md$/, '/') + (anchor ? `#${anchor}` : '');
        return;
      }
      if (repoDir) {
        const resolved = path.posix.normalize(path.posix.join(repoDir, url));
        if (!resolved.startsWith('..')) {
          node.url = `https://github.com/contenox/runtime/blob/main/${resolved}`;
        } else {
          // Climbs out of docs/ — a repo source path like ../../runtime/foo.go.
          const fromRepoRoot = path.posix.normalize(path.posix.join(repoDir.replace(/^docs/, ''), url)).replace(/^\/+/, '');
          node.url = `https://github.com/contenox/runtime/blob/main/${fromRepoRoot}`;
        }
      }
    });
  };
}
