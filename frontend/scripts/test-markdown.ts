import { renderMarkdown } from '../src/lib/editor/markdown.ts';

const src = `# Titre à éàûç

Liste accentuée :
- contrôle d'erreur
- câble
- mémoire

1. premier point
2. deuxième point

\`\`\`c
int main(void) { return 0; }
\`\`\`

Code inline \`SELECT * FROM notes\` et ==surlignage== en **gras**.

Lien [cours](https://example.org) et injection \`<script>alert('xss')</script>\`;
`;

const html = renderMarkdown(src);

console.log(html);
console.log('---');
const checks = [
  ['titre', html.includes('<h1>')],
  ['accents conservés', html.includes('éàûç') && html.includes("d'erreur") && html.includes('câble')],
  ['liste à puces', html.includes('<ul>') && html.includes('<li>')],
  ['liste ordonnée', html.includes('<ol>')],
  ['bloc de code', html.includes('<pre><code') && html.includes('int main(void)')],
  ['code inline', html.includes('<code class="md-inline">SELECT * FROM notes</code>')],
  ['évidence', html.includes('<mark>surlignage</mark>')],
  ['gras', html.includes('<strong>gras</strong>')],
  ['lien sûr', html.includes('href="https://example.org"')],
  ['XSS neutralisé', !/<script/i.test(html) && html.includes('&lt;script&gt;')]
];

const md = renderMarkdown;
const more: [string, boolean][] = [
  ['code non retraité', md('`**x** [a](https://b.c)`') === '<p><code class="md-inline">**x** [a](https://b.c)</code></p>'],
  ['backtick isolé littéral', !md('`a et b').includes('<code')],
  ['gras contenant italique', md('**a *b* c**').includes('<strong>a <em>b</em> c</strong>')],
  ['lien avec parenthèses', md('[a](https://x.com/a_(b))').includes('href="https://x.com/a_(b)"')],
  ['esperluette dans une URL', md('[a](https://x.com/?a=1&b=2)').includes('href="https://x.com/?a=1&amp;b=2"')],
  ['injection par attribut neutralisée', !/onmouseover=[^&]/.test(md('[x](https://a.com/"onmouseover="alert(1))'))],
  ['javascript: refusé', !md('[x](javascript:alert(1))').includes('href')],
  ['liste imbriquée', /<ol>\s*<li>a<ul><li>b<\/li><\/ul><\/li><\/ol>/.test(md('1. a\n   - b'))],
  ['numérotation continue', (md('1. a\n\n2. b').match(/<ol/g) ?? []).length === 1],
  ['départ de liste', md('3. a\n4. b').includes('<ol start="3">')],
  ['tableau', md('| a | b |\n|---|:-:|\n| 1 | 2 |').includes('<th>a</th><th style="text-align:center">b</th>')],
  ['citation multi-lignes', md('> a\n> b').startsWith('<blockquote><p>a<br>b</p></blockquote>')],
  ['citation contenant une liste', md('> - a').includes('<blockquote><ul><li>a</li></ul></blockquote>')],
  ['bloc de code non fermé', md('```\nx').includes('<pre><code>x</code></pre>')],
  ['balise HTML échappée', md('<img src=x onerror=alert(1)>').includes('&lt;img')],
  ['trou de révision', md('Le {{c1::noyau}} gère').includes('<span class="md-cloze" title="Carte c1">noyau</span>')],
  ['trou avec indice et gras', md('{{c2::**fork**::appel}}').includes('title="Carte c2 — indice : appel"><strong>fork</strong></span>')],
  ['trou non fermé laissé tel quel', !md('{{c1::sans fin').includes('md-cloze')],
  ['trou dans du code ignoré', !md('`{{c1::x}}`').includes('md-cloze')],
  ['pièce jointe', md('![a](attachment:abc-1)').includes('data-attachment="abc-1"')]
];
checks.push(...more);

let ok = true;
for (const [name, pass] of checks) {
  console.log(`${pass ? 'PASS' : 'FAIL'} — ${name}`);
  if (!pass) ok = false;
}
process.exit(ok ? 0 : 1);
