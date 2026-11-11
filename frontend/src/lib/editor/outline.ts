/**
 * Outline partagé entre le mode Brainstorm (éditeur) et la vue Mind map :
 * une note = une liste de puces Markdown « - texte » indentées de deux
 * espaces par niveau. Le format reste standard — FTS, versions, export.
 */

export interface OutlineNode {
  /** Identifiant stable dans la session (attribué au parse). */
  id: number;
  depth: number;
  text: string;
}

/** Découpe le contenu en nœuds ; les lignes vides et non-puces sont ignorées. */
export function parseOutline(text: string, nextId: () => number): OutlineNode[] {
  const out: OutlineNode[] = [];
  for (const line of text.split('\n')) {
    if (!line.trim()) continue;
    const m = /^(\s*)(?:[-*+]\s+)?(.*)$/.exec(line);
    if (!m) continue;
    const indent = m[1].replace(/\t/g, '  ').length;
    out.push({ id: nextId(), depth: Math.floor(indent / 2), text: m[2].trimEnd() });
  }
  if (out.length === 0) out.push({ id: nextId(), depth: 0, text: '' });
  return out;
}

export function serializeOutline(nodes: OutlineNode[]): string {
  return nodes.map((n) => `${'  '.repeat(n.depth)}- ${n.text}`).join('\n');
}

/** Case à cocher en tête de puce : « [ ] » / « [x] ». */
export function checkboxState(text: string): 'none' | 'todo' | 'done' {
  if (/^\[ \]\s*/.test(text)) return 'todo';
  if (/^\[[xX]\]\s*/.test(text)) return 'done';
  return 'none';
}

/** Texte affiché sans le marqueur de case. */
export function taskText(text: string): string {
  const state = checkboxState(text);
  if (state === 'todo') return text.replace(/^\[ \]\s*/, '');
  if (state === 'done') return text.replace(/^\[[xX]\]\s*/, '');
  return text;
}

export interface OutlineTree {
  node: OutlineNode;
  children: OutlineTree[];
}

/** Reconstruit l'arbre depuis la liste plate (profondeurs absolues). */
export function buildTree(nodes: OutlineNode[]): OutlineTree[] {
  const roots: OutlineTree[] = [];
  const stack: { depth: number; children: OutlineTree[] }[] = [{ depth: -1, children: roots }];
  for (const node of nodes) {
    while (stack.length > 1 && stack[stack.length - 1].depth >= node.depth) stack.pop();
    const entry: OutlineTree = { node, children: [] };
    stack[stack.length - 1].children.push(entry);
    stack.push({ depth: node.depth, children: entry.children });
  }
  return roots;
}
