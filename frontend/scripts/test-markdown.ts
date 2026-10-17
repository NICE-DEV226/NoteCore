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

let ok = true;
for (const [name, pass] of checks) {
  console.log(`${pass ? 'PASS' : 'FAIL'} — ${name}`);
  if (!pass) ok = false;
}
process.exit(ok ? 0 : 1);
