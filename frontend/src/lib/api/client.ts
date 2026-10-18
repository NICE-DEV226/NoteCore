import type { NoteCoreApi } from './contract';
import { mockApi } from './mock';
import type { Attachment, AttachmentData, Folder, Note, NotePatch, RestorePreview, SearchHit } from './types';

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
  interface Window { go?: Record<string, any> }
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
    return ((await this.svc.ListFolders()) as Folder[] | null) ?? [];
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

  async createFolder(name: string): Promise<Folder> {
    return (await this.svc.CreateFolder(name)) as Folder;
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
}

function resolveApi(): NoteCoreApi {
  const transport = discoverWailsTransport();
  if (transport) return new WailsAdapter(transport);
  return mockApi;
}

export const api: NoteCoreApi = resolveApi();
