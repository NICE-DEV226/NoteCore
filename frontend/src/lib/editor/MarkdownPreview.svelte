<script lang="ts">
  import { renderMarkdown } from './markdown';
  import { api } from '../api/client';

  /** Contenu Markdown brut de la note. */
  let { content = '' } = $props();

  const html = $derived(renderMarkdown(content));

  let container = $state<HTMLElement | null>(null);

  function base64ToBlob(base64: string, mime: string): Blob {
    const bin = atob(base64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return new Blob([bytes], { type: mime });
  }

  function missingNode(label: string): HTMLElement {
    const span = document.createElement('span');
    span.className = 'md-attachment-missing';
    span.textContent = `Pièce jointe indisponible : ${label}`;
    return span;
  }

  // Après chaque rendu : on résout les images de pièces jointes depuis le
  // backend (les octets ne sont pas dans le Markdown). Une pièce jointe
  // n'est rendue en image que si son type est bien `image/*` (ADR-006).
  $effect(() => {
    void html; // rejouer quand le rendu change
    const root = container;
    if (!root) return;

    let live = true;
    const created: string[] = [];

    // Un lien de pièce jointe ne navigue pas : il enregistre le fichier.
    // Écoute déléguée : le contenu injecté n'est pas connu de Svelte.
    const onClick = (event: MouseEvent): void => {
      const target = event.target as HTMLElement | null;
      const link = target?.closest('a[data-attachment]') as HTMLAnchorElement | null;
      if (!link) return;
      event.preventDefault();
      const id = link.dataset.attachment;
      if (id) void api.saveAttachment(id);
    };
    root.addEventListener('click', onClick);

    root.querySelectorAll<HTMLImageElement>('img[data-attachment]').forEach((img) => {
      const id = img.dataset.attachment;
      if (!id) return;
      api
        .readAttachment(id)
        .then((data) => {
          if (!live) return;
          if (!data.mime.startsWith('image/')) {
            img.replaceWith(missingNode(img.alt || id));
            return;
          }
          const url = URL.createObjectURL(base64ToBlob(data.dataBase64, data.mime));
          created.push(url);
          img.src = url;
        })
        .catch(() => {
          if (live) img.replaceWith(missingNode(img.alt || id));
        });
    });

    return () => {
      live = false;
      root.removeEventListener('click', onClick);
      for (const url of created) URL.revokeObjectURL(url);
    };
  });
</script>

<!-- Le rendu est produit exclusivement par notre moteur (markdown.ts) :
     il est échappé puis balisé, sans HTML entrant ({@html} est sûr ici). -->
<div class="md-preview" bind:this={container}>
  {@html html}
</div>

<style>
  .md-preview {
    font-family: var(--editor-font);
    font-size: calc(var(--editor-font-size) * 1.02);
    line-height: 1.72;
    color: var(--text);
    overflow-wrap: break-word;
  }

  /* Les titres restent en sans : la structure visuelle se voit,
     la prose garde la voix choisie par l'utilisateur. */
  .md-preview :global(h1),
  .md-preview :global(h2),
  .md-preview :global(h3),
  .md-preview :global(h4) {
    font-family: var(--font-ui);
  }

  .md-preview :global(h1) {
    font-size: 1.35rem;
    margin: 1.1em 0 0.4em;
    padding-bottom: 0.3em;
    border-bottom: 1px solid var(--border);
  }
  .md-preview :global(h1:first-child) {
    margin-top: 0;
  }
  .md-preview :global(h2) {
    font-size: 1.12rem;
    margin: 1em 0 0.35em;
  }
  .md-preview :global(h3),
  .md-preview :global(h4) {
    font-size: 0.98rem;
    margin: 0.9em 0 0.3em;
  }

  .md-preview :global(p) {
    margin: 0.55em 0;
  }

  .md-preview :global(ul),
  .md-preview :global(ol) {
    margin: 0.5em 0;
    padding-left: 1.5em;
  }
  .md-preview :global(li) {
    margin: 0.2em 0;
  }

  .md-preview :global(blockquote) {
    margin: 0.7em 0;
    padding: 0.35em 0.9em;
    border-left: 3px solid var(--accent);
    background: var(--accent-soft);
    border-radius: 0 var(--radius) var(--radius) 0;
    color: var(--text-soft);
  }
  .md-preview :global(blockquote p) {
    margin: 0.2em 0;
  }

  .md-preview :global(pre) {
    background: var(--code-bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.8em 1em;
    overflow-x: auto;
    font-family: var(--font-mono);
    font-size: 0.86rem;
    line-height: 1.5;
    margin: 0.7em 0;
  }

  .md-preview :global(code.md-inline) {
    background: var(--code-bg);
    border: 1px solid var(--border);
    border-radius: 5px;
    padding: 0.08em 0.35em;
    font-family: var(--font-mono);
    font-size: 0.85em;
  }

  .md-preview :global(mark) {
    background: var(--mark-bg);
    border-radius: 3px;
    padding: 0 0.15em;
    color: inherit;
  }

  .md-preview :global(a) {
    color: var(--accent-text);
  }

  .md-preview :global(img.md-image) {
    display: block;
    max-width: 100%;
    height: auto;
    margin: 0.8em auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--code-bg);
  }

  .md-preview :global(img.md-attachment:not([src])),
  .md-preview :global(img.md-attachment[src='']) {
    min-height: 2.5rem;
  }

  .md-preview :global(a.md-attachment-link) {
    color: var(--accent-text);
    text-decoration-style: dotted;
    cursor: pointer;
  }

  .md-preview :global(.md-attachment-missing) {
    display: inline-block;
    padding: 0.15em 0.5em;
    border: 1px dashed var(--border);
    border-radius: var(--radius-sm);
    color: var(--text-faint);
    font-size: 0.85em;
  }

  .md-preview :global(hr) {
    border: none;
    border-top: 1px solid var(--border);
    margin: 1.2em 0;
  }

  .md-preview :global(label.md-task) {
    display: inline-flex;
    align-items: center;
    gap: 0.45em;
  }
</style>
