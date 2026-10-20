/**
 * Fichiers joints : vue « Fichiers » (parcourir) et visionneuse (lire).
 * Les octets viennent de la base à la demande ; rien n'est écrit sur le disque
 * tant que l'utilisateur n'enregistre pas ou n'ouvre pas avec le système.
 */

import { get, writable } from 'svelte/store';
import { api } from '../api/client';
import type { AttachmentRef } from '../api/types';

/** La vue « Fichiers » occupe la zone principale quand aucune note n'est ouverte. */
export const filesOpen = writable(false);

export const attachmentIndex = writable<AttachmentRef[]>([]);
export const attachmentIndexError = writable<string | null>(null);

export async function refreshAttachmentIndex(): Promise<void> {
  try {
    attachmentIndex.set(await api.listAllAttachments());
    attachmentIndexError.set(null);
  } catch (err) {
    attachmentIndexError.set(err instanceof Error ? err.message : 'Les fichiers sont illisibles.');
  }
}

export interface ViewerState {
  current: AttachmentRef;
  /** Pièces jointes entre lesquelles on navigue (celles de la même note, ou la liste filtrée). */
  siblings: AttachmentRef[];
}

export const viewer = writable<ViewerState | null>(null);

/** Ouvre la visionneuse sur une pièce jointe connue par son seul identifiant. */
export async function openAttachmentViewer(id: string, within?: AttachmentRef[]): Promise<void> {
  let all = get(attachmentIndex);
  let found = all.find((a) => a.id === id);
  if (!found) {
    await refreshAttachmentIndex();
    all = get(attachmentIndex);
    found = all.find((a) => a.id === id);
  }
  if (!found) return;
  const siblings = within && within.some((a) => a.id === id) ? within : all.filter((a) => a.noteId === found!.noteId);
  viewer.set({ current: found, siblings });
}

export function closeViewer(): void {
  viewer.set(null);
}

export function stepViewer(delta: number): void {
  const v = get(viewer);
  if (!v) return;
  const i = v.siblings.findIndex((a) => a.id === v.current.id);
  const next = v.siblings[i + delta];
  if (next) viewer.set({ ...v, current: next });
}

// --- classement par type --------------------------------------------------

export type FileKind = 'image' | 'pdf' | 'markdown' | 'text' | 'table' | 'archive' | 'audio' | 'video' | 'other';

const TEXT_EXT = new Set([
  'txt', 'log', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'conf', 'env', 'sql', 'html', 'htm', 'css',
  'js', 'mjs', 'ts', 'tsx', 'jsx', 'svelte', 'vue', 'py', 'go', 'rs', 'c', 'h', 'cpp', 'hpp', 'java', 'kt', 'cs',
  'php', 'rb', 'sh', 'bash', 'zsh', 'ps1', 'bat', 'tex', 'bib', 'rst', 'adoc', 'diff', 'patch', 'gitignore'
]);

export function extensionOf(name: string): string {
  const i = name.lastIndexOf('.');
  return i < 0 ? '' : name.slice(i + 1).toLowerCase();
}

export function kindOf(a: { filename: string; mime: string }): FileKind {
  const ext = extensionOf(a.filename);
  const mime = a.mime.toLowerCase();
  if (mime.startsWith('image/')) return 'image';
  if (mime === 'application/pdf' || ext === 'pdf') return 'pdf';
  if (ext === 'md' || ext === 'markdown' || mime === 'text/markdown') return 'markdown';
  if (ext === 'csv' || ext === 'tsv' || mime === 'text/csv') return 'table';
  if (ext === 'zip' || mime === 'application/zip' || mime === 'application/x-zip-compressed') return 'archive';
  if (mime.startsWith('audio/')) return 'audio';
  if (mime.startsWith('video/')) return 'video';
  if (mime.startsWith('text/') || mime === 'application/json' || mime === 'application/xml' || TEXT_EXT.has(ext)) return 'text';
  return 'other';
}

export const KIND_LABELS: Record<FileKind, string> = {
  image: 'Image',
  pdf: 'PDF',
  markdown: 'Markdown',
  text: 'Texte et code',
  table: 'Tableau',
  archive: 'Archive',
  audio: 'Audio',
  video: 'Vidéo',
  other: 'Autre'
};

export function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} Mio`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} Kio`;
  return `${bytes} o`;
}
