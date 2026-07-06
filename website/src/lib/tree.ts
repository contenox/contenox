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
  let node = root;
  for (const part of parts.slice(0, -1)) {
    let child = node.children.find((c) => c.name === part && !c.url);
    if (!child) {
      child = { name: part, children: [] };
      node.children.push(child);
    }
    node = child;
  }
  node.children.push({ name: parts[parts.length - 1], url, title, children: [] });
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
    insert(root, ['cookbook', ...entry.id.split('/')], `/cookbook/${entry.id}/`, entryTitle(entry));
  }
  for (const entry of await getCollection('stories', published)) {
    insert(root, ['stories', ...entry.id.split('/')], `/stories/${entry.id}/`, entryTitle(entry));
  }
  sortTree(root);
  return root;
}
