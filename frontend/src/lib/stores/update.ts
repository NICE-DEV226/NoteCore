/**
 * Vérification de mise à jour : un appel réseau à la sortie du dernier
 * release GitHub, déclenché une fois par lancement (et à la demande dans
 * Paramètres). Hors ligne ou API muette → rien ne s'affiche : l'app
 * reste 100 % fonctionnelle sans Internet, la vérification est un
 * confort, jamais une dépendance.
 */
import { writable } from 'svelte/store';
import { api } from '../api/client';
import type { UpdateInfo } from '../api/types';

export const updateInfo = writable<UpdateInfo | null>(null);
export const updateDismissed = writable(false);

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
