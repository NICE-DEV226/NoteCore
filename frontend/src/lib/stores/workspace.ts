/**
 * Store principal de l'espace de travail : notes, dossiers, vues,
 * recherche, état de sauvegarde et autosauvegarde débordée.
 * Toutes les écritures passent par la couche `api/` (contrat unique),
 * jamais directement depuis les composants (ARCHITECTURE.md §2).
 */

import { derived, get, writable } from 'svelte/store';
import { api } from '../api/client';
import { refreshReview, scheduleReviewRefresh } from './review';
import { filesOpen } from './files';
import type {
  Folder,
  Note,
  NotePatch,
  RestorePreview,
  SaveState,
  SearchHit,
  View
} from '../api/types';

/** Délai sans saisie avant écriture (cahier des charges §3.5 : courte période). */
const AUTOSAVE_DELAY_MS = 800;

// ---------------------------------------------------------------------------
// État brut
// ---------------------------------------------------------------------------

export const notes = writable<Note[]>([]);
export const folders = writable<Folder[]>([]);
export const loaded = writable<boolean>(false);

export const selectedNoteId = writable<string | null>(null);
export const currentView = writable<View>({ kind: 'all' });
export const searchQuery = writable<string>('');
export const saveState = writable<SaveState>('saved');

// ---------------------------------------------------------------------------
// État dérivé
// ---------------------------------------------------------------------------

export const selectedNote = derived([notes, selectedNoteId], ([$notes, $id]) =>
  $id ? ($notes.find((n) => n.id === $id) ?? null) : null
);

/** Étiquettes en usage, triées, pour la zone latérale. */
export const allTags = derived(notes, ($notes) => {
  const set = new Set<string>();
  for (const n of $notes) for (const t of n.tags ?? []) set.add(t);
  return [...set].sort((a, b) => a.localeCompare(b, 'fr'));
});

/**
 * Résultats du moteur de recherche du backend (SQLite FTS5 — ADR-003).
 * La recherche n'est pas locale au frontend : le Set contient les ids
 * de notes qui matchent, ou null quand la recherche est vide.
 */
/** Extraits de recherche (ADR-003) indexés par identifiant de note.
 * `null` tant qu'aucune réponse n'est arrivée : on ne présente alors ni
 * faux « aucun résultat », ni extraits périmés. */
export const remoteSearchHits = writable<Map<string, SearchHit> | null>(null);

let searchSeq = 0;
let searchTimer: ReturnType<typeof setTimeout> | null = null;
let workspaceListenersReady = false;
/** Dernier texte de recherche dont les extraits ont été écartés. */
let lastSearchedQuery: string | null = null;

function handleBeforeUnload(e: BeforeUnloadEvent): void {
  if (!pendingPatch) return;
  void performSave();
  // L'écriture passe par le pont Wails et ne peut pas être attendue :
  // on demande confirmation plutôt que de perdre la saisie en silence.
  e.preventDefault();
  e.returnValue = 'Des modifications ne sont pas encore enregistrées.';
}

/** Filet de sécurité : exécuté même si `beforeunload` est ignoré. */
function handlePageHide(): void {
  if (pendingPatch) void performSave();
}

/** Fenêtre masquée (fenêtre réduite, session verrouillée) : on écrit tout
 *  de suite plutôt que d'attendre le délai d'autosauvegarde. */
function handleVisibilityChange(): void {
  if (document.visibilityState === 'hidden' && pendingPatch) void performSave();
}

/** Relance de la recherche backend, défendue contre les réponses en retard. */
export function scheduleSearch(delayMs = 200): void {
  // Dès que le texte change, les extraits connus ne décrivent plus la
  // requête courante : on les écarte tout de suite plutôt que de laisser
  // la liste filtrée par les résultats précédents (§3.3).
  const current = get(searchQuery).trim();
  if (current !== lastSearchedQuery) {
    lastSearchedQuery = current;
    remoteSearchHits.set(null);
  }

  if (searchTimer !== null) clearTimeout(searchTimer);
  searchTimer = setTimeout(async () => {
    const q = get(searchQuery).trim();
    if (!q) {
      remoteSearchHits.set(null);
      return;
    }
    const seq = ++searchSeq;
    try {
      const hits: SearchHit[] = await api.searchNotes(q);
      if (seq !== searchSeq) return; // réponse obsolète : on ignore
      remoteSearchHits.set(new Map(hits.map((h) => [h.noteId, h])));
    } catch {
      if (seq === searchSeq) remoteSearchHits.set(null);
    }
  }, delayMs);
}

/** Liste des notes de la vue courante, filtrée par la recherche backend. */
export const filteredNotes = derived(
  [notes, currentView, searchQuery, remoteSearchHits],
  ([$notes, $view, $query, $hits]) => {
    let list = $notes;

    switch ($view.kind) {
      case 'all':
        list = list.filter(() => true);
        break;
      case 'favorites':
        list = list.filter((n) => n.isPinned);
        break;
      case 'recent':
        list = [...list]
          .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
          .slice(0, 10);
        break;
      case 'folder':
        list = list.filter((n) => n.folderId === $view.folderId);
        break;
      case 'tag':
        list = list.filter((n) => (n.tags ?? []).includes($view.tag));
        break;
    }

    // La recherche s'appuie sur les extraits du moteur du backend
    // (SQLite FTS5, §3.3). Les extraits sont invalidés dès que la requête
    // change : la liste n'est jamais filtrée par une requête précédente.
    // En leur absence, un filtre local provisoire prend le relais, et
    // `searchPending` évite d'annoncer un « aucun résultat » prématuré.
    const q = $query.trim();
    if (q) {
      if ($hits) {
        list = list.filter((n) => $hits.has(n.id));
      } else {
        // Réponse du moteur encore attendue : filtre local provisoire,
        // remplacé dès l'arrivée des extraits de FTS5. `searchPending`
        // empêche la liste d'annoncer « aucun résultat » trop tôt.
        list = list.filter(
          (n) =>
            n.title.toLowerCase().includes(q.toLowerCase()) ||
            n.content.toLowerCase().includes(q.toLowerCase()) ||
            (n.tags ?? []).some((t) => t.toLowerCase().includes(q.toLowerCase()))
        );
      }
    }

    // Favoris d'abord, puis modification la plus récente.
    return [...list].sort((a, b) => {
      if (a.isPinned !== b.isPinned) return a.isPinned ? -1 : 1;
      return b.updatedAt.localeCompare(a.updatedAt);
    });
  }
);

/** Y a-t-il au moins une note dans l'application ? (état vide du CDC §4.3) */
export const hasAnyNote = derived(notes, ($notes) => $notes.length > 0);

/**
 * Une recherche est en cours d'exécution côté backend. Pendant ce délai
 * la liste ne doit annoncer ni « aucun résultat », ni des extraits
 * d'une requête précédente : elle attend (§3.3).
 */
export const searchPending = derived(
  [searchQuery, remoteSearchHits],
  ([$searchQuery, $hits]) => $searchQuery.trim().length > 0 && $hits === null
);

/** Nombre de favoris, pour le compteur de la barre latérale. */
export const favoritesCount = derived(notes, ($notes) => $notes.filter((n) => n.isPinned).length);

// ---------------------------------------------------------------------------
// Autosauvegarde
// ---------------------------------------------------------------------------

let pendingPatch: { id: string; patch: NotePatch } | null = null;
let saveTimer: ReturnType<typeof setTimeout> | null = null;

function clearSaveTimer(): void {
  if (saveTimer !== null) {
    clearTimeout(saveTimer);
    saveTimer = null;
  }
}

function scheduleSave(id: string, patch: NotePatch): void {
  pendingPatch = { id, patch };
  saveState.set('dirty');
  clearSaveTimer();
  saveTimer = setTimeout(() => void performSave(), AUTOSAVE_DELAY_MS);
}

/**
 * Les écritures sont jouées l'une après l'autre. Chaque appel du pont Wails
 * s'exécute dans sa propre goroutine : deux `updateNote` simultanés pourraient
 * être validés dans le désordre, et l'ancien état écraserait le plus récent.
 * Chaque écriture relit `pendingPatch` à son tour, donc la dernière saisie
 * l'emporte toujours.
 */
let saveChain: Promise<void> = Promise.resolve();

function performSave(): Promise<void> {
  const run = saveChain.then(writePending);
  saveChain = run.catch(() => undefined);
  return run;
}

async function writePending(): Promise<void> {
  const current = pendingPatch;
  if (!current) return;
  const { id, patch } = current;
  clearSaveTimer();
  saveState.set('saving');
  try {
    const updated = await api.updateNote(id, patch);
    // Une saisie plus récente peut avoir remplacé ce patch pendant
    // l'attente : ne retirer de la file que celui qui vient d'être écrit.
    if (pendingPatch === current) pendingPatch = null;
    notes.update((list) =>
      list.map((n) => {
        if (n.id !== id) return n;
        // Une saisie plus récente est en attente : ne pas écraser
        // les champs modifiés localement depuis cette écriture.
        if (pendingPatch?.id === id) {
          return { ...n, updatedAt: updated.updatedAt, tags: [...(n.tags ?? [])] };
        }
        return { ...updated, tags: [...(updated.tags ?? [])] };
      })
    );
    if (!pendingPatch) saveState.set('saved');
    // Le contenu vient de changer : l'index de recherche backend est
    // lui aussi potentiellement périmé — on relance si une requête
    // est active (CDC §3.3 : résultats toujours justes).
    if (get(searchQuery).trim()) scheduleSearch(50);
    // Les cartes de révision suivent le texte de la note (ADR-007).
    scheduleReviewRefresh();
  } catch {
    saveState.set('error');
    // Le patch est conservé pour que « Réessayer » rejoue réellement
    // l'écriture (CDC §3.5). S'il a été remplacé par une saisie plus
    // récente, cette dernière porte déjà l'état complet : ne pas écraser.
    if (!pendingPatch) pendingPatch = current;
  }
}

/** Force l'écriture immédiate des modifications en attente. */
export async function flushSave(): Promise<void> {
  if (!pendingPatch) return;
  clearSaveTimer();
  await performSave();
}

// ---------------------------------------------------------------------------
// Actions utilisateur
// ---------------------------------------------------------------------------

export async function initWorkspace(): Promise<void> {
  const [loadedNotes, loadedFolders, safetyCopy] = await Promise.all([
    api.listNotes(),
    api.listFolders(),
    // Une migration de schéma a pu conserver la base précédente : elle
    // est annoncée, jamais laissée sur le disque sans le dire (§11).
    api.startupReport()
  ]);
  notes.set(loadedNotes ?? []);
  folders.set(loadedFolders ?? []);
  loaded.set(true);
  void refreshReview();

  if (safetyCopy) {
    notifyTransfer(
      'info',
      `Mise à jour appliquée. Vos données précédentes sont conservées dans ${safetyCopy}.`
    );
  }

  if (workspaceListenersReady) return;
  workspaceListenersReady = true;

  // Ces abonnements sont installés une seule fois, même après une nouvelle tentative.
  searchQuery.subscribe(() => scheduleSearch());
  window.addEventListener('beforeunload', handleBeforeUnload);
  window.addEventListener('pagehide', handlePageHide);
  document.addEventListener('visibilitychange', handleVisibilityChange);
}

/** Relit notes et dossiers (après un import, par exemple), sans toucher à la sélection. */
export async function reloadData(): Promise<void> {
  const [loadedNotes, loadedFolders] = await Promise.all([api.listNotes(), api.listFolders()]);
  notes.set(loadedNotes ?? []);
  folders.set(loadedFolders ?? []);
  void refreshReview();
}

/** Retourne à l'accueil : aucune note ouverte, modifications d'abord écrites. */
export async function goHome(): Promise<void> {
  await flushSave();
  filesOpen.set(false);
  selectedNoteId.set(null);
}

/** Ouvre la vue « Fichiers » (toutes les pièces jointes) à la place de l'éditeur. */
export async function goFiles(): Promise<void> {
  await flushSave();
  selectedNoteId.set(null);
  filesOpen.set(true);
}

/** Sélectionne une note en forçant d'abord l'écriture des modifications en attente. */
export async function selectNote(id: string): Promise<void> {
  if (id === get(selectedNoteId)) return;
  await flushSave();
  filesOpen.set(false);
  selectedNoteId.set(id);
}

export async function createNote(folderId: string | null = null): Promise<void> {
  await flushSave();
  try {
    const note = await api.createNote({ folderId });
    notes.update((list) => [...list, note]);
    currentView.set({ kind: 'all' });
    searchQuery.set('');
    filesOpen.set(false);
    selectedNoteId.set(note.id);
    saveState.set('saved');
  } catch {
    saveState.set('error');
  }
}

/**
 * Séquence la mise à jour de la note sélectionnée : mémoires locales
 * immédiates, écriture débordée ensuite (cahier des charges §3.5).
 */
export function updateSelected(patch: NotePatch): void {
  const id = get(selectedNoteId);
  if (!id) return;
  let updatedNote: Note | null = null;
  notes.update((list) =>
    list.map((n) => {
      if (n.id !== id) return n;
      updatedNote = { ...n, ...patch };
      return updatedNote;
    })
  );
  const note = updatedNote as Note | null;
  if (!note) return;
  scheduleSave(id, {
    title: note.title,
    content: note.content,
    tags: note.tags ?? [],
    folderId: note.folderId,
    isPinned: note.isPinned
  });
}

export async function togglePin(id: string): Promise<void> {
  await flushSave();
  let next: Note | null = null;
  notes.update((list) =>
    list.map((n) => {
      if (n.id !== id) return n;
      next = { ...n, isPinned: !n.isPinned };
      return next;
    })
  );
  const note = next as Note | null;
  if (note) scheduleSave(id, { isPinned: note.isPinned });
}

export async function deleteNote(id: string): Promise<void> {
  await flushSave();
  try {
    await api.deleteNote(id);
    notes.update((list) => list.filter((n) => n.id !== id));
    if (get(selectedNoteId) === id) selectedNoteId.set(null);
    saveState.set('saved');
  } catch {
    saveState.set('error');
  }
}

/**
 * Dernière erreur d'une opération sur un dossier (création, renommage,
 * suppression). Affichée près de la liste des dossiers : elle reste
 * distincte de l'état d'enregistrement de la note ouverte, qui porte sur
 * un autre objet.
 */
export const folderError = writable<string | null>(null);

function folderErrorText(err: unknown): string {
  return err instanceof Error ? err.message : "L'opération a échoué.";
}

export async function createFolder(name: string): Promise<boolean> {
  const trimmed = name.trim();
  if (!trimmed) return false;
  folderError.set(null);
  try {
    const folder = await api.createFolder(trimmed);
    folders.update((list) => [...list, folder]);
    return true;
  } catch (err) {
    // Une opération sur un dossier n'est pas un enregistrement de note :
    // l'erreur s'affiche près de la liste des dossiers, pas dans la barre
    // d'enregistrement de l'éditeur.
    folderError.set(folderErrorText(err));
    return false;
  }
}

export async function renameFolder(id: string, name: string): Promise<boolean> {
  const trimmed = name.trim();
  if (!trimmed) return false;
  folderError.set(null);
  try {
    const folder = await api.renameFolder(id, trimmed);
    folders.update((list) => list.map((f) => (f.id === id ? folder : f)));
    return true;
  } catch (err) {
    folderError.set(folderErrorText(err));
    return false;
  }
}

/** Supprime un dossier sans toucher aux notes (cahier des charges §3.2). */
export async function deleteFolder(id: string): Promise<boolean> {
  folderError.set(null);
  try {
    const moved = await api.deleteFolder(id);
    folders.update((list) => list.filter((f) => f.id !== id));
    const movedIds = new Set(moved.map((n) => n.id));
    notes.update((list) =>
      list.map((n) => (movedIds.has(n.id) ? { ...n, folderId: null } : n))
    );
    // Si la vue courante est le dossier supprimé, repasser sur « toutes ».
    const view = get(currentView);
    if (view.kind === 'folder' && view.folderId === id) currentView.set({ kind: 'all' });
    return true;
  } catch (err) {
    folderError.set(folderErrorText(err));
    return false;
  }
}

/** Rejoue l'écriture en attente après un échec (CDC §3.5). */
export async function retrySave(): Promise<void> {
  await performSave();
}

export function clearSearch(): void {
  searchQuery.set('');
  remoteSearchHits.set(null);
}

// ---------------------------------------------------------------------------
// Export / sauvegarde (cahier des charges §3.7)
// ---------------------------------------------------------------------------

/** Message de résultat de transfert, affiché discrètement puis effacé. */
export const transferNotice = writable<{ kind: 'info' | 'error'; text: string } | null>(null);

let transferNoticeTimer: ReturnType<typeof setTimeout> | null = null;

function notifyTransfer(kind: 'info' | 'error', text: string): void {
  transferNotice.set({ kind, text });
  if (transferNoticeTimer !== null) clearTimeout(transferNoticeTimer);
  transferNoticeTimer = setTimeout(() => {
    transferNotice.set(null);
    transferNoticeTimer = null;
  }, 6000);
}

function transferErrorText(err: unknown): string {
  return err instanceof Error ? err.message : "l'opération a échoué";
}

/** Exporte la note sélectionnée en Markdown autonome. */
export async function exportSelectedNote(): Promise<void> {
  const id = get(selectedNoteId);
  if (!id) return;
  await flushSave();
  try {
    const done = await api.exportNote(id);
    if (done) notifyTransfer('info', 'Note exportée en Markdown.');
  } catch (err) {
    notifyTransfer('error', `Échec de l'export : ${transferErrorText(err)}`);
  }
}

/** Archive toutes les notes avec leur manifeste (§3.7). */
export async function exportAllNotes(): Promise<void> {
  try {
    const done = await api.exportAllNotes();
    if (done) notifyTransfer('info', 'Archive d’export créée.');
  } catch (err) {
    notifyTransfer('error', `Échec de l'export : ${transferErrorText(err)}`);
  }
}

/** Sauvegarde complète vérifiable de la base (VACUUM INTO, ADR-004). */
export async function createBackup(): Promise<void> {
  try {
    const done = await api.createBackup();
    if (done) notifyTransfer('info', 'Sauvegarde créée.');
  } catch (err) {
    notifyTransfer('error', `Échec de la sauvegarde : ${transferErrorText(err)}`);
  }
}

/** Sauvegarde validée, en attente de confirmation avant écrasement. */
export const restorePreview = writable<RestorePreview | null>(null);

/**
 * Choix de la sauvegarde à restaurer : ouvre le sélecteur système et
 * valide l'archive SANS rien écrire. La confirmation porte ensuite sur un
 * fichier identifié, daté et compté — jamais sur une action aveugle
 * (ADR-004). Une archive corrompue ou trop récente est refusée ici.
 */
export async function chooseBackupToRestore(): Promise<void> {
  try {
    const preview = await api.inspectBackup();
    if (!preview.path) return; // sélecteur annulé
    restorePreview.set(preview);
  } catch (err) {
    notifyTransfer('error', `Sauvegarde refusée : ${transferErrorText(err)}`);
  }
}

/** Restaure la sauvegarde confirmée puis recharge les données visibles. */
export async function restoreBackup(): Promise<void> {
  const preview = get(restorePreview);
  restorePreview.set(null);
  if (!preview) return;
  try {
    // Aucune écriture différée ne doit subsister : elle pourrait se
    // déclencher pendant la restauration et écrire dans la base restaurée.
    await flushSave();
    const safetyPath = await api.restoreBackup(preview.path);
    await initWorkspace();
    selectedNoteId.set(null);
    currentView.set({ kind: 'all' });
    searchQuery.set('');
    remoteSearchHits.set(null);
    // La copie de sécurité est annoncée : aucun fichier n'est écrit en
    // silence sur le disque de l'utilisateur.
    notifyTransfer(
      'info',
      safetyPath
        ? `Sauvegarde restaurée. Vos données précédentes sont conservées dans ${safetyPath}.`
        : 'Sauvegarde restaurée.'
    );
  } catch (err) {
    notifyTransfer('error', `Échec de la restauration : ${transferErrorText(err)}`);
  }
}
