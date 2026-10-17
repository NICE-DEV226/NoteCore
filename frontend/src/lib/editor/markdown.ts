/**
 * Rendu Markdown minimal et sûr pour l'aperçu de la V1 (cdc.md §3.4).
 *
 * Règle de sécurité (cdc.md §6.4, ARCHITECTURE.md §7) : tout le texte
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
 * (cdc.md §6.4).
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

function renderInline(text: string): string {
  // code inline en premier : son contenu n'est plus retraité
  const parts = text.split('`');
  const inCode = parts.length % 2 === 0; // backtick impair : texte ouvert
  for (let i = 0; i < parts.length; i++) {
    if (i % 2 === 1) parts[i] = `<code class="md-inline">${parts[i]}</code>`;
  }
  const joined = (inCode ? parts.join('') : parts.join('')).slice(0); // marqueurs si impair

  let out = joined;
  // images ![légende](url) — traitées AVANT les liens, sinon le « ! »
  // laissé seul deviendrait un texte parasite devant le lien.
  out = out.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_m, alt: string, url: string) => {
    const raw = url.replace(/&quot;/g, '"');
    const att = attachmentId(raw);
    if (att) {
      // Résolue de façon asynchrone par l'aperçu (ADR-006).
      return `<img class="md-image md-attachment" data-attachment="${att}" src="" alt="${alt}">`;
    }
    const src = safeImageSrc(raw);
    if (!src) return alt;
    // Pas de referrer (confidentialité) et chargement différé.
    return `<img class="md-image" src="${escapeHtml(src)}" alt="${alt}" loading="lazy" referrerpolicy="no-referrer">`;
  });
  // liens [texte](url) — texte et url potentiellement échappés par "
  out = out.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (m, label: string, url: string) => {
    const raw = url.replace(/&quot;/g, '"');
    const att = attachmentId(raw);
    if (att) {
      return `<a class="md-attachment-link" data-attachment="${att}" href="#" title="Enregistrer la pièce jointe">${label}</a>`;
    }
    const href = safeHref(raw);
    if (!href) return label;
    return `<a href="${escapeHtml(href)}" rel="noreferrer">${label}</a>`;
  });
  // gras, mise en évidence, italique
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  out = out.replace(/==([^=]+)==/g, '<mark>$1</mark>');
  out = out.replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em>$2</em>');
  return out;
}

// --- mise en bloc ----------------------------------------------------------

export function renderMarkdown(source: string): string {
  const lines = source.replace(/\r\n?/g, '\n').split('\n');
  const out: string[] = [];
  let para: string[] = [];
  let listMode: 'ul' | 'ol' | null = null;
  let inCode = false;
  let codeLang = '';
  let codeBuf: string[] = [];
  let quoteMode = false;

  const flushParagraph = () => {
    if (para.length) {
      out.push(`<p>${renderInline(para.join('<br>'))}</p>`);
      para = [];
    }
  };
  const flushList = () => {
    if (listMode) {
      out.push(`</${listMode}>`);
      listMode = null;
    }
  };
  const closeQuote = () => {
    if (quoteMode) {
      out.push('</blockquote>');
      quoteMode = false;
    }
  };

  for (const raw of lines) {
    const line = raw;

    if (inCode) {
      if (line.trim().startsWith('```')) {
        const cls = codeLang ? ` class="language-${escapeHtml(codeLang)}"` : '';
        out.push(`<pre><code${cls}>${codeBuf.join('\n')}</code></pre>`);
        inCode = false;
        codeBuf = [];
        codeLang = '';
      } else {
        codeBuf.push(escapeHtml(line));
      }
      continue;
    }

    const fence = line.match(/^```(\S*)\s*$/);
    if (fence) {
      flushParagraph();
      flushList();
      closeQuote();
      inCode = true;
      codeLang = fence[1] ?? '';
      codeBuf = [];
      continue;
    }

    // ligne vide : sépareur d'éléments
    if (line.trim() === '') {
      flushParagraph();
      flushList();
      closeQuote();
      out.push('');
      continue;
    }

    // citation
    if (line.startsWith('>')) {
      flushParagraph();
      flushList();
      if (!quoteMode) {
        out.push('<blockquote>');
        quoteMode = true;
      }
      out.push(`<p>${renderInline(escapeHtml(line.replace(/^>\s*/, '')))}</p>`);
      continue;
    }

    // titres
    const heading = line.match(/^(#{1,6})\s+(.*)$/);
    if (heading) {
      flushParagraph();
      flushList();
      closeQuote();
      const level = heading[1].length;
      out.push(`<h${level}>${renderInline(escapeHtml(heading[2]))}</h${level}>`);
      continue;
    }

    // séparateur horizontal
    if (/^(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      flushParagraph();
      flushList();
      closeQuote();
      out.push('<hr>');
      continue;
    }

    // listes (items désordonnés, ordonnés, cases à cocher)
    const ul = line.match(/^[-*+]\s+(.*)$/);
    const ol = line.match(/^\d+[.)]\s+(.*)$/);
    if (ul || ol) {
      flushParagraph();
      closeQuote();
      const mode = ul ? 'ul' : 'ol';
      if (listMode !== mode) {
        flushList();
        out.push(`<${mode}>`);
        listMode = mode;
      }
      let item = (ul ? ul[1] : ol![1]) ?? '';
      // case à cocher simple : - [ ] / - [x]
      const box = item.match(/^\[([ xX])\]\s+(.*)$/);
      if (box) {
        const checked = box[1].toLowerCase() === 'x';
        item = `<label class="md-task"><input type="checkbox" disabled${checked ? ' checked' : ''}> ${renderInline(escapeHtml(box[2]))}</label>`;
      } else {
        item = renderInline(escapeHtml(item));
      }
      out.push(`<li>${item}</li>`);
      continue;
    }

    // paragraphe
    closeQuote();
    para.push(escapeHtml(line));
  }

  // fin de fichier
  flushParagraph();
  flushList();
  closeQuote();
  if (inCode) {
    const cls = codeLang ? ` class="language-${escapeHtml(codeLang)}"` : '';
    out.push(`<pre><code${cls}>${codeBuf.join('\n')}</code></pre>`);
  }

  return out.filter((b) => b.trim() !== '').join('\n');
}
