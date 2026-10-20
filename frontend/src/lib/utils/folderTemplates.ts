/**
 * Modèles de matière (ADR-009) : une structure de dossiers décrite par un texte
 * indenté, que l'utilisateur peut modifier avant de la créer. Fonctions pures,
 * testables sans interface.
 *
 *   {nom}              ← « {nom} » est remplacé par le nom saisi
 *     Cours            ← chaque niveau d'indentation = un niveau de dossier
 *     TD
 *       Séance 1
 */

import type { FolderSpec } from '../api/types';

export interface FolderTemplate {
  id: string;
  label: string;
  hint: string;
  text: string;
  /** Modèle créé par l'utilisateur (supprimable). */
  custom?: boolean;
}

export const BUILTIN_TEMPLATES: FolderTemplate[] = [
  { id: 'matiere', label: 'Matière', hint: 'Cours, TD, TP, révisions, examens', text: '{nom}\n  Cours\n  TD\n  TP\n  Révisions\n  Examens' },
  { id: 'matiere-simple', label: 'Matière simple', hint: 'Cours, exercices, révisions', text: '{nom}\n  Cours\n  Exercices\n  Révisions' },
  { id: 'projet', label: 'Projet', hint: 'Cahier des charges à documentation', text: '{nom}\n  Cahier des charges\n  Conception\n  Développement\n  Documentation\n  Réunions' },
  { id: 'semestre', label: 'Semestre', hint: 'Plusieurs matières d\'un coup', text: '{nom}\n  Matière 1\n    Cours\n    TD\n  Matière 2\n    Cours\n    TD\n  Matière 3\n    Cours\n    TD' },
  { id: 'annee', label: 'Année', hint: 'Semestres', text: '{nom}\n  Semestre 1\n  Semestre 2' },
  { id: 'seul', label: 'Un dossier', hint: 'Juste le dossier', text: '{nom}' }
];

export const MAX_NODES = 200;
export const MAX_NAME = 100;
export const NAME_PLACEHOLDER = '{nom}';

export interface ParsedStructure {
  tree: FolderSpec[];
  total: number;
  /** Profondeur de la structure (un dossier seul : 1). */
  height: number;
  errors: string[];
}

const BULLET = /^(?:[-*•+]\s+)/;

/** Lit un texte indenté. `name` remplace « {nom} » ; `maxHeight` borne la profondeur. */
export function parseStructure(text: string, name: string, maxHeight = 8): ParsedStructure {
  const errors: string[] = [];
  const lines: { level: number; raw: string; n: number }[] = [];
  const src = text.replace(/\r\n?/g, '\n').split('\n');

  // Unité d'indentation : le plus petit retrait non nul (2 espaces par défaut).
  const indents = src
    .filter((l) => l.trim() !== '')
    .map((l) => (l.match(/^[ \t]*/)![0].replace(/\t/g, '  ')).length)
    .filter((n) => n > 0);
  const unit = indents.length > 0 ? Math.min(...indents) : 2;

  src.forEach((line, i) => {
    if (line.trim() === '') return;
    const indent = line.match(/^[ \t]*/)![0].replace(/\t/g, '  ').length;
    if (indent % unit !== 0) {
      errors.push(`Ligne ${i + 1} : indentation irrégulière (utilisez ${unit} espaces par niveau).`);
      return;
    }
    lines.push({ level: indent / unit, raw: line.trim().replace(BULLET, ''), n: i + 1 });
  });

  const tree: FolderSpec[] = [];
  const stack: FolderSpec[][] = [tree];
  let total = 0;
  let height = 0;
  let prev = -1;
  const needsName = text.includes(NAME_PLACEHOLDER);
  if (needsName && name.trim() === '') errors.push('Donnez un nom : le modèle en a besoin (à la place de « {nom} »).');

  for (const l of lines) {
    if (l.level > prev + 1) {
      errors.push(`Ligne ${l.n} : un niveau ne peut pas dépasser le précédent de plus d'un cran.`);
      continue;
    }
    const label = l.raw.split(NAME_PLACEHOLDER).join(name.trim()).trim();
    if (label === '') {
      errors.push(`Ligne ${l.n} : nom de dossier vide.`);
      continue;
    }
    if ([...label].length > MAX_NAME) {
      errors.push(`Ligne ${l.n} : nom trop long (${MAX_NAME} caractères au plus).`);
      continue;
    }
    const node: FolderSpec = { name: label };
    stack.length = l.level + 1;
    const siblings = stack[l.level];
    siblings.push(node);
    node.children = [];
    stack[l.level + 1] = node.children;
    prev = l.level;
    total += 1;
    height = Math.max(height, l.level + 1);
  }
  if (lines.length === 0 && errors.length === 0) errors.push('Le modèle est vide.');
  if (total > MAX_NODES) errors.push(`Trop de dossiers (${total}, ${MAX_NODES} au plus).`);
  if (height > maxHeight) errors.push(`Trop de niveaux (${height}) : ${maxHeight} au plus à cet endroit.`);

  const clean = (nodes: FolderSpec[]): FolderSpec[] =>
    nodes.map((n) => ({ name: n.name, ...(n.children && n.children.length ? { children: clean(n.children) } : {}) }));
  return { tree: clean(tree), total, height, errors };
}

/** Compte les dossiers de la structure qui existent déjà sous `parent` (réutilisés). */
export function countExisting(tree: FolderSpec[], siblingsOf: (path: string[]) => Set<string>, path: string[] = []): number {
  let n = 0;
  for (const node of tree) {
    const key = node.name.toLowerCase();
    const found = siblingsOf(path).has(key);
    if (found) n += 1;
    // Sous un dossier qui n'existe pas, rien ne peut exister.
    if (found && node.children) n += countExisting(node.children, siblingsOf, [...path, key]);
  }
  return n;
}

// --- modèles personnalisés : un par réglage (≤ 512 octets chacun) -----------------

export const CUSTOM_KEY_PREFIX = 'folders.tpl.';
export const MAX_CUSTOM = 6;
/** Taille maximale du texte d'un modèle enregistré (le réglage entier tient en 512 octets). */
export const MAX_CUSTOM_TEXT = 380;

export function encodeCustom(label: string, text: string): string {
  return JSON.stringify({ n: label.trim().slice(0, 30), t: text });
}

export function decodeCustom(key: string, raw: string): FolderTemplate | null {
  try {
    const v = JSON.parse(raw) as { n?: unknown; t?: unknown };
    if (typeof v.n === 'string' && typeof v.t === 'string' && v.n.trim() && v.t.trim()) {
      return { id: key, label: v.n, hint: 'Votre modèle', text: v.t, custom: true };
    }
  } catch {
    /* réglage illisible : ignoré */
  }
  return null;
}

/** Première clé libre parmi folders.tpl.0 … folders.tpl.5, ou null si tout est pris. */
export function freeCustomKey(used: Iterable<string>): string | null {
  const taken = new Set(used);
  for (let i = 0; i < MAX_CUSTOM; i++) if (!taken.has(`${CUSTOM_KEY_PREFIX}${i}`)) return `${CUSTOM_KEY_PREFIX}${i}`;
  return null;
}
