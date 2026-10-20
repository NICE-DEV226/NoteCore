import type { NoteCoreApi } from './contract';
import type { Folder, MasteryRow, Note, NotePatch, ReviewCard, SearchHit } from './types';

/**
 * Implémentation en mémoire du contrat NoteCoreApi.
 * Existe uniquement tant que le backend Go n'est pas branché (phase 1
 * du cahier des charges). Les délais simulés exercent les états de
 * sauvegarde (`saved`, `dirty`, `saving`).
 */

let counter = 0;
function newId(prefix: string): string {
  counter += 1;
  return `${prefix}-${counter.toString().padStart(4, '0')}`;
}

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();
/** Petit délai simulé pour exercer les états d'interface. */
const delay = (ms = 120) => new Promise<void>((r) => setTimeout(r, ms));

/**
 * Réglages locaux (table `settings` en production). Le mock ne survit
 * pas au rechargement : c'est un substitut en mémoire, pas un stockage
 * durable.
 */
const settings: Record<string, string> = {};

/**
 * Pièces jointes en mémoire (table `attachments` en production). Les
 * octets restent en base64 pour refléter le transport réel.
 */
interface MockAttachment {
  id: string;
  noteId: string;
  filename: string;
  mime: string;
  size: number;
  createdAt: string;
  dataBase64: string;
}
const attachments: MockAttachment[] = [];

function makeFolder(name: string, ageMinutes: number): Folder {
  return {
    id: newId('folder'),
    name,
    createdAt: minutesAgo(ageMinutes),
    updatedAt: minutesAgo(ageMinutes)
  };
}

function makeNote(
  title: string,
  content: string,
  folderId: string | null,
  tags: string[],
  updatedMinutes: number,
  createdMinutes = updatedMinutes + 1440 * 3,
  isPinned = false
): Note {
  return {
    id: newId('note'),
    title,
    content,
    folderId,
    isPinned,
    tags,
    createdAt: minutesAgo(createdMinutes),
    updatedAt: minutesAgo(updatedMinutes)
  };
}

function buildSeed(): { notes: Note[]; folders: Folder[] } {
  const cours = makeFolder('Cours', 60 * 24 * 10);
  const projets = makeFolder('Projets', 60 * 24 * 9);
  const algo = makeFolder('Algorithmique', 60 * 24 * 7);

  const notes: Note[] = [
    makeNote(
      "Systèmes d'exploitation — séance 3",
      `# Processus et ordonnancement

## Notions clés

Un **processus** est un programme en cours d'exécution avec son contexte.

Liste des états usuels :

1. Prêt
2. En cours
3. Bloqué (attente d'E/S)
4. Terminé

## Ordonnancement

À revoir : la notion de *quantum* et la politique du tourniquet.

\`\`\`c
// exemple simple de boucle d'ordonnancement (pseudocode)
while (que_non_vide()) {
    next = prochain_processus();
    swap_context(&courant, &next);
}
\`\`\`

> Le cahier des charges demande la conservation fidèle des accents, des listes et des blocs de code.

Voir aussi [la page du cours](https://example.org/ens/os).`,
      cours.id,
      ['os', 'systeme'],
      95,
      60 * 24 * 8,
      true
    ),
    makeNote(
      'Bases de données — requêtes SQL utiles',
      `# SQL — le pense-bête

## Jointures

- \`INNER JOIN\` : lignes appariées uniquement
- \`LEFT JOIN\` : toutes les lignes de gauche
- \`CROSS JOIN\` : produit cartésien

## Exemple

\`\`\`sql
SELECT n.title, f.name
  FROM notes n
  LEFT JOIN folders f ON f.id = n.folder_id
 WHERE n.is_pinned = 1
 ORDER BY n.updated_at DESC;
\`\`\`

À noter : \`NOT NULL\` et \`UNIQUE\` font partie des contraintes d'intégrité.`,
      cours.id,
      ['sql', 'bdd'],
      40
    ),
    makeNote(
      'Algorithme — tri rapide',
      `# Quicksort

## Principe

1. Choisir un pivot.
2. Partitionner le tableau.
3. Trier récursivement les deux parties.

\`\`\`python
def tri_rapide(t):
    if len(t) <= 1:
        return t
    pivot = t[0]
    petits = [x for x in t[1:] if x <= pivot]
    grands = [x for x in t[1:] if x > pivot]
    return tri_rapide(petits) + [pivot] + tri_rapide(grands)
\`\`\`

Complexité moyenne ==O(n log n)==, pire cas O(n²).`,
      algo.id,
      ['algorithmique', 'python'],
      60 * 24 * 2
    ),
    makeNote(
      'Projet NoteCore — journal des décisions',
      `# Journal du projet

## Fait

- Cahier des charges relu et validé.
- Choix de stockage : SQLite unique source de vérité (ADR-002).
- Maquette de l'interface en trois zones.

## À faire

- [x] Créer les états de l'interface
- [ ] Brancher les bindings Wails
- [ ] Ajuster la fenêtre d'autosauvegarde

## Question ouverte

Le calendrier avance-t-il assez vite pour préparer l'export ?`,
      projets.id,
      ['gestionprojet'],
      15,
      60 * 24 * 12,
      true
    ),
    makeNote(
      'Réseaux — le modèle en couches',
      `# TCP/IP en résumé

Architecture en couches, à mémoriser par cœur :

- **Application** : HTTP, SMTP, DNS
- **Transport** : TCP ou UDP
- **Réseau** : IP, routage
- **Accès** : Wi-Fi, Ethernet, câble

Attention au vocabulaire accentué : liaison de données, contrôle d'erreur,
adresse physique \`MAC\`.

Voir la [RFC 1122](https://www.rfc-editor.org/rfc/rfc1122).`,
      cours.id,
      ['réseaux'],
      60 * 24 * 5
    ),
    makeNote(
      'JavaScript — pièges de `this`',
      `# \`this\` en JavaScript

Règles de résolution, de la plus courante à la plus rare :

1. Appel de méthode : \`obj.m()\` → \`obj\`
2. Appel nu : \`f()\` → \`undefined\` en module strict
3. \`new\` : instance en construction
4. \`call\`, \`apply\`, \`bind\` : valeur transmise explicitement

\`\`\`js
const compta = {
  total: 0,
  ajoute(m) { this.total += m; }
};
compta.ajoute(5); // this === compta
\`\`\`

> Dans une fonction fléchée, \`this\` est capturé dans le contexte englobant.`,
      projets.id,
      ['javascript'],
      230
    )
  ];

  return { notes, folders: [cours, projets, algo] };
}

const seed = buildSeed();

// --- révision (substitut en mémoire : analyse simplifiée, planification naïve) ---

interface MockCard {
  id: string;
  noteId: string;
  key: string;
  kind: 'cloze' | 'qa';
  front: string;
  back: string;
  state: number;
  reps: number;
  due: number;
  last: number | null;
  log: Array<{ at: number; prev: Pick<MockCard, 'state' | 'reps' | 'due' | 'last'> & { id: string } }>;
}
const mockCards: MockCard[] = [];
const mockLog: MockCard['log'] = [];
const mockReview = { newPerDay: 20, retention: 0.9 };

function deriveCards(noteId: string, content: string): void {
  const found = new Map<string, { kind: 'cloze' | 'qa'; front: string; back: string }>();
  for (const line of content.split('\n')) {
    const re = /\{\{c(\d+)::(.+?)(?:::(.+?))?\}\}/g;
    const ns = new Set([...line.matchAll(re)].map((m) => m[1]));
    for (const n of ns) {
      found.set(`c${n}`, {
        kind: 'cloze',
        front: line.replace(re, (_m, k, a, h) => (k === n ? (h ? `[${h}]` : '[…]') : a)),
        back: line.replace(re, (_m, k, a) => (k === n ? `==${a}==` : a))
      });
    }
    const qa = !ns.size && line.match(/^(?:[-*+]\s+)?(.*?\S) :: (\S.*)$/);
    if (qa) found.set(`q:${qa[1].toLowerCase()}`, { kind: 'qa', front: qa[1], back: qa[2] });
  }
  for (const [key, d] of found) {
    const ex = mockCards.find((c) => c.noteId === noteId && c.key === key);
    if (ex) Object.assign(ex, d);
    else
      mockCards.push({ id: newId('card'), noteId, key, ...d, state: 0, reps: 0, due: Date.now(), last: null, log: [] });
  }
  for (let i = mockCards.length - 1; i >= 0; i--) {
    if (mockCards[i].noteId === noteId && !found.has(mockCards[i].key)) mockCards.splice(i, 1);
  }
}

const humanizeMs = (ms: number): string =>
  ms < 3_600_000 ? `${Math.max(1, Math.round(ms / 60_000))} min` : ms < 86_400_000 ? `${Math.round(ms / 3_600_000)} h` : `${Math.round(ms / 86_400_000)} j`;

function intervalFor(c: MockCard, rating: 1 | 2 | 3 | 4): number {
  if (rating === 1) return 10 * 60_000;
  const days = [0, 0, 1, 3, 7][rating] * Math.pow(2, c.reps);
  return Math.max(rating === 2 ? 20 * 3_600_000 : 86_400_000, days * 86_400_000);
}

function cardView(c: MockCard): ReviewCard {
  const note = seed.notes.find((n) => n.id === c.noteId);
  return {
    id: c.id,
    noteId: c.noteId,
    noteTitle: note?.title ?? '',
    kind: c.kind,
    front: c.front,
    back: c.back,
    state: c.state,
    reps: c.reps,
    intervals: {
      again: humanizeMs(intervalFor(c, 1)),
      hard: humanizeMs(intervalFor(c, 2)),
      good: humanizeMs(intervalFor(c, 3)),
      easy: humanizeMs(intervalFor(c, 4))
    }
  };
}

const eod = () => new Date(new Date().setHours(24, 0, 0, 0)).getTime();

// Quelques cartes d'exemple, écrites comme le ferait un utilisateur.
{
  const extra: Record<string, string> = {
    Réseaux: '\n\n## À retenir\n- Le modèle {{c1::OSI}} compte {{c2::7}} couches.\n- Quel protocole de transport est fiable ? :: TCP\n- {{c3::UDP::protocole}} privilégie la rapidité.',
    exploitation: '\n\n## À retenir\nUn {{c1::processus}} possède son propre {{c2::espace d\'adressage}}.\nQuel appel système crée un processus ? :: fork()',
    SQL: '\n\nUne jointure {{c1::LEFT JOIN}} conserve toutes les lignes de la table de {{c2::gauche}}.'
  };
  for (const n of seed.notes) {
    const hit = Object.keys(extra).find((k) => n.title.includes(k));
    if (hit) n.content += extra[hit];
    deriveCards(n.id, n.content);
  }
  mockCards.forEach((c, i) => {
    if (i % 3 === 0) Object.assign(c, { state: 2, reps: 3, due: Date.now() - 3_600_000, last: Date.now() - 4 * 86_400_000 });
    if (i % 3 === 1) Object.assign(c, { state: 2, reps: 5, due: Date.now() + 5 * 86_400_000, last: Date.now() - 86_400_000 });
  });
}

// Fichiers joints d'exemple (lecture, vue « Fichiers »).
{
  const reseaux = seed.notes.find((n) => n.title.includes('Réseaux'));
  const os = seed.notes.find((n) => n.title.includes('exploitation'));
  const demo: Array<[string | undefined, string, string, string]> = [
    [reseaux?.id, 'cours-reseaux.pdf', 'application/pdf', 'JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAwIFIgPj4KZW5kb2JqCjIgMCBvYmoKPDwgL1R5cGUgL1BhZ2VzIC9LaWRzIFszIDAgUiA1IDAgUl0gL0NvdW50IDIgPj4KZW5kb2JqCjMgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUiAvTWVkaWFCb3ggWzAgMCA0MjAgNTk1XSAvQ29udGVudHMgNCAwIFIgL1Jlc291cmNlcyA8PCAvRm9udCA8PCAvRjEgNyAwIFIgPj4gPj4gPj4KZW5kb2JqCjQgMCBvYmoKPDwgL0xlbmd0aCAxMzUgPj4Kc3RyZWFtCkJUIC9GMSAyNCBUZiA0MCA1MjAgVGQgKE5vdGVDb3JlIC0gZXhlbXBsZSBkZSBQREYpIFRqIEVUIEJUIC9GMSAxMyBUZiA0MCA0ODAgVGQgKEx1IGRhbnMgbGEgdmlzaW9ubmV1c2UsIHNhbnMgbW9kdWxlIGR1IHN5c3RlbWUuKSBUaiBFVAplbmRzdHJlYW0KZW5kb2JqCjUgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUiAvTWVkaWFCb3ggWzAgMCA0MjAgNTk1XSAvQ29udGVudHMgNiAwIFIgL1Jlc291cmNlcyA8PCAvRm9udCA8PCAvRjEgNyAwIFIgPj4gPj4gPj4KZW5kb2JqCjYgMCBvYmoKPDwgL0xlbmd0aCAxMDcgPj4Kc3RyZWFtCkJUIC9GMSAyNCBUZiA0MCA1MjAgVGQgKFBhZ2UgMikgVGogRVQgQlQgL0YxIDEzIFRmIDQwIDQ4MCBUZCAoTGVzIHBhZ2VzIHNvbnQgZGVzc2luZWVzIGEgbGEgZGVtYW5kZS4pIFRqIEVUCmVuZHN0cmVhbQplbmRvYmoKNyAwIG9iago8PCAvVHlwZSAvRm9udCAvU3VidHlwZSAvVHlwZTEgL0Jhc2VGb250IC9IZWx2ZXRpY2EgPj4KZW5kb2JqCnhyZWYKMCA4CjAwMDAwMDAwMDAgNjU1MzUgZiAKMDAwMDAwMDAwOSAwMDAwMCBuIAowMDAwMDAwMDU4IDAwMDAwIG4gCjAwMDAwMDAxMjEgMDAwMDAgbiAKMDAwMDAwMDI0NyAwMDAwMCBuIAowMDAwMDAwNDMzIDAwMDAwIG4gCjAwMDAwMDA1NTkgMDAwMDAgbiAKMDAwMDAwMDcxNyAwMDAwMCBuIAp0cmFpbGVyCjw8IC9TaXplIDggL1Jvb3QgMSAwIFIgPj4Kc3RhcnR4cmVmCjc4NwolJUVPRgo='],
    [reseaux?.id, 'modele-en-couches.svg', 'image/svg+xml', 'PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI0ODAiIGhlaWdodD0iMzAwIj48cmVjdCB3aWR0aD0iNDgwIiBoZWlnaHQ9IjMwMCIgZmlsbD0iI2UwZTdmZiIvPjxjaXJjbGUgY3g9IjE1MCIgY3k9IjE1MCIgcj0iODAiIGZpbGw9IiM2MzY2ZjEiLz48cmVjdCB4PSIyNjAiIHk9IjkwIiB3aWR0aD0iMTUwIiBoZWlnaHQ9IjEyMCIgcng9IjE0IiBmaWxsPSIjYTViNGZjIi8+PHRleHQgeD0iMjQwIiB5PSIyNzAiIHRleHQtYW5jaG9yPSJtaWRkbGUiIGZvbnQtZmFtaWx5PSJzYW5zLXNlcmlmIiBmb250LXNpemU9IjIyIiBmaWxsPSIjMzEyZTgxIj5Nb2RlbGUgZW4gY291Y2hlczwvdGV4dD48L3N2Zz4='],
    [reseaux?.id, 'protocoles.csv', 'text/csv', 'Q291Y2hlO1Byb3RvY29sZTtQb3J0CkFwcGxpY2F0aW9uO0hUVFA7ODAKQXBwbGljYXRpb247SFRUUFM7NDQzClRyYW5zcG9ydDtUQ1A7LQpUcmFuc3BvcnQ7VURQOy0KUmVzZWF1O0lQOy0K'],
    [os?.id, 'journal.txt', 'text/plain', 'IyBKb3VybmFsIGRlIGJvcmQKCjIwMjYtMTAtMTggIG1pZ3JhdGlvbiBkdSBzY2hlbWEgdjMKMjAyNi0xMC0xOSAgdGVzdHMgZHUgbW90ZXVyIEZTUlMKMjAyNi0xMC0yMCAgcGFydGFnZSBob3JzIGNvbm5leGlvbgo='],
    [os?.id, 'tp-processus.zip', 'application/zip', 'UEsFBgAAAAAAAAAAAAAAAAAAAAAAAA==']
  ];
  demo.forEach(([noteId, filename, mime, data], i) => {
    if (!noteId) return;
    attachments.push({
      id: newId('att'), noteId, filename, mime, size: Math.round((data.length * 3) / 4),
      createdAt: minutesAgo(30 + i * 90), dataBase64: data
    });
  });
}

export const mockApi: NoteCoreApi = {
  async listNotes() {
    await delay();
    return seed.notes.map((n) => ({ ...n, tags: [...n.tags] }));
  },

  async listFolders() {
    await delay();
    return seed.folders.map((f) => ({ ...f }));
  },

  async createNote(data) {
    await delay();
    const note = makeNote(
      data.title?.trim() ? data.title : 'Sans titre',
      '',
      data.folderId ?? null,
      [],
      0
    );
    seed.notes.push(note);
    return { ...note };
  },

  async updateNote(id, patch: NotePatch) {
    await delay();
    const idx = seed.notes.findIndex((n) => n.id === id);
    if (idx === -1) throw new Error('note introuvable');
    seed.notes[idx] = { ...seed.notes[idx], ...patch, updatedAt: minutesAgo(0) };
    if (patch.content !== undefined) deriveCards(id, patch.content);
    return { ...seed.notes[idx], tags: [...seed.notes[idx].tags] };
  },

  async deleteNote(id) {
    await delay();
    const idx = seed.notes.findIndex((n) => n.id === id);
    if (idx !== -1) seed.notes.splice(idx, 1);
    // Cascade (ADR-006) : les pièces jointes de la note disparaissent.
    for (let i = attachments.length - 1; i >= 0; i--) {
      if (attachments[i].noteId === id) attachments.splice(i, 1);
    }
  },

  async createFolder(name) {
    await delay();
    const folder = makeFolder(name.trim(), 0);
    seed.folders.push(folder);
    return { ...folder };
  },

  async renameFolder(id, name) {
    await delay();
    const idx = seed.folders.findIndex((f) => f.id === id);
    if (idx === -1) throw new Error('dossier introuvable');
    seed.folders[idx] = { ...seed.folders[idx], name: name.trim(), updatedAt: minutesAgo(0) };
    return { ...seed.folders[idx] };
  },

  async deleteFolder(id) {
    await delay();
    const idx = seed.folders.findIndex((f) => f.id === id);
    if (idx === -1) throw new Error('dossier introuvable');
    seed.folders.splice(idx, 1);
    // Règle du cahier des charges (§3.2) : aucune note n'est perdue,
    // les notes du dossier repassent à la racine.
    const affected = seed.notes.filter((n) => n.folderId === id);
    for (const n of affected) n.folderId = null;
    return affected.map((n) => ({ ...n }));
  },

  async searchNotes(query) {
    await delay();
    const q = query.trim().toLowerCase();
    if (!q) return [];
    return seed.notes
      .filter(
        (n) =>
          n.title.toLowerCase().includes(q) ||
          n.content.toLowerCase().includes(q) ||
          n.tags.some((t) => t.toLowerCase().includes(q))
      )
      .map((n) => ({
        noteId: n.id,
        title: n.title,
        excerpt: n.content.slice(0, 120)
      })) satisfies SearchHit[];
  },

  // Les dialogs d'enregistrement système n'existent pas hors application
  // native : le mock refuse explicitement plutôt que simuler un succès.
  async exportNote() {
    await delay();
    throw new Error("L'export est disponible dans l'application native.");
  },

  async startupReport() {
    // Mock en mémoire : aucun schéma n'est migré, donc aucune copie.
    await delay();
    return '';
  },

  async exportAllNotes() {
    await delay();
    throw new Error("L'export est disponible dans l'application native.");
  },

  async createBackup() {
    await delay();
    throw new Error('La sauvegarde est disponible dans l’application native.');
  },

  async inspectBackup() {
    await delay();
    throw new Error('La restauration est disponible dans l’application native.');
  },

  async restoreBackup() {
    await delay();
    throw new Error('La restauration est disponible dans l’application native.');
  },

  async getSettings() {
    await delay();
    return { ...settings };
  },

  async setSetting(key, value) {
    await delay();
    settings[key] = value;
  },

  // --- pièces jointes (ADR-006) ---

  async addAttachment(noteId, filename, mime, dataBase64) {
    await delay();
    if (!seed.notes.some((n) => n.id === noteId)) {
      throw new Error('note introuvable');
    }
    const att: MockAttachment = {
      id: newId('att'),
      noteId,
      filename: filename.trim() || 'fichier',
      mime: mime || 'application/octet-stream',
      size: Math.floor((dataBase64.length * 3) / 4),
      createdAt: minutesAgo(0),
      dataBase64
    };
    attachments.push(att);
    return {
      id: att.id,
      noteId: att.noteId,
      filename: att.filename,
      mime: att.mime,
      size: att.size,
      createdAt: att.createdAt
    };
  },

  async listAttachments(noteId) {
    await delay();
    return attachments
      .filter((a) => a.noteId === noteId)
      .map((a) => ({
        id: a.id,
        noteId: a.noteId,
        filename: a.filename,
        mime: a.mime,
        size: a.size,
        createdAt: a.createdAt
      }));
  },

  async readAttachment(id) {
    await delay();
    const att = attachments.find((a) => a.id === id);
    if (!att) throw new Error('pièce jointe introuvable');
    return { mime: att.mime, filename: att.filename, dataBase64: att.dataBase64 };
  },

  async deleteAttachment(id) {
    await delay();
    const idx = attachments.findIndex((a) => a.id === id);
    if (idx !== -1) attachments.splice(idx, 1);
  },

  async reviewStats() {
    await delay(40);
    const intro = mockCards.filter((c) => c.state !== 0).length;
    const due = mockCards.filter((c) => c.state !== 0 && c.due <= eod()).length;
    const fresh = Math.min(mockCards.filter((c) => c.state === 0).length, Math.max(0, mockReview.newPerDay - 0 * intro));
    const day = 86_400_000;
    const activity = Array.from({ length: 14 }, (_v, i) => (i === 13 ? mockLog.length : [0, 3, 8, 0, 12, 5, 9, 14, 0, 6, 11, 7, 10][i] ?? 0));
    return {
      due,
      new: fresh,
      total: mockCards.length,
      reviewedToday: mockLog.length,
      nextDue: due === 0 && mockCards.length ? new Date(Date.now() + day).toISOString() : '',
      streak: 4,
      activity
    };
  },

  async reviewNext() {
    await delay(40);
    const ready = mockCards.filter((c) => c.state !== 0 && c.due <= eod()).sort((a, b) => a.due - b.due)[0];
    const next = ready ?? mockCards.find((c) => c.state === 0);
    return next ? cardView(next) : null;
  },

  async reviewAnswer(cardId, rating) {
    await delay(40);
    const c = mockCards.find((x) => x.id === cardId);
    if (!c) throw new Error('carte introuvable');
    mockLog.push({ at: Date.now(), prev: { id: c.id, state: c.state, reps: c.reps, due: c.due, last: c.last } });
    c.due = Date.now() + intervalFor(c, rating);
    c.state = rating === 1 ? 1 : 2;
    c.reps += 1;
    c.last = Date.now();
  },

  async reviewUndo() {
    await delay(40);
    const entry = mockLog.pop();
    if (!entry) return null;
    const c = mockCards.find((x) => x.id === entry.prev.id);
    if (!c) return null;
    Object.assign(c, entry.prev);
    return cardView(c);
  },

  async reviewMastery() {
    await delay(40);
    const rows: MasteryRow[] = [];
    for (const f of seed.folders) {
      const ids = new Set(seed.notes.filter((n) => n.folderId === f.id).map((n) => n.id));
      const cs = mockCards.filter((c) => ids.has(c.noteId));
      if (!cs.length) continue;
      const learned = cs.filter((c) => c.state !== 0);
      rows.push({ kind: 'folder', id: f.id, label: f.name, cards: cs.length, learned: learned.length, due: learned.filter((c) => c.due <= eod()).length, retention: learned.length ? 0.86 : 0 });
    }
    return rows;
  },

  async noteCards(noteId) {
    await delay(20);
    const cs = mockCards.filter((c) => c.noteId === noteId);
    return { cards: cs.length, due: cs.filter((c) => c.state !== 0 && c.due <= eod()).length, new: cs.filter((c) => c.state === 0).length };
  },

  async reviewSettings() {
    return { ...mockReview };
  },

  async setReviewSettings(newPerDay, retention) {
    mockReview.newPerDay = newPerDay;
    mockReview.retention = retention;
  },

  async listAllAttachments() {
    await delay(40);
    return attachments.map((a) => ({
      id: a.id, noteId: a.noteId, filename: a.filename, mime: a.mime, size: a.size, createdAt: a.createdAt,
      noteTitle: seed.notes.find((n) => n.id === a.noteId)?.title ?? '', folderId: seed.notes.find((n) => n.id === a.noteId)?.folderId ?? null
    }));
  },

  async listArchive() {
    return { entries: [{ name: 'cours/', size: 0, isDir: true }, { name: 'cours/notes.txt', size: 1204, isDir: false }], truncated: false };
  },

  async openAttachment() {
    throw new Error("L'ouverture avec le système est disponible dans l'application native.");
  },

  async shareToFile() {
    throw new Error("L'enregistrement est disponible dans l'application native.");
  },

  async shareStart() {
    await delay(200);
    return { urls: ['http://192.168.1.24:41823/s/Zk3p0wX-demo'], filename: 'Cours.notecore', size: 20480, expiresAt: new Date(Date.now() + 600_000).toISOString(), encrypted: false, notes: 3, attachments: 1 };
  },

  async shareStatus() {
    return { state: 'waiting' as const };
  },

  async shareStop() {},

  async importFromFile() {
    throw new Error("L'import est disponible dans l'application native.");
  },

  async importFromUrl() {
    throw new Error("L'import est disponible dans l'application native.");
  },

  async importPending() {
    throw new Error("L'import est disponible dans l'application native.");
  },

  async cancelPending() {},

  async appInfo() {
    return { os: 'linux', dataDir: '~/.config/NoteCore', databasePath: '~/.config/NoteCore/notecore.db' };
  },

  async openExternal(url: string) {
    window.open(url, '_blank', 'noopener,noreferrer');
  },

  // L'enregistrement passe par un dialog système : indisponible hors
  // application native (comme l'export).
  async saveAttachment() {
    await delay();
    throw new Error("L'enregistrement est disponible dans l'application native.");
  }
};
