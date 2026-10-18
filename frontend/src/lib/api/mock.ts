import type { NoteCoreApi } from './contract';
import type { Folder, Note, NotePatch, SearchHit } from './types';

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

  // L'enregistrement passe par un dialog système : indisponible hors
  // application native (comme l'export).
  async saveAttachment() {
    await delay();
    throw new Error("L'enregistrement est disponible dans l'application native.");
  }
};
