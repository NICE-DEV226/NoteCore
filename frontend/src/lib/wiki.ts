/**
 * Liens « wiki » `[[titre de note]]` et backlinks, calculés côté client :
 * le store `notes` contient déjà toutes les notes actives avec leur
 * contenu, aucune requête supplémentaire n'est nécessaire.
 *
 * Sémantique : la cible est le TITRE de la note, comparé sans tenir
 * compte de la casse et des espaces de bord. `[[titre|alias]]` affiche
 * l'alias. Un doublon de titre se résout vers la note la plus
 * récemment modifiée — déterministe, et cohérent entre le lien et le
 * backlink (les deux passent par la même fonction de résolution).
 */
import type { Note } from './api/types';

export interface WikiLink {
  /** Cible brute, telle qu'écrite entre les crochets. */
  target: string;
  /** Texte affiché : l'alias s'il est fourni, sinon la cible. */
  label: string;
}

// [[cible]] ou [[cible|alias]] — pas de saut de ligne ni de crochet dans
// la cible (un titre contenant « ] » ne peut pas être lié, limitation
// assumée et documentée).
const WIKI_RE = /\[\[([^\][|\n]+?)(?:\|([^\][\n]*?))?\]\]/g;

/** Toutes les cibles `[[…]]` d'un contenu, dans l'ordre, sans doublon. */
export function extractWikiLinks(content: string): WikiLink[] {
  const seen = new Set<string>();
  const out: WikiLink[] = [];
  for (const m of content.matchAll(WIKI_RE)) {
    const target = m[1].trim();
    if (!target || seen.has(target.toLowerCase())) continue;
    seen.add(target.toLowerCase());
    out.push({ target, label: (m[2] ?? target).trim() || target });
  }
  return out;
}

/**
 * Résout une cible vers une note : titre identique insensible à la
 * casse, à quantité d'espaces de bord près. En cas d'homonymes, la note
 * la plus récemment modifiée gagne.
 */
export function resolveNoteByTitle(target: string, notes: Note[]): Note | null {
  const wanted = target.trim().toLowerCase();
  if (!wanted) return null;
  let best: Note | null = null;
  for (const n of notes) {
    if (n.title.trim().toLowerCase() !== wanted) continue;
    if (!best || n.updatedAt > best.updatedAt) best = n;
  }
  return best;
}

/** Liens sortants de `note` : cibles résolues vers les notes visées. */
export function outgoingLinks(note: Note, notes: Note[]): { link: WikiLink; note: Note }[] {
  const out: { link: WikiLink; note: Note }[] = [];
  for (const link of extractWikiLinks(note.content)) {
    const target = resolveNoteByTitle(link.target, notes);
    if (target) out.push({ link, note: target });
  }
  return out;
}

/**
 * Backlinks : les autres notes dont un `[[…]]` se résout vers `note`.
 * Le self-lien est ignoré (une note qui se cite elle-même n'apporte
 * rien). La comparaison porte sur l'identifiant résolu, pas sur le
 * texte : « [[ Ma note ]] » et « [[ma note]] » comptent pour la même.
 */
export function backlinksFor(note: Note, notes: Note[]): Note[] {
  const out: Note[] = [];
  for (const n of notes) {
    if (n.id === note.id) continue;
    const hit = extractWikiLinks(n.content).some(
      (l) => resolveNoteByTitle(l.target, notes)?.id === note.id
    );
    if (hit) out.push(n);
  }
  return out;
}
