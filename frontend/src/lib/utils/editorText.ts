/**
 * Helpers purs de l'éditeur : calculs de lignes, nettoyage de mise en
 * forme, pièces jointes. Aucun état Svelte ici — tout est testable.
 */

/** Bornes des lignes touchées par une sélection [start, end). */
export function selectedLines(content: string, start: number, end: number) {
  const lineStart = start === 0 ? 0 : content.lastIndexOf('\n', start - 1) + 1;
  const nextBreak = content.indexOf('\n', end);
  return { lineStart, blockEnd: nextBreak === -1 ? content.length : nextBreak };
}

/** Retire les marqueurs de mise en forme d'un fragment (§3.4). */
export function cleanFormatText(text: string): string {
  return text
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/==([^=]+)==/g, '$1')
    .replace(/(^|[^*])\*([^*\n]+)\*/g, '$1$2')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/^>\s?/gm, '')
    .replace(/^\s*([-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?/gm, '');
}

/** Borne miroir du backend : refuser tôt évite un aller-retour inutile. */
export const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024;

/** Extensions d'image reconnues localement : certaines WebView
 *  laissent `File.type` vide. Le backend affine ensuite le type. */
const IMAGE_EXT = /\.(png|jpe?g|gif|webp|bmp|avif|svgz?|ico|tiff?)$/i;

export function looksLikeImage(file: File): boolean {
  return file.type.startsWith('image/') || IMAGE_EXT.test(file.name);
}

/** Légende sûre pour un lien/une image Markdown. */
export function altText(name: string): string {
  return name.replace(/[[\]\n\r]/g, ' ').trim() || 'pièce jointe';
}

/** Lit un fichier en base64 (sans le préfixe data:). */
export function readFileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result ?? '');
      const comma = result.indexOf(',');
      resolve(comma >= 0 ? result.slice(comma + 1) : '');
    };
    reader.onerror = () => reject(reader.error ?? new Error('lecture impossible'));
    reader.readAsDataURL(file);
  });
}

/** Retire du texte les références Markdown à une pièce jointe : une
 *  référence orpheline afficherait une image cassée dans l'aperçu. */
export function stripAttachmentRefs(content: string, attachmentId: string): string {
  const id = attachmentId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const reference = new RegExp(`!?\\[[^\\]]*\\]\\(attachment:${id}\\)`, 'g');
  return content.replace(reference, '').replace(/[ \t]+\n/g, '\n');
}

/** Libellé court d'une pièce jointe : taille lisible, nom complet en info. */
export function attachmentSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} Mio`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} Kio`;
  return `${bytes} o`;
}

// --- cases à cocher cliquables dans l'aperçu -------------------------------

/** Reconnaît la même syntaxe que le moteur Markdown : item de liste
 *  dont le texte commence par `[ ]`, `[x]` ou `[X]`. */
const TASK_LINE_RE = /^(\s*)([-*+]|\d+[.)])\s+\[([ xX])\]\s+/;

/**
 * Bascule la n-ième case à cocher du document (ordre d'apparition, comme
 * les `data-task` attribués par l'aperçu). Renvoie le contenu modifié,
 * ou null si l'index est hors limites — l'appelant ne touche alors à
 * rien plutôt que de deviner.
 */
export function toggleNthTask(content: string, index: number): string | null {
  if (index < 0) return null;
  const lines = content.split('\n');
  let seen = -1;
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(TASK_LINE_RE);
    if (!m) continue;
    seen++;
    if (seen !== index) continue;
    const mark = m[3].toLowerCase() === 'x' ? ' ' : 'x';
    lines[i] = lines[i].replace(TASK_LINE_RE, `$1$2 [${mark}] `);
    return lines.join('\n');
  }
  return null;
}
