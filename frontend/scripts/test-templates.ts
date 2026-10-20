import {
  BUILTIN_TEMPLATES,
  countExisting,
  decodeCustom,
  encodeCustom,
  freeCustomKey,
  MAX_CUSTOM_TEXT,
  parseStructure
} from '../src/lib/utils/folderTemplates.ts';

const names = (t: { name: string; children?: unknown[] }[]): string => t.map((n) => n.name).join('|');
const matiere = parseStructure(BUILTIN_TEMPLATES[0].text, 'Réseaux');
const semestre = parseStructure(BUILTIN_TEMPLATES.find((t) => t.id === 'semestre')!.text, 'Semestre 3');
const tabs = parseStructure('{nom}\n\tCours\n\t\tChap 1', 'X');

const checks: [string, boolean][] = [
  ['modèle Matière : racine nommée', matiere.errors.length === 0 && matiere.tree.length === 1 && matiere.tree[0].name === 'Réseaux'],
  ['modèle Matière : 5 sous-dossiers', names(matiere.tree[0].children!) === 'Cours|TD|TP|Révisions|Examens' && matiere.total === 6],
  ['hauteur', matiere.height === 2 && semestre.height === 3],
  ['semestre : sous-sous-dossiers', semestre.tree[0].children![0].children!.length === 2 && semestre.total === 10],
  ['tous les modèles intégrés sont valides', BUILTIN_TEMPLATES.every((t) => parseStructure(t.text, 'Test').errors.length === 0)],
  ['tabulations comme indentation', tabs.errors.length === 0 && tabs.tree[0].children![0].children![0].name === 'Chap 1'],
  ['puces ignorées', names(parseStructure('A\n  - B\n  * C', '').tree[0].children!) === 'B|C'],
  ['nom obligatoire avec {nom}', parseStructure('{nom}\n  Cours', '  ').errors.length > 0],
  ['sans {nom}, pas de nom exigé', parseStructure('Cours\nTD', '').errors.length === 0],
  ['indentation irrégulière refusée', parseStructure('A\n  B\n   C', '').errors.length > 0],
  ['saut de niveau refusé', parseStructure('A\n  B\n      C', '').errors.length > 0],
  ['modèle vide refusé', parseStructure('  \n\n', '').errors.length > 0],
  ['trop profond refusé', parseStructure('A\n  B\n    C', '', 2).errors.length > 0],
  ['trop de dossiers refusé', parseStructure(Array.from({ length: 201 }, (_, i) => `d${i}`).join('\n'), '').errors.length > 0],
  ['nom trop long refusé', parseStructure('x'.repeat(101), '').errors.length > 0],
  ['plusieurs racines', parseStructure('A\nB\n  C', '').tree.length === 2],
  ['dossiers existants comptés', countExisting(matiere.tree, (p) => (p.length === 0 ? new Set(['réseaux']) : p.join() === 'réseaux' ? new Set(['cours', 'td']) : new Set())) === 3],
  ['rien d\'existant sous un dossier absent', countExisting(matiere.tree, (p) => (p.length === 0 ? new Set() : new Set(['cours']))) === 0],
  ['modèle personnalisé : aller-retour', decodeCustom('folders.tpl.0', encodeCustom('Mon modèle', 'A\n  B'))?.text === 'A\n  B' && decodeCustom('k', 'pas du json') === null],
  ['modèle enregistré tient dans un réglage', encodeCustom('x'.repeat(40), 'a'.repeat(MAX_CUSTOM_TEXT)).length <= 512],
  ['clé libre', freeCustomKey(['folders.tpl.0', 'folders.tpl.2']) === 'folders.tpl.1' && freeCustomKey(Array.from({ length: 6 }, (_, i) => `folders.tpl.${i}`)) === null]
];

let ok = true;
for (const [name, pass] of checks) {
  console.log(`${pass ? 'PASS' : 'FAIL'} — ${name}`);
  if (!pass) ok = false;
}
process.exit(ok ? 0 : 1);
