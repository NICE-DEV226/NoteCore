/**
 * Helpers de présentation du texte et des dates (interface en français).
 */

const unitsMinutes: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ['minute', 60_000],
  ['hour', 3_600_000],
  ['day', 86_400_000]
];

const rtf = new Intl.RelativeTimeFormat('fr', { numeric: 'auto' });

/** « il y a 3 minutes », « hier », « il y a 2 jours », sinon date courte. */
export function formatRelative(iso: string): string {
  const date = new Date(iso);
  const deltaMs = date.getTime() - Date.now();
  const abs = Math.abs(deltaMs);

  // Au-delà de six jours : la date suffit.
  if (abs > 6 * 86_400_000) {
    return date.toLocaleDateString('fr-FR', { day: 'numeric', month: 'short', year: 'numeric' });
  }

  for (let i = unitsMinutes.length - 1; i >= 0; i--) {
    const [unit, ms] = unitsMinutes[i];
    if (abs >= ms) return rtf.format(Math.round(deltaMs / ms), unit);
  }
  return "à l'instant";
}

export function formatFullDate(iso: string): string {
  return new Date(iso).toLocaleDateString('fr-FR', {
    day: 'numeric',
    month: 'long',
    year: 'numeric'
  });
}

/** Compte de mots du contenu brut. */
export function countWords(text: string): number {
  const trimmed = text.trim();
  return trimmed ? trimmed.split(/\s+/).length : 0;
}

/** Un fragment d'extrait de recherche : terme trouvé, ou texte de liaison. */
export interface SnippetPart {
  text: string;
  hit: boolean;
}

/**
 * Découpe l'extrait renvoyé par le moteur de recherche (cahier des charges §3.3,
 * ADR-003). Le backend encadre déjà les termes trouvés de guillemets
 * français « » ; on en fait des fragments surlignables, sans jamais
 * interpréter le contenu comme du HTML.
 */
export function snippetParts(raw: string, max = 160): SnippetPart[] {
  const flat = raw.replace(/\s+/g, ' ').trim();
  if (flat.length > max) return [{ text: `${flat.slice(0, max).trimEnd()}…`, hit: false }];

  const parts: SnippetPart[] = [];
  const re = /«([^»]*)»/g;
  let last = 0;
  let match: RegExpExecArray | null;
  while ((match = re.exec(flat)) !== null) {
    if (match.index > last) parts.push({ text: flat.slice(last, match.index), hit: false });
    parts.push({ text: match[1], hit: true });
    last = match.index + match[0].length;
  }
  if (last < flat.length) parts.push({ text: flat.slice(last), hit: false });
  return parts;
}

/** Extrait de texte brut pour la liste des notes (Zone B, cahier des charges §4.2). */
export function excerpt(content: string, max = 90): string {
  const flat = content
    .replace(/```[\s\S]*?```/g, ' [code] ')
    .replace(/[#>*`~\[\]()=_-]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (flat.length <= max) return flat;
  return `${flat.slice(0, max).trimEnd()}…`;
}
