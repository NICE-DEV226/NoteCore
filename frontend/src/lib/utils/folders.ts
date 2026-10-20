/**
 * Arborescence des dossiers (ADR-009) : fonctions pures, testables sans
 * interface. Le backend garantit l'absence de boucle ; ces fonctions s'en
 * protègent quand même (données corrompues, restauration partielle).
 */

import type { Folder, Note } from '../api/types';

export interface FolderNode {
  folder: Folder;
  depth: number;
  children: FolderNode[];
  /** Notes directement dans ce dossier. */
  own: number;
  /** Notes dans ce dossier et tous ses sous-dossiers. */
  total: number;
}

const MAX_DEPTH = 12;

const byName = (a: Folder, b: Folder): number => a.name.localeCompare(b.name, 'fr', { sensitivity: 'base' });

/** Construit l'arbre ; un dossier dont le parent est introuvable remonte à la racine. */
export function buildTree(folders: Folder[], notes: Pick<Note, 'folderId'>[]): FolderNode[] {
  const ids = new Set(folders.map((f) => f.id));
  const kids = new Map<string, Folder[]>();
  for (const f of folders) {
    const parent = f.parentId && ids.has(f.parentId) && f.parentId !== f.id ? f.parentId : '';
    (kids.get(parent) ?? kids.set(parent, []).get(parent)!).push(f);
  }
  const own = new Map<string, number>();
  for (const n of notes) if (n.folderId) own.set(n.folderId, (own.get(n.folderId) ?? 0) + 1);

  const seen = new Set<string>();
  const make = (f: Folder, depth: number): FolderNode => {
    seen.add(f.id);
    const children =
      depth >= MAX_DEPTH
        ? []
        : (kids.get(f.id) ?? [])
            .filter((c) => !seen.has(c.id))
            .sort(byName)
            .map((c) => make(c, depth + 1));
    const mine = own.get(f.id) ?? 0;
    return { folder: f, depth, children, own: mine, total: mine + children.reduce((s, c) => s + c.total, 0) };
  };
  const roots = (kids.get('') ?? []).sort(byName).map((f) => make(f, 0));
  // Un dossier pris dans une boucle de données corrompues n'a pas de racine :
  // on le rattache quand même, pour qu'aucune note ne devienne inaccessible.
  for (const f of [...folders].sort(byName)) if (!seen.has(f.id)) roots.push(make(f, 0));
  return roots;
}

/** Aplatit l'arbre en ne descendant que dans les dossiers dépliés. */
export function visibleNodes(roots: FolderNode[], expanded: ReadonlySet<string>): FolderNode[] {
  const out: FolderNode[] = [];
  const walk = (nodes: FolderNode[]): void => {
    for (const n of nodes) {
      out.push(n);
      if (n.children.length > 0 && expanded.has(n.folder.id)) walk(n.children);
    }
  };
  walk(roots);
  return out;
}

/** Identifiants du dossier et de tous ses descendants. */
export function subtreeIds(folders: Folder[], id: string): Set<string> {
  const out = new Set<string>([id]);
  let grew = true;
  for (let pass = 0; grew && pass <= MAX_DEPTH + 1; pass++) {
    grew = false;
    for (const f of folders) {
      if (f.parentId && out.has(f.parentId) && !out.has(f.id)) {
        out.add(f.id);
        grew = true;
      }
    }
  }
  return out;
}

/** Noms de la racine jusqu'au dossier (vide si inconnu). */
export function folderPath(folders: Folder[], id: string | null | undefined): string[] {
  const byId = new Map(folders.map((f) => [f.id, f]));
  const rev: string[] = [];
  for (let cur = id ?? '', i = 0; cur && i <= MAX_DEPTH + 1; i++) {
    const f = byId.get(cur);
    if (!f) break;
    rev.push(f.name);
    cur = f.parentId ?? '';
  }
  return rev.reverse();
}

export const pathLabel = (path: string[]): string => path.join(' › ');

/** Ancêtres d'un dossier, du plus proche au plus éloigné (pour les déplier). */
export function ancestorIds(folders: Folder[], id: string): string[] {
  const byId = new Map(folders.map((f) => [f.id, f]));
  const out: string[] = [];
  for (let cur = byId.get(id)?.parentId ?? '', i = 0; cur && i <= MAX_DEPTH + 1; i++) {
    out.push(cur);
    cur = byId.get(cur)?.parentId ?? '';
  }
  return out;
}

/** Un dossier peut-il être déposé sur `target` ? (pas dans lui-même ni son sous-arbre) */
export function canDropFolder(folders: Folder[], dragged: string, target: string | null): boolean {
  if (target === null) return true;
  return !subtreeIds(folders, dragged).has(target);
}
