/**
 * Préférence de thème, conservée après redémarrage (cahier des charges §3.6).
 *
 * La source de vérité est la table `settings` de SQLite, atteinte via le
 * backend (`getSettings` / `setSetting`, ARCHITECTURE.md §2). Le stockage
 * du navigateur ne sert plus que d'amorce : il évite un premier rendu
 * dans le mauvais thème le temps de l'aller-retour vers le backend. Si
 * les deux divergent, la valeur de la table `settings` gagne.
 */

export type ThemePref = 'light' | 'dark' | 'system';

/** Clé du réglage dans la table `settings` (backend). */
export const THEME_SETTING_KEY = 'theme';
/** Clé d'amorce dans le stockage du navigateur (cache local uniquement). */
const CACHE_KEY = 'notecore.theme';

function normalize(value: unknown): ThemePref {
  return value === 'light' || value === 'dark' || value === 'system' ? value : 'system';
}

/** Convertit une valeur brute en préférence, ou `undefined` si absente. */
export function toThemePref(value: unknown): ThemePref | undefined {
  if (value === 'light' || value === 'dark' || value === 'system') return value;
  return undefined;
}

/** Amorce synchrone depuis le cache local (thème correct avant le backend). */
export function readThemePref(): ThemePref {
  if (typeof localStorage === 'undefined') return 'system';
  return normalize(localStorage.getItem(CACHE_KEY));
}

/** Met à jour l'amorce locale : confort de démarrage, pas la vérité. */
export function writeThemePref(pref: ThemePref): void {
  try {
    localStorage.setItem(CACHE_KEY, pref);
  } catch {
    /* stockage indisponible : la préférence reste valable pour la session */
  }
}

/** Résout la préférence en thème concret appliqué au document. */
export function resolveTheme(pref: ThemePref, prefersDark = false): 'dark' | 'light' {
  if (pref === 'system') return prefersDark ? 'dark' : 'light';
  return pref;
}

export function systemPrefersDark(): boolean {
  return typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches;
}

/** Applique le thème sur <html data-theme="...">. */
export function applyTheme(state: { pref: ThemePref; theme: 'light' | 'dark' }): void {
  if (typeof document === 'undefined') return;
  document.documentElement.dataset.theme = state.theme;
}

/** Recalcule le thème système quand l'OS bascule (préférence « system »). */
export function watchSystemTheme(recompute: () => void): void {
  if (typeof matchMedia === 'undefined') return;
  const mediaQuery = matchMedia('(prefers-color-scheme: dark)');
  if (typeof mediaQuery.addEventListener === 'function') {
    mediaQuery.addEventListener('change', recompute);
  } else if (typeof mediaQuery.addListener === 'function') {
    // Compatibilité avec les versions WebKitGTK utilisées par certains runtimes Wails.
    mediaQuery.addListener(recompute);
  }
}
