/**
 * Rendu Markdown minimal et sûr pour l'aperçu de la V1 (cahier des charges §3.4).
 *
 * Règle de sécurité (cahier des charges §6.4, ARCHITECTURE.md §7) : tout le texte
 * d'entrée est d'abord échappé, puis balisé par ce module uniquement.
 * Aucun HTML inline n'est interprété et seuls les liens http(s)/mailto/
 * internes reçoivent un href. Les images distantes sont limitées à
 * http(s) — jamais `data:`, `file:` ou `javascript:`. Conséquence :
 * aucun contenu actif ne peut atteindre l'aperçu.
 */

const escapeHtml = (s: string): string =>
  s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');

function safeHref(url: string): string | null {
  const trimmed = url.trim();
  try {
    if (trimmed.startsWith('#')) return trimmed;
    const u = new URL(trimmed);
    if (u.protocol === 'http:' || u.protocol === 'https:' || u.protocol === 'mailto:') {
      return trimmed;
    }
  } catch {
    return null;
  }
  return null;
}

/**
 * Une image n'est chargée que depuis http(s). On refuse `javascript:`,
 * `data:` (vecteur de SVG actif) et `file:` : l'aperçu ne doit jamais
 * exécuter ni lire autre chose que le réseau demandé explicitement
 * (cahier des charges §6.4).
 */
function safeImageSrc(url: string): string | null {
  const trimmed = url.trim();
  try {
    const u = new URL(trimmed);
    if (u.protocol === 'http:' || u.protocol === 'https:') return trimmed;
  } catch {
    return null;
  }
  return null;
}

/**
 * Une pièce jointe locale est référencée `attachment:<id>` (ADR-006).
 * L'identifiant est strictement borné : il vient de NoteCore, jamais
 * d'un chemin arbitraire. Les octets sont résolus par l'aperçu, pas ici.
 */
const ATTACHMENT_RE = /^attachment:([A-Za-z0-9-]{1,64})$/;

function attachmentId(url: string): string | null {
  const m = ATTACHMENT_RE.exec(url.trim());
  return m ? m[1] : null;
}

// --- rendu en ligne (reçoit du texte déjà échappé) ------------------------

/** Décode les entités produites par escapeHtml, pour valider une URL. */
const unescapeHtml = (s: string): string =>
  s
    .replace(/&quot;/g, '"')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');

// Parenthèses : un niveau d'imbrication toléré dans l'URL (`a_(b)`).
const URL_PART = '((?:[^()]|\\([^()]*\\))+)';
const IMAGE_RE = new RegExp(`!\\[([^\\]]*)\\]\\(${URL_PART}\\)`, 'g');
const LINK_RE = new RegExp(`\\[([^\\]]+)\\]\\(${URL_PART}\\)`, 'g');

/**
 * Le balisage produit (code, liens, images) est mis de côté sous forme de
 * marqueurs avant le traitement du gras et de l'italique : le contenu d'un
 * code ou d'une URL n'est ainsi jamais retraité. Le caractère NUL est retiré
 * de l'entrée en amont, aucun marqueur ne peut donc être forgé.
 */
function renderInline(text: string): string {
  const stash: string[] = [];
  const keep = (html: string): string => `\u0000${stash.push(html) - 1}\u0000`;

  // code inline d'abord ; un backtick isolé reste un simple caractère
  let out = text.replace(/`([^`\n]+)`/g, (_m, code: string) =>
    keep(`<code class="md-inline">${code}</code>`)
  );

  // trous de révision {{c1::réponse}} / {{c1::réponse::indice}} (ADR-007) :
  // dans une note, la réponse reste lisible, mise en évidence. Seules les
  // balises sont isolées : la réponse garde son gras, son lien, etc.
  out = out.replace(/\{\{c(\d{1,2})::(.+?)(?:::(.+?))?\}\}/g, (_m, n: string, answer: string, hint?: string) => {
    const title = hint ? `Carte c${n} — indice : ${hint}` : `Carte c${n}`;
    return `${keep(`<span class="md-cloze" title="${title}">`)}${answer}${keep('</span>')}`;
  });

  // images ![légende](url) — AVANT les liens, sinon le « ! » resterait seul
  out = out.replace(IMAGE_RE, (_m, alt: string, url: string) => {
    const raw = unescapeHtml(url.trim());
    const att = attachmentId(raw);
    if (att) {
      // Résolue de façon asynchrone par l'aperçu (ADR-006).
      return keep(`<img class="md-image md-attachment" data-attachment="${att}" src="" alt="${alt}">`);
    }
    const src = safeImageSrc(raw);
    if (!src) return alt;
    // Pas de referrer (confidentialité) et chargement différé.
    return keep(
      `<img class="md-image" src="${escapeHtml(src)}" alt="${alt}" loading="lazy" referrerpolicy="no-referrer">`
    );
  });

  // liens [texte](url) : seules les balises sont isolées, le texte garde
  // son gras / italique
  out = out.replace(LINK_RE, (_m, label: string, url: string) => {
    const raw = unescapeHtml(url.trim());
    const att = attachmentId(raw);
    if (att) {
      return `${keep(`<a class="md-attachment-link" data-attachment="${att}" href="#" title="Enregistrer la pièce jointe">`)}${label}${keep('</a>')}`;
    }
    const href = safeHref(raw);
    if (!href) return label;
    return `${keep(`<a href="${escapeHtml(href)}" rel="noreferrer">`)}${label}${keep('</a>')}`;
  });

  // gras, mise en évidence, italique (imbrication permise)
  out = out.replace(/\*\*(?=\S)(.+?)(?<=\S)\*\*/g, '<strong>$1</strong>');
  out = out.replace(/==(?=\S)(.+?)(?<=\S)==/g, '<mark>$1</mark>');
  out = out.replace(/(^|[^*])\*(?=[^\s*])([^*\n]+?)(?<=[^\s*])\*(?!\*)/g, '$1<em>$2</em>');

  return out.replace(/\u0000(\d+)\u0000/g, (_m, i: string) => stash[Number(i)] ?? '');
}

// --- mise en bloc ----------------------------------------------------------

const FENCE_RE = /^\s{0,3}```(\S*)\s*$/;
const HEADING_RE = /^(#{1,6})\s+(.*)$/;
const HR_RE = /^\s{0,3}(-{3,}|\*{3,}|_{3,})\s*$/;
const ITEM_RE = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;
const TABLE_SEP_RE = /^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/;

/** Découpe une ligne de tableau en cellules (`\|` reste un caractère). */
function splitCells(line: string): string[] {
  let body = line.trim();
  if (body.startsWith('|')) body = body.slice(1);
  if (body.endsWith('|') && !body.endsWith('\\|')) body = body.slice(0, -1);
  return body.split(/(?<!\\)\|/).map((c) => c.replace(/\\\|/g, '|').trim());
}

const isTableStart = (line: string, next: string | undefined): boolean =>
  next !== undefined &&
  line.includes('|') &&
  next.includes('-') &&
  TABLE_SEP_RE.test(next) &&
  splitCells(line).length === splitCells(next).length;

function renderTable(header: string, sep: string, rows: string[]): string {
  const align = splitCells(sep).map((c) =>
    c.startsWith(':') && c.endsWith(':') ? 'center' : c.endsWith(':') ? 'right' : c.startsWith(':') ? 'left' : ''
  );
  const cell = (tag: 'th' | 'td', text: string, i: number): string => {
    const style = align[i] ? ` style="text-align:${align[i]}"` : '';
    return `<${tag}${style}>${renderInline(escapeHtml(text))}</${tag}>`;
  };
  const head = splitCells(header).map((c, i) => cell('th', c, i)).join('');
  const body = rows
    .map((r) => {
      const cells = splitCells(r);
      const padded = align.map((_a, i) => cells[i] ?? '');
      return `<tr>${padded.map((c, i) => cell('td', c, i)).join('')}</tr>`;
    })
    .join('');
  return `<table><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table>`;
}

function renderItemText(raw: string): string {
  const box = raw.match(/^\[([ xX])\]\s+(.*)$/);
  if (box) {
    const checked = box[1].toLowerCase() === 'x';
    return `<label class="md-task"><input type="checkbox" disabled${checked ? ' checked' : ''}> ${renderInline(escapeHtml(box[2]))}</label>`;
  }
  return renderInline(escapeHtml(raw));
}

/** Rend un bloc de lignes de liste (items et continuations), imbrication comprise. */
function renderList(lines: string[]): string {
  const out: string[] = [];
  const stack: { tag: 'ul' | 'ol'; indent: number }[] = [];

  for (const line of lines) {
    const m = line.match(ITEM_RE);
    if (!m) {
      // continuation indentée : même item, nouvelle ligne
      out.push(`<br>${renderInline(escapeHtml(line.trim()))}`);
      continue;
    }
    const indent = m[1].replace(/\t/g, '    ').length;
    const tag: 'ul' | 'ol' = /^\d/.test(m[2]) ? 'ol' : 'ul';

    if (stack.length === 0) {
      out.push(openList(tag, m[2]));
      stack.push({ tag, indent });
    } else if (indent > stack[stack.length - 1].indent) {
      out.push(openList(tag, m[2])); // sous-liste, dans l'item courant
      stack.push({ tag, indent });
    } else {
      while (stack.length > 1 && indent < stack[stack.length - 1].indent) {
        out.push(`</li></${stack.pop()!.tag}>`);
      }
      const top = stack[stack.length - 1];
      if (top.tag !== tag) {
        out.push(`</li></${top.tag}>`);
        out.push(openList(tag, m[2]));
        stack[stack.length - 1] = { tag, indent: top.indent };
      } else {
        out.push('</li>');
      }
    }
    out.push(`<li>${renderItemText(m[3])}`);
  }
  while (stack.length) out.push(`</li></${stack.pop()!.tag}>`);
  return out.join('');
}

function openList(tag: 'ul' | 'ol', marker: string): string {
  if (tag === 'ol') {
    const start = parseInt(marker, 10);
    if (start > 1) return `<ol start="${start}">`;
  }
  return `<${tag}>`;
}

export function renderMarkdown(source: string): string {
  const lines = source.replace(/\r\n?/g, '\n').replace(/\u0000/g, '').split('\n');
  const out: string[] = [];
  let para: string[] = [];

  const flushParagraph = () => {
    if (para.length) {
      out.push(`<p>${renderInline(para.join('<br>'))}</p>`);
      para = [];
    }
  };
  const startsBlock = (line: string, next: string | undefined): boolean =>
    FENCE_RE.test(line) ||
    HEADING_RE.test(line) ||
    HR_RE.test(line) ||
    ITEM_RE.test(line) ||
    line.startsWith('>') ||
    isTableStart(line, next);

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];

    // bloc de code : tout est littéral jusqu'à la clôture (ou la fin)
    const fence = line.match(FENCE_RE);
    if (fence) {
      flushParagraph();
      const lang = fence[1] ?? '';
      const buf: string[] = [];
      i++;
      while (i < lines.length && !FENCE_RE.test(lines[i])) buf.push(escapeHtml(lines[i++]));
      i++; // clôture
      const cls = lang ? ` class="language-${escapeHtml(lang)}"` : '';
      out.push(`<pre><code${cls}>${buf.join('\n')}</code></pre>`);
      continue;
    }

    if (line.trim() === '') {
      flushParagraph();
      i++;
      continue;
    }

    const heading = line.match(HEADING_RE);
    if (heading) {
      flushParagraph();
      const level = heading[1].length;
      out.push(`<h${level}>${renderInline(escapeHtml(heading[2]))}</h${level}>`);
      i++;
      continue;
    }

    if (HR_RE.test(line)) {
      flushParagraph();
      out.push('<hr>');
      i++;
      continue;
    }

    if (line.startsWith('>')) {
      flushParagraph();
      const inner: string[] = [];
      while (i < lines.length && lines[i].startsWith('>')) inner.push(lines[i++].replace(/^>\s?/, ''));
      out.push(`<blockquote>${renderMarkdown(inner.join('\n'))}</blockquote>`);
      continue;
    }

    if (isTableStart(line, lines[i + 1])) {
      flushParagraph();
      const header = line;
      const sep = lines[i + 1];
      i += 2;
      const rows: string[] = [];
      while (i < lines.length && lines[i].trim() !== '' && lines[i].includes('|')) rows.push(lines[i++]);
      out.push(renderTable(header, sep, rows));
      continue;
    }

    if (ITEM_RE.test(line)) {
      flushParagraph();
      const block: string[] = [];
      while (i < lines.length) {
        const cur = lines[i];
        if (ITEM_RE.test(cur) && !HR_RE.test(cur)) {
          block.push(cur);
        } else if (cur.trim() === '') {
          // une ligne vide ne ferme pas la liste si un autre item suit
          let j = i;
          while (j < lines.length && lines[j].trim() === '') j++;
          if (j < lines.length && ITEM_RE.test(lines[j]) && !HR_RE.test(lines[j])) {
            i = j;
            continue;
          }
          break;
        } else if (/^\s{2,}\S|^\t/.test(cur) && !startsBlock(cur, undefined)) {
          block.push(cur);
        } else {
          break;
        }
        i++;
      }
      out.push(renderList(block));
      continue;
    }

    // paragraphe : se prolonge jusqu'à une ligne vide ou un autre bloc
    para.push(escapeHtml(line));
    i++;
    while (i < lines.length && lines[i].trim() !== '' && !startsBlock(lines[i], lines[i + 1])) {
      para.push(escapeHtml(lines[i++]));
    }
    flushParagraph();
  }

  flushParagraph();
  return out.join('\n');
}
