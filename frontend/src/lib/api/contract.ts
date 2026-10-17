import type { Attachment, AttachmentData, Folder, Note, NotePatch, SearchHit } from './types';

/**
 * Contrat unique entre l'interface et le backend (ARCHITECTURE.md §2).
 * Le frontend n'appelle jamais SQL ni le système de fichiers directement :
 * il passe uniquement par cette interface, implémentée
 * - en phase 1 par un mock en mémoire ;
 * - à partir du backend Go, par les bindings Wails.
 */
export interface NoteCoreApi {
  listNotes(): Promise<Note[]>;
  listFolders(): Promise<Folder[]>;

  createNote(data: { title?: string; folderId?: string | null }): Promise<Note>;
  updateNote(id: string, patch: NotePatch): Promise<Note>;
  deleteNote(id: string): Promise<void>;

  createFolder(name: string): Promise<Folder>;
  renameFolder(id: string, name: string): Promise<Folder>;
  /**
   * Supprime un dossier. Conformément au cdc.md §3.2, la suppression
   * ne doit jamais entraîner la perte silencieuse des notes : elles
   * repassent à la racine (ON DELETE SET NULL côté SQLite).
   * Renvoie les notes déplacées.
   */
  deleteFolder(id: string): Promise<Note[]>;

  /** Recherche full-text côté moteur (titre, contenu, étiquettes). */
  searchNotes(query: string): Promise<SearchHit[]>;

  // --- export / sauvegarde (cdc.md §3.7) ---

  /**
   * Exporte une note en Markdown autonome. Le chemin de destination est
   * choisi par le dialog système ; renvoie false si annulé.
   */
  exportNote(id: string): Promise<boolean>;
  /**
   * Archive toutes les notes (Markdown + manifeste) dans un .zip.
   * Renvoie false si le dialog est annulé.
   */
  exportAllNotes(): Promise<boolean>;
  /**
   * Produit une sauvegarde vérifiable de la base complète.
   * Renvoie false si le dialog est annulé.
   */
  createBackup(): Promise<boolean>;
  restoreBackup(): Promise<boolean>;

  // --- préférences (cdc.md §3.6) ---

  /**
   * Tous les réglages locaux, lus dans la table `settings` de SQLite.
   * Le frontend n'utilise plus le stockage du navigateur comme source
   * de vérité : les préférences survivent au redémarrage.
   */
  getSettings(): Promise<Record<string, string>>;
  /** Enregistre (ou remplace) un réglage local. */
  setSetting(key: string, value: string): Promise<void>;

  // --- pièces jointes (ADR-006) ---

  /**
   * Ajoute une pièce jointe à une note. Les octets sont encodés en
   * base64 (le pont Wails ne transporte pas de binaire). Le backend
   * borne la taille et assainit le nom.
   */
  addAttachment(
    noteId: string,
    filename: string,
    mime: string,
    dataBase64: string
  ): Promise<Attachment>;
  /** Métadonnées des pièces jointes d'une note (sans les octets). */
  listAttachments(noteId: string): Promise<Attachment[]>;
  /** Octets d'une pièce jointe (aperçu d'image). */
  readAttachment(id: string): Promise<AttachmentData>;
  /** Supprime une pièce jointe. */
  deleteAttachment(id: string): Promise<void>;
  /** Enregistre une pièce jointe sur le disque (fichiers non-images). */
  saveAttachment(id: string): Promise<boolean>;
}
