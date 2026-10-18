/**
 * Préférences d'affichage (cahier des charges §3.6 « Apparence ») : polices,
 * taille, interligne, largeur, accent, densité… Ce sont des réglages LOCAUX,
 * conservés dans la table `settings` comme le thème. Ils n'altèrent jamais la
 * note elle-même : le contenu reste du Markdown portable (§3.4, §6.5). Ils
 * s'appliquent par variables CSS sur `<html>`, lues par toute l'interface.
 */

import { get, writable } from 'svelte/store';
import { api } from '../api/client';
import { fontCss, isFontId } from '../fonts';

export type DensityChoice = 'comfortable' | 'compact';
export type RadiusChoice = 'sharp' | 'soft' | 'round';
export type AlignChoice = 'left' | 'justify';

export interface DisplayPrefs {
  /** Police de l'interface (menus, listes, boutons). */
  uiFont: string;
  /** Police du texte des notes (saisie et aperçu). */
  editorFont: string;
  /** Police des titres (« ui » = celle de l'interface). */
  headingFont: string;
  /** Police du code (blocs et code en ligne). */
  codeFont: string;
  /** Taille du texte de la note, en pixels. */
  fontSize: number;
  /** Interligne du texte (multiplicateur). */
  lineHeight: number;
  /** Graisse du texte courant. */
  weight: number;
  /** Espacement des lettres, en centièmes d'em. */
  letterSpacing: number;
  /** Espace entre paragraphes de l'aperçu, en dixièmes d'em. */
  paragraphSpacing: number;
  /** Largeur de la colonne d'écriture, en rem. */
  width: number;
  /** Taille du titre de la note, en dixièmes de rem. */
  titleSize: number;
  align: AlignChoice;
  /** Teinte de l'accent (0–360, espace OKLCH). */
  accentHue: number;
  density: DensityChoice;
  radius: RadiusChoice;
  spellcheck: boolean;
}

export const DEFAULTS: DisplayPrefs = {
  uiFont: 'inter',
  editorFont: 'source-serif',
  headingFont: 'ui',
  codeFont: 'jetbrains-mono',
  fontSize: 17,
  lineHeight: 1.75,
  weight: 400,
  letterSpacing: 0,
  paragraphSpacing: 5.5,
  width: 46,
  titleSize: 20,
  align: 'left',
  accentHue: 275,
  density: 'comfortable',
  radius: 'soft',
  spellcheck: true
};

/** Bornes de chaque réglage numérique : l'interface et la lecture s'y tiennent. */
export const LIMITS = {
  fontSize: { min: 12, max: 28, step: 1 },
  lineHeight: { min: 1.3, max: 2.3, step: 0.05 },
  weight: { min: 300, max: 600, step: 50 },
  letterSpacing: { min: -2, max: 8, step: 1 },
  paragraphSpacing: { min: 2, max: 20, step: 1 },
  width: { min: 30, max: 80, step: 2 },
  titleSize: { min: 14, max: 40, step: 1 },
  accentHue: { min: 0, max: 360, step: 1 }
} as const;

/** Réglages d'ensemble en un clic. */
export const PRESETS: Array<{ id: string; label: string; hint: string; prefs: Partial<DisplayPrefs> }> = [
  { id: 'paper', label: 'Papier', hint: 'Serif, aéré', prefs: DEFAULTS },
  {
    id: 'book',
    label: 'Livre',
    hint: 'Lecture longue',
    prefs: { editorFont: 'lora', headingFont: 'playfair', fontSize: 18, lineHeight: 1.9, width: 40, titleSize: 24, align: 'left', weight: 400 }
  },
  {
    id: 'code',
    label: 'Développeur',
    hint: 'Tout en mono',
    prefs: { editorFont: 'jetbrains-mono', headingFont: 'ui', codeFont: 'jetbrains-mono', fontSize: 15, lineHeight: 1.65, width: 60, titleSize: 22, letterSpacing: 0 }
  },
  {
    id: 'easy',
    label: 'Lecture facilitée',
    hint: 'Lexend, espacé',
    prefs: { uiFont: 'lexend', editorFont: 'lexend', headingFont: 'lexend', fontSize: 19, lineHeight: 2, letterSpacing: 2, paragraphSpacing: 9, width: 42, weight: 400 }
  },
  {
    id: 'notebook',
    label: 'Cahier',
    hint: 'Manuscrit',
    prefs: { editorFont: 'caveat', headingFont: 'caveat', fontSize: 22, lineHeight: 1.6, width: 44, titleSize: 30, weight: 500 }
  },
  {
    id: 'dense',
    label: 'Dense',
    hint: 'Beaucoup de texte',
    prefs: { editorFont: 'inter', headingFont: 'ui', fontSize: 14, lineHeight: 1.5, width: 72, titleSize: 18, density: 'compact', paragraphSpacing: 4 }
  }
];

/** Teintes proposées pour l'accent (en plus du curseur libre). */
export const ACCENT_SWATCHES: Array<{ label: string; hue: number }> = [
  { label: 'Indigo', hue: 275 },
  { label: 'Bleu', hue: 245 },
  { label: 'Turquoise', hue: 200 },
  { label: 'Vert', hue: 155 },
  { label: 'Citron', hue: 105 },
  { label: 'Ambre', hue: 65 },
  { label: 'Corail', hue: 30 },
  { label: 'Rose', hue: 350 },
  { label: 'Violet', hue: 315 }
];

/** Clé unique de la table `settings` : un objet JSON (≤ 512 octets). */
export const DISPLAY_SETTING_KEY = 'display.prefs';
/** Anciennes clés (v0.1.x), relues une fois pour ne rien perdre. */
const LEGACY = { font: 'editor.font', size: 'editor.size', width: 'editor.width' };

export const display = writable<DisplayPrefs>({ ...DEFAULTS });

const clamp = (v: unknown, min: number, max: number, fallback: number): number => {
  const n = typeof v === 'number' ? v : Number(v);
  return Number.isFinite(n) ? Math.min(max, Math.max(min, n)) : fallback;
};

/** Valide un objet quelconque : toute valeur hors bornes retombe sur le défaut. */
export function sanitize(raw: unknown, base: DisplayPrefs = DEFAULTS): DisplayPrefs {
  const o = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>;
  const num = (k: keyof typeof LIMITS) =>
    clamp(o[k], LIMITS[k].min, LIMITS[k].max, base[k] as number);
  const font = (k: 'uiFont' | 'editorFont' | 'codeFont') => (isFontId(o[k]) ? (o[k] as string) : base[k]);
  return {
    uiFont: font('uiFont'),
    editorFont: font('editorFont'),
    headingFont: o.headingFont === 'ui' || isFontId(o.headingFont) ? (o.headingFont as string) : base.headingFont,
    codeFont: font('codeFont'),
    fontSize: num('fontSize'),
    lineHeight: num('lineHeight'),
    weight: num('weight'),
    letterSpacing: num('letterSpacing'),
    paragraphSpacing: num('paragraphSpacing'),
    width: num('width'),
    titleSize: num('titleSize'),
    align: o.align === 'justify' || o.align === 'left' ? o.align : base.align,
    accentHue: num('accentHue'),
    density: o.density === 'compact' || o.density === 'comfortable' ? o.density : base.density,
    radius: o.radius === 'sharp' || o.radius === 'soft' || o.radius === 'round' ? o.radius : base.radius,
    spellcheck: typeof o.spellcheck === 'boolean' ? o.spellcheck : base.spellcheck
  };
}

const RADII: Record<RadiusChoice, [string, string, string]> = {
  sharp: ['3px', '5px', '8px'],
  soft: ['7px', '11px', '18px'],
  round: ['10px', '16px', '26px']
};

function apply(p: DisplayPrefs): void {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  const set = (k: string, v: string) => root.style.setProperty(k, v);

  set('--font-ui', fontCss(p.uiFont, DEFAULTS.uiFont));
  set('--editor-font', fontCss(p.editorFont, DEFAULTS.editorFont));
  set('--font-mono', fontCss(p.codeFont, DEFAULTS.codeFont));
  set('--font-heading', p.headingFont === 'ui' ? 'var(--font-ui)' : fontCss(p.headingFont, DEFAULTS.uiFont));

  set('--editor-font-size', `${p.fontSize / 16}rem`);
  set('--editor-line-height', String(p.lineHeight));
  set('--editor-weight', String(p.weight));
  set('--editor-spacing', `${p.letterSpacing / 100}em`);
  set('--editor-para', `${p.paragraphSpacing / 10}em`);
  set('--editor-width', `${p.width}rem`);
  set('--editor-align', p.align);
  set('--title-size', `${p.titleSize / 10}rem`);

  set('--accent-hue', String(p.accentHue));
  const [sm, md, lg] = RADII[p.radius];
  set('--radius-sm', sm);
  set('--radius', md);
  set('--radius-lg', lg);

  const compact = p.density === 'compact';
  set('--nav-h', compact ? '28px' : '32px');
  set('--item-pad', compact ? '0.4rem' : '0.65rem');
}

display.subscribe(apply);

let persistTimer: ReturnType<typeof setTimeout> | null = null;

/** Écriture groupée : un curseur déplacé ne déclenche pas cent écritures. */
function persist(): void {
  if (persistTimer !== null) clearTimeout(persistTimer);
  persistTimer = setTimeout(() => {
    persistTimer = null;
    void api.setSetting(DISPLAY_SETTING_KEY, JSON.stringify(get(display))).catch((err) => {
      console.warn("Enregistrement de la préférence d'affichage impossible :", err);
    });
  }, 250);
}

/** Change une préférence : applique tout de suite, persiste ensuite. */
export function setDisplayPref<K extends keyof DisplayPrefs>(key: K, value: DisplayPrefs[K]): void {
  display.update((prefs) => sanitize({ ...prefs, [key]: value }, prefs));
  persist();
}

/** Applique un ensemble de réglages (préréglage), en conservant le reste. */
export function applyPreset(partial: Partial<DisplayPrefs>): void {
  display.update((prefs) => sanitize({ ...prefs, ...partial }, prefs));
  persist();
}

/** Revient aux réglages d'origine. */
export function resetDisplay(): void {
  display.set({ ...DEFAULTS });
  persist();
}

/** Migre les anciens réglages (police, taille, largeur) vers le nouveau modèle. */
function fromLegacy(settings: Record<string, string>): Partial<DisplayPrefs> {
  const out: Partial<DisplayPrefs> = {};
  const font = { serif: 'source-serif', sans: 'inter', mono: 'jetbrains-mono' }[settings[LEGACY.font]];
  if (font) out.editorFont = font;
  const size = { compact: 15, normal: 17, large: 19 }[settings[LEGACY.size]];
  if (size) out.fontSize = size;
  const width = { narrow: 38, normal: 46, wide: 54 }[settings[LEGACY.width]];
  if (width) out.width = width;
  return out;
}

/** Applique les valeurs relues de la table `settings` (défauts sinon). */
export function applyStoredDisplay(settings: Record<string, string>): void {
  let stored: unknown = null;
  const raw = settings[DISPLAY_SETTING_KEY];
  if (raw) {
    try {
      stored = JSON.parse(raw);
    } catch {
      stored = null; // valeur corrompue : réglages par défaut
    }
  }
  display.set(sanitize(stored ?? fromLegacy(settings)));
}

/** Hydrate les préférences d'affichage depuis la base au démarrage. */
export async function loadDisplayPrefs(): Promise<void> {
  try {
    applyStoredDisplay(await api.getSettings());
  } catch (err) {
    console.warn("Chargement des préférences d'affichage impossible :", err);
  }
}
