/**
 * État purement visuel de la fenêtre : panneaux ouverts, mode de l'éditeur.
 * Rien ici n'est une donnée de l'utilisateur ; le mode d'édition est mémorisé
 * localement (navigateur), comme l'amorce du thème, pour ne pas solliciter la
 * base à chaque changement de présentation.
 */

import { writable } from 'svelte/store';

export type EditorMode = 'edit' | 'split' | 'preview';

const MODE_KEY = 'notecore.editorMode';

function readMode(): EditorMode {
  try {
    const stored = localStorage.getItem(MODE_KEY);
    if (stored === 'edit' || stored === 'split' || stored === 'preview') return stored;
  } catch {
    /* stockage indisponible : valeur par défaut */
  }
  return 'edit';
}

export const editorMode = writable<EditorMode>(readMode());

editorMode.subscribe((mode) => {
  try {
    localStorage.setItem(MODE_KEY, mode);
  } catch {
    /* sans conséquence */
  }
});

/** Palette de commandes (Ctrl/Cmd+K). */
export const paletteOpen = writable(false);

/** Fenêtre des paramètres (Ctrl/Cmd+,). */
export const settingsOpen = writable(false);

/** Mode concentration : seule la zone d'écriture reste visible. */
export const focusMode = writable(false);

/** Boîte de partage : ce qu'elle propose de partager (note ouverte, dossier, tout). */
export const shareDialog = writable<{ scope: 'note' | 'folder' | 'all'; id: string } | null>(null);

/** Boîte d'import : depuis un fichier ou un autre appareil du réseau local. */
export const importOpen = writable(false);

/** Ce qui est en train d'être glissé (notes ou dossier), lisible pendant le survol. */
export const dragging = writable<{ kind: 'notes' | 'folder'; ids: string[] } | null>(null);

/** Sélecteur de dossier (déplacer des notes ou un dossier). */
export const folderPicker = writable<{
  title: string;
  /** Dossiers interdits comme destination (un dossier et son sous-arbre). */
  exclude: string[];
  current: string | null;
  rootLabel: string;
  onpick: (folderId: string | null) => void;
} | null>(null);

/** Boîte « Nouvelle matière » (modèles de dossiers) ; parentId = dossier d'accueil proposé. */
export const templateDialog = writable<{ parentId: string | null } | null>(null);
