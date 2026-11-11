import { extractWikiLinks, resolveNoteByTitle, outgoingLinks, backlinksFor } from '../src/lib/wiki.ts';
import type { Note } from '../src/lib/api/types.ts';

const note = (id: string, title: string, content: string, updatedAt = '2026-01-01T00:00:00Z'): Note => ({
  id,
  title,
  content,
  isPinned: false,
  tags: [],
  createdAt: updatedAt,
  updatedAt
});

const a = note('a', 'Cours algo', 'Voir [[Structures de données]] et [[Graphes|le graphe]].');
const b = note('b', 'Structures de données', 'Contenu.');
const c = note('c', 'Graphes', 'Renvoie vers [[cours algo]] et [[Vraiment absente]].');
const dup1 = note('d1', 'Doublon', '', '2026-01-01T00:00:00Z');
const dup2 = note('d2', 'Doublon', '', '2026-02-01T00:00:00Z');
const e = note('e', 'Auto', 'Se cite [[Auto]] elle-même.');

const all = [a, b, c, dup1, dup2, e];

const checks: [string, boolean][] = [
  ['extraction simple et alias', extractWikiLinks(a.content).length === 2 && extractWikiLinks(a.content)[1].label === 'le graphe'],
  ['extraction sans doublon insensible à la casse', extractWikiLinks('[[X]] [[x]]').length === 1],
  ['extraction ignore les non fermés', extractWikiLinks('[[ouvert').length === 0],
  ['résolution insensible à la casse', resolveNoteByTitle('  cours ALGO ', all)?.id === 'a'],
  ['résolution doublon → plus récent', resolveNoteByTitle('Doublon', all)?.id === 'd2'],
  ['cible inconnue → null', resolveNoteByTitle('Rien', all) === null],
  ['liens sortants résolus', outgoingLinks(a, all).map((o) => o.note.id).join(',') === 'b,c'],
  ['lien mort exclu des sortants', outgoingLinks(c, all).map((o) => o.note.id).join(',') === 'a'],
  ['backlinks de b', backlinksFor(b, all).map((n) => n.id).join(',') === 'a'],
  ['backlinks insensible à la casse', backlinksFor(a, all).map((n) => n.id).includes('c')],
  ['self-lien ignoré', backlinksFor(e, all).length === 0]
];

let ok = true;
for (const [name, pass] of checks) {
  console.log(`${pass ? 'PASS' : 'FAIL'} — ${name}`);
  if (!pass) ok = false;
}
process.exit(ok ? 0 : 1);
