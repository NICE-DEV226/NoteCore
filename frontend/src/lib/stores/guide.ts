/**
 * Guide de prise en main : affiché une fois au premier lancement, puis
 * rouvrable à tout moment (paramètres, palette de commandes, accueil).
 * Seul le fait de l'avoir vu est mémorisé, dans la table `settings`.
 */

import { writable } from 'svelte/store';
import { api } from '../api/client';

const SEEN_KEY = 'app.guide';

export const guideOpen = writable(false);

/** Ouvre le guide s'il n'a jamais été vu. Sans effet si les réglages sont illisibles. */
export async function initGuide(): Promise<void> {
  try {
    const settings = await api.getSettings();
    if (settings[SEEN_KEY] !== '1') guideOpen.set(true);
  } catch {
    /* pas de guide plutôt qu'un guide qui revient à chaque lancement */
  }
}

export function openGuide(): void {
  guideOpen.set(true);
}

export function closeGuide(): void {
  guideOpen.set(false);
  void api.setSetting(SEEN_KEY, '1').catch((err) => {
    console.warn('Enregistrement du guide impossible :', err);
  });
}
