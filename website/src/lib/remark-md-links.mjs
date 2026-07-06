import { visit } from 'unist-util-visit';

// Rewrites relative `*.md` links in content to their rendered routes.
// A file at docs/a/b.md renders at /docs/a/b/ (one extra path segment), so a
// file-relative link needs one leading `../` to stay correct from the page URL.
export default function remarkMdLinks() {
  return (tree) => {
    visit(tree, 'link', (node) => {
      const url = node.url ?? '';
      if (/^(https?:)?\/\//.test(url) || url.startsWith('/') || url.startsWith('#') || url.startsWith('mailto:')) return;
      if (!/\.md(#|$)/.test(url)) return;
      // Astro's glob loader slugifies path segments (README.md -> readme/),
      // so lowercase the path part to match the generated routes.
      const [path, anchor] = url.split('#');
      node.url = '../' + path.toLowerCase().replace(/\.md$/, '/') + (anchor ? `#${anchor}` : '');
    });
  };
}
