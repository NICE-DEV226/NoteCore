/**
 * Catalogue des polices proposées dans les paramètres. Toutes celles marquées
 * `bundled` sont embarquées dans l'application (src/fonts.css) : aucune
 * requête réseau, le rendu est identique sur toutes les machines.
 */

export type FontCategory = 'serif' | 'sans' | 'mono' | 'script' | 'system';

export interface FontOption {
  id: string;
  label: string;
  category: FontCategory;
  /** Valeur CSS `font-family`, avec repli système. */
  css: string;
}

const SERIF_FALLBACK = "Georgia, 'Iowan Old Style', Charter, Cambria, 'Times New Roman', serif";
const SANS_FALLBACK = "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif";
const MONO_FALLBACK = "ui-monospace, 'Cascadia Code', Consolas, 'DejaVu Sans Mono', monospace";

const f = (
  id: string,
  label: string,
  category: FontCategory,
  family: string,
  fallback: string
): FontOption => ({ id, label, category, css: `'${family} Variable', ${fallback}` });

export const FONTS: FontOption[] = [
  // Empattement : textes longs, cours, lecture
  f('source-serif', 'Source Serif 4', 'serif', 'Source Serif 4', SERIF_FALLBACK),
  f('lora', 'Lora', 'serif', 'Lora', SERIF_FALLBACK),
  f('literata', 'Literata', 'serif', 'Literata', SERIF_FALLBACK),
  f('newsreader', 'Newsreader', 'serif', 'Newsreader', SERIF_FALLBACK),
  f('crimson', 'Crimson Pro', 'serif', 'Crimson Pro', SERIF_FALLBACK),
  f('garamond', 'EB Garamond', 'serif', 'EB Garamond', SERIF_FALLBACK),
  f('fraunces', 'Fraunces', 'serif', 'Fraunces', SERIF_FALLBACK),
  f('playfair', 'Playfair Display', 'serif', 'Playfair Display', SERIF_FALLBACK),
  // Sans empattement : interface et écriture moderne
  f('inter', 'Inter', 'sans', 'Inter', SANS_FALLBACK),
  f('dm-sans', 'DM Sans', 'sans', 'DM Sans', SANS_FALLBACK),
  f('manrope', 'Manrope', 'sans', 'Manrope', SANS_FALLBACK),
  f('nunito', 'Nunito Sans', 'sans', 'Nunito Sans', SANS_FALLBACK),
  f('jakarta', 'Plus Jakarta Sans', 'sans', 'Plus Jakarta Sans', SANS_FALLBACK),
  f('lexend', 'Lexend (lecture facilitée)', 'sans', 'Lexend', SANS_FALLBACK),
  f('space-grotesk', 'Space Grotesk', 'sans', 'Space Grotesk', SANS_FALLBACK),
  // Chasse fixe : code, notes techniques
  f('jetbrains-mono', 'JetBrains Mono', 'mono', 'JetBrains Mono', MONO_FALLBACK),
  f('fira-code', 'Fira Code', 'mono', 'Fira Code', MONO_FALLBACK),
  f('source-code', 'Source Code Pro', 'mono', 'Source Code Pro', MONO_FALLBACK),
  f('roboto-mono', 'Roboto Mono', 'mono', 'Roboto Mono', MONO_FALLBACK),
  f('inconsolata', 'Inconsolata', 'mono', 'Inconsolata', MONO_FALLBACK),
  f('red-hat-mono', 'Red Hat Mono', 'mono', 'Red Hat Mono', MONO_FALLBACK),
  // Écriture manuscrite
  f('caveat', 'Caveat (manuscrit)', 'script', 'Caveat', "'Segoe Script', 'Comic Sans MS', cursive"),
  // Polices du système : rendu natif, aucune police embarquée
  { id: 'sys-sans', label: 'Système (sans)', category: 'system', css: SANS_FALLBACK },
  { id: 'sys-serif', label: 'Système (serif)', category: 'system', css: SERIF_FALLBACK },
  { id: 'sys-mono', label: 'Système (mono)', category: 'system', css: MONO_FALLBACK }
];

export const FONT_CATEGORY_LABELS: Record<FontCategory, string> = {
  serif: 'Avec empattement',
  sans: 'Sans empattement',
  mono: 'Chasse fixe',
  script: 'Manuscrit',
  system: 'Système'
};

const byId = new Map(FONTS.map((x) => [x.id, x]));

export function fontCss(id: string, fallbackId: string): string {
  return (byId.get(id) ?? byId.get(fallbackId) ?? FONTS[0]).css;
}

export function isFontId(id: unknown): id is string {
  return typeof id === 'string' && byId.has(id);
}
