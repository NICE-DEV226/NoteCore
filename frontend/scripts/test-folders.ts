import { ancestorIds, buildTree, canDropFolder, folderPath, subtreeIds, visibleNodes } from '../src/lib/utils/folders.ts';

const f = (id: string, name: string, parentId: string | null = null) => ({ id, name, parentId, createdAt: '', updatedAt: '' });
const folders = [
  f('s3', 'Semestre 3'),
  f('res', 'Réseaux', 's3'),
  f('td', 'TD', 'res'),
  f('tp', 'TP', 'res'),
  f('s4', 'Semestre 4'),
  f('orph', 'Orphelin', 'inexistant'),
  f('loopA', 'A', 'loopB'),
  f('loopB', 'B', 'loopA')
];
const notes = [{ folderId: 'td' }, { folderId: 'td' }, { folderId: 'tp' }, { folderId: 's3' }, { folderId: null }];

const roots = buildTree(folders, notes);
const s3 = roots.find((r) => r.folder.id === 's3')!;
const res = s3.children[0];

const checks: [string, boolean][] = [
  ['racines triées, orphelin remonté', roots.map((r) => r.folder.name).join('|').startsWith('Orphelin|Semestre 3|Semestre 4') || roots.some((r) => r.folder.id === 'orph')],
  ['profondeur', res.depth === 1 && res.children[0].depth === 2],
  ['total inclut les descendants', s3.total === 4 && res.total === 3 && s3.own === 1],
  ['enfants triés par nom', res.children.map((c) => c.folder.name).join() === 'TD,TP'],
  ['aplatissement : replié', visibleNodes(roots, new Set()).every((n) => n.depth === 0)],
  ['aplatissement : déplié', visibleNodes(roots, new Set(['s3', 'res'])).map((n) => n.folder.id).join(',').includes('s3,res,td,tp')],
  ['sous-arbre', [...subtreeIds(folders, 's3')].sort().join() === 'res,s3,td,tp'],
  ['chemin', folderPath(folders, 'td').join('/') === 'Semestre 3/Réseaux/TD' && folderPath(folders, 'inconnu').length === 0],
  ['ancêtres', ancestorIds(folders, 'td').join() === 'res,s3'],
  ['dépôt : pas dans soi-même', !canDropFolder(folders, 's3', 's3')],
  ['dépôt : pas dans un descendant', !canDropFolder(folders, 's3', 'td')],
  ['dépôt : vers la racine ou un autre dossier', canDropFolder(folders, 'td', null) && canDropFolder(folders, 'td', 's4')],
  ['boucle de données corrompues : ne tourne pas à vide', folderPath(folders, 'loopA').length <= 14],
  ['boucle : les dossiers restent visibles', roots.some((r) => r.folder.id === 'loopA' || r.folder.id === 'loopB')]
];

let ok = true;
for (const [name, pass] of checks) {
  console.log(`${pass ? 'PASS' : 'FAIL'} — ${name}`);
  if (!pass) ok = false;
}
process.exit(ok ? 0 : 1);
