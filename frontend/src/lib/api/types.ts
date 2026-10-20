/**
 * Types de transport entre le frontend et le backend.
 * Ils reflètent le modèle de données du cahier des charges (§7)
 * et la source de vérité SQLite (ADR-002).
 */

export interface Note {
  id: string;
  title: string;
  content: string;
  /** Dossier de rattachement, facultatif (null = racine). */
  folderId: string | null;
  /**
   * État favori / épinglé. Le CDC emploie les deux termes pour la même
   * notion (cahier des charges §3.1 « un état favori ou épinglé ») : un seul champ
   * `is_pinned` représente cette notion, affichée comme « favoris ».
   */
  isPinned: boolean;
  /** Titres d'étiquettes dénormalisés pour l'affichage. */
  tags: string[];
  /** Dates au format UTC normalisé (ARCHITECTURE.md §4). */
  createdAt: string;
  updatedAt: string;
}

/** Résultat de recherche renvoyé par le backend (ADR-003 : extraits
 * déjà structurés côté Go, requête côte front bornée). */
export interface SearchHit {
  noteId: string;
  title: string;
  /** Extrait pertinent construit par le backend (snippet FTS, ADR-003). */
  excerpt: string;
  updatedAt?: string;
}

export interface Folder {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
}

/**
 * Pièce jointe locale d'une note (ADR-006). Les octets vivent dans
 * SQLite ; l'interface ne manipule que les métadonnées et une référence
 * `attachment:<id>` dans le Markdown.
 */
export interface Attachment {
  id: string;
  noteId: string;
  filename: string;
  mime: string;
  size: number;
  createdAt: string;
}

/** Octets d'une pièce jointe, transportés en base64 par le pont Wails. */
export interface AttachmentData {
  mime: string;
  filename: string;
  dataBase64: string;
}

/**
 * Description d'une sauvegarde choisie par l'utilisateur, établie AVANT
 * toute écriture (cahier des charges §3.7) : la confirmation de
 * restauration porte sur un fichier identifié et daté.
 * `path` vide signifie que le sélecteur a été annulé.
 */
export interface RestorePreview {
  path: string;
  /** Nom du fichier choisi, pour l'identification. */
  name: string;
  /** Date de création de la sauvegarde (RFC 3339), si connue. */
  createdAt: string;
  schemaVersion: number;
  /** Nombre de notes de la sauvegarde (0 si l'information est absente). */
  notes: number;
}

/** Champs modifiables d'une note. */
export type NotePatch = Partial<
  Pick<Note, 'title' | 'content' | 'folderId' | 'isPinned' | 'tags'>
>;

/** États de sauvegarde exposés à l'interface (ARCHITECTURE.md §5). */
export type SaveState = 'saved' | 'dirty' | 'saving' | 'error';

/** Vues de la barre latérale et de la liste des notes. */
export type View =
  | { kind: 'all' }
  | { kind: 'favorites' }
  | { kind: 'recent' }
  | { kind: 'folder'; folderId: string }
  | { kind: 'tag'; tag: string };

// --- révision espacée (ADR-007) ---------------------------------------------

/** Délais proposés pour chaque réponse, déjà formulés par le backend. */
export interface ReviewIntervals {
  again: string;
  hard: string;
  good: string;
  easy: string;
}

/** Carte à réviser : recto et verso sont du Markdown dérivé de la note. */
export interface ReviewCard {
  id: string;
  noteId: string;
  noteTitle: string;
  kind: 'cloze' | 'qa';
  front: string;
  back: string;
  /** 0 nouvelle, 1 apprentissage, 2 révision, 3 ré-apprentissage. */
  state: number;
  reps: number;
  intervals: ReviewIntervals;
}

export interface ReviewStats {
  due: number;
  new: number;
  total: number;
  reviewedToday: number;
  /** Prochaine échéance (RFC 3339) quand rien n'est dû maintenant. */
  nextDue: string;
  streak: number;
  /** Réponses par jour sur 14 jours, du plus ancien au plus récent. */
  activity: number[];
}

export interface MasteryRow {
  kind: 'folder' | 'tag';
  id: string;
  label: string;
  cards: number;
  learned: number;
  due: number;
  /** Probabilité moyenne de se souvenir, de 0 à 1. */
  retention: number;
}

export interface NoteCards {
  cards: number;
  due: number;
  new: number;
}

export interface ReviewSettings {
  newPerDay: number;
  retention: number;
}

/** Note de la réponse donnée à une carte. */
export type ReviewRating = 1 | 2 | 3 | 4;

/** Où l'application range les données de l'utilisateur (guide de prise en main). */
export interface AppInfo {
  os: 'linux' | 'windows' | 'darwin' | string;
  dataDir: string;
  databasePath: string;
}

// --- fichiers joints et partage hors connexion (ADR-006, ADR-008) -----------

/** Pièce jointe avec la note qui la porte (vue « Fichiers »). */
export interface AttachmentRef extends Attachment {
  noteTitle: string;
  folderId?: string | null;
}

export interface ArchiveEntry {
  name: string;
  size: number;
  isDir: boolean;
}

export interface ArchiveListing {
  entries: ArchiveEntry[];
  truncated: boolean;
}

export type ShareScope = 'note' | 'folder' | 'all';
export type ShareFormat = 'notecore' | 'markdown';

export interface ShareOptions {
  scope: ShareScope;
  id: string;
  format: ShareFormat;
  passphrase: string;
}

export interface ShareSaved {
  saved: boolean;
  path: string;
  notes: number;
  attachments: number;
  encrypted: boolean;
}

export interface LanShare {
  urls: string[];
  filename: string;
  size: number;
  expiresAt: string;
  encrypted: boolean;
  notes: number;
  attachments: number;
}

export interface ShareStatus {
  state: 'idle' | 'waiting' | 'downloaded' | 'expired' | 'stopped';
  urls?: string[];
  filename?: string;
  size?: number;
  expiresAt?: string;
}

export interface ImportReport {
  imported: number;
  skipped: number;
  renamed: number;
  foldersCreated: number;
  attachments: number;
  attachmentsSkipped: number;
  titles: string[];
}

export interface ImportResult {
  cancelled: boolean;
  needsPassphrase: boolean;
  wrongPassphrase: boolean;
  pendingId: string;
  report: ImportReport;
}
