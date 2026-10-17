/**
 * Store applicable seulement : encapsule la préférence de thème
 * (light / dark / system), la persiste dans la table `settings` via le
 * backend (cdc.md §3.6) et réagit à un changement de thème système.
 */

import { get, writable } from 'svelte/store';
import { api } from '../api/client';
import {
  applyTheme,
  readThemePref,
  resolveTheme,
  systemPrefersDark,
  toThemePref,
  watchSystemTheme,
  writeThemePref,
  THEME_SETTING_KEY,
  type ThemePref
} from './theme';

// Amorce synchrone (cache local) : le thème est correct dès le premier
// rendu, avant même la réponse du backend.
const currentPref = readThemePref();
const currentTheme = resolveTheme(currentPref, systemPrefersDark());

export const theme = writable<{ pref: ThemePref; theme: 'light' | 'dark' }>({
  pref: currentPref,
  theme: currentTheme
});

theme.subscribe(applyTheme);

export function setThemePref(pref: ThemePref): void {
  theme.set({ pref, theme: resolveTheme(pref, systemPrefersDark()) });
  // Amorce locale immédiate, puis persistance de référence côté backend.
  writeThemePref(pref);
  void api.setSetting(THEME_SETTING_KEY, pref).catch((err) => {
    console.warn('Enregistrement de la préférence de thème impossible :', err);
  });
}

/**
 * Hydrate les préférences depuis la table `settings` au démarrage. En
 * cas d'échec (backend indisponible), on conserve l'amorce locale : le
 * thème reste cohérent pour la session.
 */
export async function loadPreferences(): Promise<void> {
  try {
    const settings = await api.getSettings();
    const stored = toThemePref(settings[THEME_SETTING_KEY]);
    if (stored) {
      theme.set({ pref: stored, theme: resolveTheme(stored, systemPrefersDark()) });
      return;
    }
    // Première exécution après la bascule vers la table `settings` : on y
    // recopie la préférence amorcée, pour que la base devienne la source
    // de vérité sans perdre le choix existant (migration douce).
    await api.setSetting(THEME_SETTING_KEY, get(theme).pref);
  } catch (err) {
    console.warn('Chargement des préférences impossible :', err);
  }
}

/** Suit l'OS lorsque la préférence est « système ». */
export function watchSystemThemeChanges(): void {
  watchSystemTheme(() => {
    theme.update((state) => ({
      ...state,
      theme: resolveTheme(state.pref, systemPrefersDark())
    }));
  });
}
