/**
 * Préférences d'affichage de l'éditeur (cdc.md §3.6 « Apparence ») :
 * police, taille du texte et largeur de colonne.
 *
 * Ce sont des réglages LOCAUX, conservés dans la table `settings` comme
 * le thème. Ils n'altèrent jamais la note elle-même : le contenu reste
 * du Markdown portable (§3.4, §6.5). Ils s'appliquent par variables CSS
 * sur `<html>`, lues par l'éditeur et l'aperçu.
 */

import { get, writable } from 'svelte/store';
import { api } from '../api/client';

export type FontChoice = 'serif' | 'sans' | 'mono';
export type SizeChoice = 'compact' | 'normal' | 'large';
export type WidthChoice = 'narrow' | 'normal' | 'wide';

export interface DisplayPrefs {
  font: FontChoice;
  size: SizeChoice;
  width: WidthChoice;
}

/** Clés de la table `settings`. */
export const FONT_SETTING_KEY = 'editor.font';
export const SIZE_SETTING_KEY = 'editor.size';
export const WIDTH_SETTING_KEY = 'editor.width';

const DEFAULTS: DisplayPrefs = { font: 'serif', size: 'normal', width: 'normal' };

const FONT_VAR: Record<FontChoice, string> = {
  serif: 'var(--font-serif)',
  sans: 'var(--font-ui)',
  mono: 'var(--font-mono)'
};
const SIZE_REM: Record<SizeChoice, string> = {
  compact: '0.94rem',
  normal: '1rem',
  large: '1.12rem'
};
const WIDTH_REM: Record<WidthChoice, string> = {
  narrow: '38rem',
  normal: '44rem',
  wide: '54rem'
};

const FONTS = ['serif', 'sans', 'mono'] as const;
const SIZES = ['compact', 'normal', 'large'] as const;
const WIDTHS = ['narrow', 'normal', 'wide'] as const;

function pick<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  return typeof value === 'string' && (allowed as readonly string[]).includes(value)
    ? (value as T)
    : fallback;
}

export const display = writable<DisplayPrefs>({ ...DEFAULTS });

function apply(prefs: DisplayPrefs): void {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  root.style.setProperty('--editor-font', FONT_VAR[prefs.font]);
  root.style.setProperty('--editor-font-size', SIZE_REM[prefs.size]);
  root.style.setProperty('--editor-width', WIDTH_REM[prefs.width]);
}

display.subscribe(apply);

/** Change une préférence : applique tout de suite, persiste ensuite. */
export function setDisplayPref<K extends keyof DisplayPrefs>(
  key: K,
  value: DisplayPrefs[K]
): void {
  display.update((prefs) => ({ ...prefs, [key]: value }));
  const settingKey =
    key === 'font' ? FONT_SETTING_KEY : key === 'size' ? SIZE_SETTING_KEY : WIDTH_SETTING_KEY;
  void api.setSetting(settingKey, value).catch((err) => {
    console.warn("Enregistrement de la préférence d'affichage impossible :", err);
  });
}

/** Applique les valeurs relues de la table `settings` (défauts sinon). */
export function applyStoredDisplay(settings: Record<string, string>): void {
  const current = get(display);
  display.set({
    font: pick(settings[FONT_SETTING_KEY], FONTS, current.font),
    size: pick(settings[SIZE_SETTING_KEY], SIZES, current.size),
    width: pick(settings[WIDTH_SETTING_KEY], WIDTHS, current.width)
  });
}

/** Hydrate les préférences d'affichage depuis la base au démarrage. */
export async function loadDisplayPrefs(): Promise<void> {
  try {
    applyStoredDisplay(await api.getSettings());
  } catch (err) {
    console.warn("Chargement des préférences d'affichage impossible :", err);
  }
}
