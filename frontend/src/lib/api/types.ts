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
