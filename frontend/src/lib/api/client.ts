import type { NoteCoreApi } from './contract';
import { mockApi } from './mock';
import type {
  AppInfo,
  FolderSpec,
  TreeResult,
  FolderSuggestion,
  InboxItem,
  ArchiveListing,
  AttachmentRef,
  ImportResult,
  LanShare,
  ShareOptions,
  ShareSaved,
  ShareStatus,
  Attachment,
  AttachmentData,
  Folder,
  MasteryRow,
  Note,
  NoteCards,
  NotePatch,
  RestorePreview,
  ReviewCard,
  ReviewRating,
  ReviewSettings,
  ReviewStats,
  SearchHit
} from './types';

/**
 * Point d'accès unique de l'interface au backend (ARCHITECTURE.md §2).
 *
 * Deux modes :
 * - backend Wails branché (production / binaire Go) : l'adaptateur
 *   appelle les méthodes Go exposées sur window.go — découverte du
 *   namespace à l'exécution pour ne pas dépendre du détail des
 *   bindings v2 ;
 * - sinon : le mock en mémoire, tant que le backend ne tourne pas.
 * Seul ce fichier connaît le transport.
 */

declare global {
  // injecté par le runtime Wails quand le backend Go est branché
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  interface Window {
    go?: Record<string, any>;
    runtime?: { BrowserOpenURL(url: string): void };
  }
}

type Transport = Record<string, (...args: unknown[]) => Promise<unknown>>;

/** Retrouve l'objet bindings Wails par ses méthodes caractéristiques. */
function discoverWailsTransport(): Transport | null {
  if (typeof window === 'undefined' || !window.go) return null;
  for (const pkg of Object.values(window.go)) {
    if (!pkg || typeof pkg !== 'object') continue;
    for (const svc of Object.values(pkg)) {
      if (!svc || typeof svc !== 'object') continue;
      const candidate = svc as Transport;
      if (
        typeof candidate.ListNotes === 'function' &&
        typeof candidate.UpdateNote === 'function' &&
        typeof candidate.CreateFolder === 'function'
      ) {
        return candidate;
      }
    }
  }
  return null;
}
// wails warrper function for api contrat by svelte
class WailsAdapter implements NoteCoreApi {
  private readonly svc: Transport;

  constructor(svc: Transport) {
    this.svc = svc;
  }

  private toNote(p: unknown): Note {
    const n = p as Partial<Note>;
    // Go omet folderId quand il est nul (omitempty) et peut renvoyer null
    // pour un slice vide : normaliser pour que le type Note soit vrai et
    // qu'aucun consommateur n'itère sur null.
    return {
      ...n,
      folderId: n.folderId ?? null,
      tags: n.tags ?? [],
      createdAt: n.createdAt ?? '',
      updatedAt: n.updatedAt ?? ''
    } as Note;
  }

  async listNotes(): Promise<Note[]> {
    const raw = (await this.svc.ListNotes()) as unknown[] | null;
    // Les notes passent par toNote : normalise folderId/tags (Go peut
    // renvoyer null pour un slice vide selon le chemin de construction).
    return (raw ?? []).map((p) => this.toNote(p));
  }

  async listFolders(): Promise<Folder[]> {
    const raw = ((await this.svc.ListFolders()) as Folder[] | null) ?? [];
    return raw.map((f) => ({ ...f, parentId: f.parentId ?? null }));
  }

  async createNote(data: { title?: string; folderId?: string | null }): Promise<Note> {
    // '' = non précisé (titre par défaut, racine) côté Go.
    const note = await this.svc.CreateNote(data.title ?? '', data.folderId ?? '');
    return this.toNote(note);
  }

  async updateNote(id: string, patch: NotePatch): Promise<Note> {
    // Les champs absents sont omis : le patch Go reste partiel. Mais
    // folderId = null signifie « remettre à la racine » et doit devenir
    // '' (la convention Go), sinon le champ est omis et le déplacement
    // n'a aucun effet.
    const wire: Record<string, unknown> = {};
    if (patch.title !== undefined) wire.title = patch.title;
    if (patch.content !== undefined) wire.content = patch.content;
    if (patch.tags !== undefined) wire.tags = patch.tags;
    if (patch.isPinned !== undefined) wire.isPinned = patch.isPinned;
    if (patch.folderId !== undefined) {
      wire.folderId = patch.folderId === null ? '' : patch.folderId;
    }
    const note = await this.svc.UpdateNote(id, wire);
    return this.toNote(note);
  }

  async deleteNote(id: string): Promise<void> {
    await this.svc.DeleteNote(id);
  }
// FUNCTION asyncrhone for folder creation
  async createFolder(name: string, parentId?: string | null): Promise<Folder> {
    return (await this.svc.CreateFolder(name, parentId ?? '')) as Folder;
  }

  async createFolderTree(parentId: string | null, tree: FolderSpec[]): Promise<TreeResult> {
    const r = (await this.svc.CreateFolderTree(parentId ?? '', tree)) as Partial<TreeResult> | null;
    return {
      created: (r?.created ?? []).map((f) => ({ ...f, parentId: f.parentId ?? null })),
      existing: r?.existing ?? 0,
      rootIds: r?.rootIds ?? []
    };
  }

  async moveFolder(id: string, parentId: string | null): Promise<void> {
    await this.svc.MoveFolder(id, parentId ?? '');
  }

  async moveNotes(noteIds: string[], folderId: string | null): Promise<void> {
    await this.svc.MoveNotes(noteIds, folderId ?? '');
  }

  async suggestFolders(noteId: string): Promise<FolderSuggestion[]> {
    return ((await this.svc.SuggestFolders(noteId)) as FolderSuggestion[] | null) ?? [];
  }

  async inbox(): Promise<InboxItem[]> {
    const raw = ((await this.svc.Inbox()) as InboxItem[] | null) ?? [];
    return raw.map((i) => ({ ...i, suggestions: i.suggestions ?? [] }));
  }

  async renameFolder(id: string, name: string): Promise<Folder> {
    await this.svc.RenameFolder(id, name);
    // L'interface ne lit jamais les dates du renommage : renvoyer
    // l'objet suffit ; le backend est l'autorité sur les timestamps.
    return { id, name, createdAt: '', updatedAt: '' };
  }

  async deleteFolder(id: string): Promise<Note[]> {
    const raw = (await this.svc.DeleteFolder(id)) as unknown[] | null;
    // Les notes rapatriées à la racine sont normalisées comme listNotes.
    return (raw ?? []).map((p) => this.toNote(p));
  }

  async searchNotes(query: string): Promise<SearchHit[]> {
    return (await this.svc.SearchNotes(query)) as SearchHit[];
  }

  async startupReport(): Promise<string> {
    // Copie éventuelle écrite par le backend avant migration du schéma ;
    // jamais de chemin silencieux : l'interface l'annonce.
    return ((await this.svc.StartupReport()) as string | null) ?? '';
  }

  async exportNote(id: string): Promise<boolean> {
    return (await this.svc.ExportNote(id)) as boolean;
  }

  async exportAllNotes(): Promise<boolean> {
    return (await this.svc.ExportAllNotes()) as boolean;
  }

  async createBackup(): Promise<boolean> {
    return (await this.svc.CreateBackup()) as boolean;
  }

  async inspectBackup(): Promise<RestorePreview> {
    return ((await this.svc.InspectBackup()) as RestorePreview | null) ?? {
      path: '',
      name: '',
      createdAt: '',
      schemaVersion: 0,
      notes: 0
    };
  }

  async restoreBackup(path: string): Promise<string> {
    // Go renvoie le chemin de la copie de sécurité (chaîne vide si la
    // plateforme n'a pas pu en produire) : normalisé pour l'appelant.
    return ((await this.svc.RestoreBackup(path)) as string | null) ?? '';
  }

  async getSettings(): Promise<Record<string, string>> {
    // Go renvoie un objet JSON (map[string]string) ; on protège contre
    // une réponse nulle pour que les consommateurs itèrent sans garde.
    return ((await this.svc.GetSettings()) as Record<string, string> | null) ?? {};
  }

  async setSetting(key: string, value: string): Promise<void> {
    await this.svc.SetSetting(key, value);
  }

  // --- pièces jointes (ADR-006) ---

  async addAttachment(
    noteId: string,
    filename: string,
    mime: string,
    dataBase64: string
  ): Promise<Attachment> {
    return (await this.svc.AddAttachment(noteId, filename, mime, dataBase64)) as Attachment;
  }

  async listAttachments(noteId: string): Promise<Attachment[]> {
    return ((await this.svc.ListAttachments(noteId)) as Attachment[] | null) ?? [];
  }

  async readAttachment(id: string): Promise<AttachmentData> {
    return (await this.svc.ReadAttachment(id)) as AttachmentData;
  }

  async deleteAttachment(id: string): Promise<void> {
    await this.svc.DeleteAttachment(id);
  }

  async saveAttachment(id: string): Promise<boolean> {
    return (await this.svc.SaveAttachment(id)) as boolean;
  }

  async listAllAttachments(): Promise<AttachmentRef[]> {
    return ((await this.svc.ListAllAttachments()) as AttachmentRef[] | null) ?? [];
  }

  async listArchive(id: string): Promise<ArchiveListing> {
    const r = (await this.svc.ListArchive(id)) as Partial<ArchiveListing> | null;
    return { entries: r?.entries ?? [], truncated: r?.truncated ?? false };
  }

  async openAttachment(id: string): Promise<void> {
    await this.svc.OpenAttachment(id);
  }

  async shareToFile(options: ShareOptions): Promise<ShareSaved> {
    return (await this.svc.ShareToFile(options)) as ShareSaved;
  }

  async shareStart(options: ShareOptions): Promise<LanShare> {
    return (await this.svc.ShareStart(options)) as LanShare;
  }

  async shareStatus(): Promise<ShareStatus> {
    return ((await this.svc.ShareStatus()) as ShareStatus | null) ?? { state: 'idle' };
  }

  async shareStop(): Promise<void> {
    await this.svc.ShareStop();
  }

  private toImport(r: unknown): ImportResult {
    const x = r as Partial<ImportResult> | null;
    const rep = x?.report;
    return {
      cancelled: x?.cancelled ?? false,
      needsPassphrase: x?.needsPassphrase ?? false,
      wrongPassphrase: x?.wrongPassphrase ?? false,
      pendingId: x?.pendingId ?? '',
      report: {
        imported: rep?.imported ?? 0,
        skipped: rep?.skipped ?? 0,
        renamed: rep?.renamed ?? 0,
        foldersCreated: rep?.foldersCreated ?? 0,
        attachments: rep?.attachments ?? 0,
        attachmentsSkipped: rep?.attachmentsSkipped ?? 0,
        titles: rep?.titles ?? []
      }
    };
  }

  async importFromFile(): Promise<ImportResult> {
    return this.toImport(await this.svc.ImportFromFile());
  }

  async importFromUrl(url: string): Promise<ImportResult> {
    return this.toImport(await this.svc.ImportFromURL(url));
  }

  async importPending(id: string, passphrase: string): Promise<ImportResult> {
    return this.toImport(await this.svc.ImportPending(id, passphrase));
  }

  async cancelPending(): Promise<void> {
    await this.svc.CancelPending();
  }

  async appInfo(): Promise<AppInfo> {
    return ((await this.svc.AppInfo()) as AppInfo | null) ?? { os: '', dataDir: '', databasePath: '' };
  }

  async openExternal(url: string): Promise<void> {
    // Fourni par le runtime Wails (hors des bindings Go). Sans lui, un lien
    // remplacerait l'interface dans la fenêtre de l'application.
    window.runtime?.BrowserOpenURL(url);
  }

  // --- révision espacée (ADR-007) ---

  async reviewStats(): Promise<ReviewStats> {
    const s = (await this.svc.ReviewStats()) as Partial<ReviewStats> | null;
    return {
      due: s?.due ?? 0,
      new: s?.new ?? 0,
      total: s?.total ?? 0,
      reviewedToday: s?.reviewedToday ?? 0,
      nextDue: s?.nextDue ?? '',
      streak: s?.streak ?? 0,
      activity: s?.activity ?? []
    };
  }

  async reviewNext(): Promise<ReviewCard | null> {
    return ((await this.svc.ReviewNext()) as ReviewCard | null) ?? null;
  }

  async reviewAnswer(cardId: string, rating: ReviewRating): Promise<void> {
    await this.svc.ReviewAnswer(cardId, rating);
  }

  async reviewUndo(): Promise<ReviewCard | null> {
    return ((await this.svc.ReviewUndo()) as ReviewCard | null) ?? null;
  }

  async reviewMastery(): Promise<MasteryRow[]> {
    return ((await this.svc.ReviewMastery()) as MasteryRow[] | null) ?? [];
  }

  async noteCards(noteId: string): Promise<NoteCards> {
    return ((await this.svc.NoteCards(noteId)) as NoteCards | null) ?? { cards: 0, due: 0, new: 0 };
  }

  async reviewSettings(): Promise<ReviewSettings> {
    return ((await this.svc.ReviewSettings()) as ReviewSettings | null) ?? { newPerDay: 20, retention: 0.9 };
  }

  async setReviewSettings(newPerDay: number, retention: number): Promise<void> {
    await this.svc.SetReviewSettings(newPerDay, retention);
  }
}

/**
 * En production, un pont Wails introuvable est une panne, pas un mode de
 * repli : afficher des données factices ferait croire à l'utilisateur que
 * ses notes sont vides. Chaque appel échoue alors avec un message lisible
 * (écran de démarrage + « Réessayer »).
 */
function brokenApi(): NoteCoreApi {
  const fail = (): never => {
    throw new Error("Le lien avec l'application est introuvable. Relancez NoteCore.");
  };
  return new Proxy({} as NoteCoreApi, {
    get: () => async () => fail()
  });
}

function resolveApi(): NoteCoreApi {
  const transport = discoverWailsTransport();
  if (transport) return new WailsAdapter(transport);
  // Le mock ne sert qu'au développement dans un navigateur (`npm run dev`).
  return import.meta.env.DEV ? mockApi : brokenApi();
}

export const api: NoteCoreApi = resolveApi();
