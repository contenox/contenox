import { getCollection } from 'astro:content';
import { entryTitle, published } from './entries';

export interface TreeNode {
  name: string;
  /** Route url when this node is a page. */
  url?: string;
  title?: string;
  children: TreeNode[];
}

function insert(root: TreeNode, parts: string[], url: string, title: string) {
  const lastName = parts[parts.length - 1];
  const isIndex = lastName === 'index' || lastName === 'README';

  let node = root;
  // Walk to the parent of the final segment (or to the node itself for index files).
  const folderParts = isIndex ? parts.slice(0, -1) : parts.slice(0, -1);
  for (const part of folderParts) {
    // Match by name only — a file and a same-named directory cannot coexist
    // in the same level, so there's no ambiguity.
    let child = node.children.find((c) => c.name === part);
    if (!child) {
      child = { name: part, children: [] };
      node.children.push(child);
    }
    node = child;
  }

  if (isIndex) {
    // Promote the index URL + title to the folder node itself.
    node.url = url;
    if (!node.title) node.title = title;
  } else {
    node.children.push({ name: lastName, url, title, children: [] });
  }
}

function sortTree(node: TreeNode) {
  // Mirror `ls`: alphabetical, with README/index entries first inside a dir.
  node.children.sort((a, b) => {
    const rank = (n: TreeNode) => (n.name === 'README' || n.name === 'index' ? 0 : 1);
    return rank(a) - rank(b) || a.name.localeCompare(b.name);
  });
  node.children.forEach(sortTree);
}

/** Builds the sidebar tree mirroring the docs/ folder structure exactly. */
export async function buildDocsTree(): Promise<TreeNode> {
  const root: TreeNode = { name: '', children: [] };
  for (const entry of await getCollection('docs', published)) {
    insert(root, entry.id.split('/'), `/docs/${entry.id}/`, entryTitle(entry));
  }
  for (const entry of await getCollection('cookbook', published)) {
    const url = entry.id === 'index' ? '/cookbook/' : `/cookbook/${entry.id}/`;
    insert(root, ['cookbook', ...entry.id.split('/')], url, entryTitle(entry));
  }
  for (const entry of await getCollection('stories', published)) {
    const url = entry.id === 'index' ? '/stories/' : `/stories/${entry.id}/`;
    insert(root, ['stories', ...entry.id.split('/')], url, entryTitle(entry));
  }
  sortTree(root);
  return root;
}

/** Returns the top-level nav sections derived from the docs tree.
 *  Each entry has a name and the URL of the section index (or first child). */
export async function buildNavSections(): Promise<{ name: string; url: string }[]> {
  const tree = await buildDocsTree();
  return tree.children
    .filter((n) => n.url || n.children.length > 0)
    .map((n) => ({
      name: n.name,
      url: n.url ?? n.children[0]?.url ?? `/docs/${n.name}/`,
    }));
}
