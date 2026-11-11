/**
 * Vérification et installation de mise à jour : un appel réseau à la
 * sortie du dernier release GitHub, déclenché une fois par lancement
 * (et à la demande dans Paramètres). Hors ligne ou API muette → rien ne
 * s'affiche : l'app reste 100 % fonctionnelle sans Internet.
 *
 * « Mettre à jour » enchaîne téléchargement → vérification SHA256 →
 * installation adaptée au mode d'installation → redémarrage quand
 * c'est possible. La progression du téléchargement arrive par événement
 * Wails ; en mode mock/navigateur, elle reste à zéro.
 */
import { get } from 'svelte/store';
import { writable } from 'svelte/store';
import { api } from '../api/client';
import type { UpdateInfo } from '../api/types';

export const updateInfo = writable<UpdateInfo | null>(null);
export const updateDismissed = writable(false);
/** 0-100 pendant le téléchargement, null hors installation. */
export const updateProgress = writable<number | null>(null);
export const updateApplying = writable(false);

let checking = false;

export async function checkForUpdate(): Promise<void> {
  if (checking) return;
  checking = true;
  try {
    const info = await api.checkForUpdate();
    updateInfo.set(info.available ? info : null);
  } catch {
    // Pas de réseau, délai dépassé, API indisponible : silence.
  } finally {
    checking = false;
  }
}

/**
 * Lance l'installation complète. Retourne le message à afficher, ou
 * null si l'opération est déjà en cours. Quand le backend doit
 * redémarrer l'application, il le fait de lui-même : le message
 * retourné n'est jamais vu.
 */
export async function applyUpdate(): Promise<string | null> {
  if (get(updateApplying)) return null;
  updateApplying.set(true);
  updateProgress.set(0);
  const stop = api.onUpdateProgress((pct) => updateProgress.set(pct));
  try {
    const res = await api.applyUpdate();
    return res.message || 'Mise à jour installée.';
  } catch (err) {
    return err instanceof Error ? err.message : 'Mise à jour impossible.';
  } finally {
    stop();
    updateApplying.set(false);
    updateProgress.set(null);
  }
}
