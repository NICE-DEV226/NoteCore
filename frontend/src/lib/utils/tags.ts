/**
 * Attribution déterministe d'une teinte pastel à une étiquette.
 *
 * Le design system (app.css) définit quatre pastels de surligneur — bleu,
 * vert, jaune, violet — utilisés comme couleurs d'étiquettes. On répartit le
 * nom sur quatre seaux par un hachage FNV-1a : le choix est stable (une même
 * étiquette garde toujours la même couleur) et ne requiert aucun stockage.
 */
export const TAG_TINTS = ['blue', 'green', 'yellow', 'purple'] as const;

export type TagTint = (typeof TAG_TINTS)[number];

/** Renvoie la teinte pastel associée à une étiquette. */
export function tagTint(tag: string): TagTint {
  let hash = 0x811c9dc5;
  for (let i = 0; i < tag.length; i += 1) {
    hash ^= tag.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return TAG_TINTS[(hash >>> 0) % TAG_TINTS.length];
}
