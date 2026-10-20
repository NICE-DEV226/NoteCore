<script lang="ts">
  /**
   * Visionneuse de pièces jointes : lire sans quitter NoteCore. Images (zoom),
   * PDF (pdf.js, embarqué : aucun module du système requis), Markdown, texte et
   * code, tableaux CSV, contenu d'une archive .zip, audio et vidéo. Pour le
   * reste — ou sur demande — ouverture avec l'application du système.
   *
   * Tout est rendu sans exécuter le contenu : le texte passe par textContent,
   * le Markdown par notre moteur (échappé puis balisé), un PDF par pdf.js dans
   * son worker, une image par <img>.
   */
  import { onMount, tick } from 'svelte';
  import Icon from './Icon.svelte';
  import { api } from '../api/client';
  import { renderMarkdown } from '../editor/markdown';
  import { closeViewer, formatBytes, kindOf, KIND_LABELS, stepViewer, viewer, type FileKind } from '../stores/files';
  import { selectNote } from '../stores/workspace';
  import type { ArchiveEntry } from '../api/types';

  type PdfDoc = { numPages: number; getPage(n: number): Promise<any>; destroy(): Promise<void> };

  const vs = $derived($viewer);
  const file = $derived(vs?.current ?? null);
  const kind = $derived<FileKind>(file ? kindOf(file) : 'other');
  const position = $derived(vs ? vs.siblings.findIndex((a) => a.id === vs.current.id) : -1);
  const total = $derived(vs?.siblings.length ?? 0);

  let loading = $state(false);
  let error = $state('');
  let notice = $state('');
  let blobUrl = $state('');
  let text = $state('');
  let partial = $state(false);
  let rows = $state<string[][]>([]);
  let entries = $state<ArchiveEntry[]>([]);
  let archiveTruncated = $state(false);
  let zoom = $state(1);
  let pdfEl = $state<HTMLDivElement | null>(null);
  let pdfDoc: PdfDoc | null = null;
  let pdfPages = $state(0);
  let dialogEl = $state<HTMLDivElement | null>(null);

  const MAX_TEXT_CHARS = 1_000_000;
  const MAX_ROWS = 500;
  const MAX_COLS = 60;

  function toBytes(base64: string): Uint8Array<ArrayBuffer> {
    const bin = atob(base64);
    const out = new Uint8Array(new ArrayBuffer(bin.length));
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }

  /** Découpe un CSV/TSV en respectant les guillemets ; détecte « , », « ; » ou tabulation. */
  function parseTable(src: string): string[][] {
    const firstLine = src.split('\n', 1)[0] ?? '';
    const delim = [',', ';', '\t'].map((d) => [d, firstLine.split(d).length] as const).sort((a, b) => b[1] - a[1])[0][0];
    const out: string[][] = [];
    let row: string[] = [];
    let cell = '';
    let quoted = false;
    for (let i = 0; i < src.length && out.length <= MAX_ROWS; i++) {
      const c = src[i];
      if (quoted) {
        if (c === '"' && src[i + 1] === '"') {
          cell += '"';
          i++;
        } else if (c === '"') quoted = false;
        else cell += c;
      } else if (c === '"') quoted = true;
      else if (c === delim) {
        row.push(cell);
        cell = '';
      } else if (c === '\n' || c === '\r') {
        if (c === '\r' && src[i + 1] === '\n') i++;
        row.push(cell);
        out.push(row.slice(0, MAX_COLS));
        row = [];
        cell = '';
      } else cell += c;
    }
    if (cell !== '' || row.length) {
      row.push(cell);
      out.push(row.slice(0, MAX_COLS));
    }
    return out;
  }

  async function destroyPdf(): Promise<void> {
    const d = pdfDoc;
    pdfDoc = null;
    pdfPages = 0;
    if (pdfEl) pdfEl.replaceChildren();
    if (d) await d.destroy().catch(() => undefined);
  }

  // Charge le contenu à chaque changement de pièce jointe.
  $effect(() => {
    const current = file;
    const k = kind;
    if (!current) return;
    let cancelled = false;
    let created = '';
    loading = true;
    error = '';
    notice = '';
    blobUrl = '';
    text = '';
    partial = false;
    rows = [];
    entries = [];
    zoom = 1;
    void destroyPdf();

    void (async () => {
      try {
        if (k === 'other') return;
        if (k === 'archive') {
          const listing = await api.listArchive(current.id);
          if (!cancelled) {
            entries = listing.entries;
            archiveTruncated = listing.truncated;
          }
          return;
        }
        const data = await api.readAttachment(current.id);
        if (cancelled) return;
        const bytes = toBytes(data.dataBase64);
        if (k === 'image' || k === 'audio' || k === 'video') {
          created = URL.createObjectURL(new Blob([bytes], { type: data.mime || 'application/octet-stream' }));
          blobUrl = created;
        } else if (k === 'pdf') {
          await renderPdf(bytes, () => cancelled);
        } else {
          let decoded = new TextDecoder('utf-8').decode(bytes);
          if (decoded.length > MAX_TEXT_CHARS) {
            decoded = decoded.slice(0, MAX_TEXT_CHARS);
            partial = true;
          }
          text = decoded;
          if (k === 'table') rows = parseTable(decoded);
        }
      } catch (err) {
        if (!cancelled) error = err instanceof Error ? err.message : 'Ce fichier ne peut pas être lu.';
      } finally {
        if (!cancelled) loading = false;
      }
    })();

    return () => {
      cancelled = true;
      if (created) URL.revokeObjectURL(created);
      void destroyPdf();
    };
  });

  async function renderPdf(bytes: Uint8Array, isCancelled: () => boolean): Promise<void> {
    // Chargé à la demande : le poids de pdf.js ne pèse pas sur le démarrage.
    const pdfjs = await import('pdfjs-dist/legacy/build/pdf.min.mjs');
    const worker = (await import('pdfjs-dist/legacy/build/pdf.worker.min.mjs?url')).default;
    pdfjs.GlobalWorkerOptions.workerSrc = worker;
    const doc = (await pdfjs.getDocument({ data: bytes.slice() }).promise) as unknown as PdfDoc;
    if (isCancelled()) {
      await doc.destroy();
      return;
    }
    pdfDoc = doc;
    pdfPages = doc.numPages;
    loading = false;
    await tick(); // le conteneur est affiché : on peut mesurer sa largeur
    await layoutPdf();
  }

  let observer: IntersectionObserver | null = null;

  /** Prépare un emplacement par page ; chaque page n'est dessinée que lorsqu'elle approche de l'écran. */
  async function layoutPdf(): Promise<void> {
    const doc = pdfDoc;
    const host = pdfEl;
    if (!doc || !host) return;
    observer?.disconnect();
    host.replaceChildren();

    const first = await doc.getPage(1);
    const base = first.getViewport({ scale: 1 });
    const width = Math.max(240, (host.clientWidth - 48) * zoom);
    const scale = width / base.width;
    const height = base.height * scale;

    observer = new IntersectionObserver(
      (items) => {
        for (const item of items) {
          if (!item.isIntersecting) continue;
          const el = item.target as HTMLDivElement;
          observer?.unobserve(el);
          void drawPage(el, Number(el.dataset.page), width);
        }
      },
      { root: host, rootMargin: '600px 0px' }
    );
    for (let i = 1; i <= doc.numPages; i++) {
      const slot = document.createElement('div');
      slot.className = 'pdf-page';
      slot.dataset.page = String(i);
      slot.style.width = `${width}px`;
      slot.style.height = `${height}px`;
      host.appendChild(slot);
      observer.observe(slot);
    }
  }

  async function drawPage(slot: HTMLDivElement, n: number, width: number): Promise<void> {
    const doc = pdfDoc;
    if (!doc) return;
    try {
      const page = await doc.getPage(n);
      const base = page.getViewport({ scale: 1 });
      const viewport = page.getViewport({ scale: width / base.width });
      const ratio = window.devicePixelRatio || 1;
      const canvas = document.createElement('canvas');
      canvas.width = Math.floor(viewport.width * ratio);
      canvas.height = Math.floor(viewport.height * ratio);
      canvas.style.width = `${viewport.width}px`;
      canvas.style.height = `${viewport.height}px`;
      slot.style.height = `${viewport.height}px`;
      const ctx = canvas.getContext('2d');
      if (!ctx) return;
      await page.render({ canvasContext: ctx, viewport, transform: ratio !== 1 ? [ratio, 0, 0, ratio, 0, 0] : undefined }).promise;
      if (pdfDoc === doc) slot.replaceChildren(canvas);
    } catch {
      /* page annulée (fichier changé ou fermé) */
    }
  }

  // Un changement de zoom redessine le PDF.
  let lastZoom = 1;
  $effect(() => {
    const z = zoom;
    if (kind === 'pdf' && pdfDoc && z !== lastZoom) void layoutPdf();
    lastZoom = z;
  });

  function zoomBy(factor: number): void {
    zoom = Math.min(4, Math.max(0.4, +(zoom * factor).toFixed(3)));
  }

  async function openWithSystem(): Promise<void> {
    if (!file) return;
    notice = '';
    try {
      await api.openAttachment(file.id);
    } catch (err) {
      notice = err instanceof Error ? err.message : "L'ouverture a échoué.";
    }
  }

  async function saveAs(): Promise<void> {
    if (!file) return;
    notice = '';
    try {
      await api.saveAttachment(file.id);
    } catch (err) {
      notice = err instanceof Error ? err.message : "L'enregistrement a échoué.";
    }
  }

  function goToNote(): void {
    const id = file?.noteId;
    closeViewer();
    if (id) void selectNote(id);
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.focus();
    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeViewer();
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        stepViewer(1);
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault();
        stepViewer(-1);
      } else if ((e.key === '+' || e.key === '=') && (kind === 'image' || kind === 'pdf')) {
        e.preventDefault();
        zoomBy(1.25);
      } else if (e.key === '-' && (kind === 'image' || kind === 'pdf')) {
        e.preventDefault();
        zoomBy(0.8);
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      observer?.disconnect();
      restoreTo?.focus();
    };
  });
</script>

{#if file}
  <div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && closeViewer()}>
    <div class="viewer" bind:this={dialogEl} role="dialog" aria-modal="true" aria-label={`Lecture de ${file.filename}`} tabindex="-1">
      <header>
        <div class="title">
          <Icon name={kind === 'image' ? 'image' : kind === 'archive' ? 'file-archive' : kind === 'pdf' || kind === 'markdown' || kind === 'text' || kind === 'table' ? 'file-text' : kind === 'audio' ? 'music' : kind === 'video' ? 'film' : 'file'} size={17} />
          <div class="names">
            <strong title={file.filename}>{file.filename}</strong>
            <span>{KIND_LABELS[kind]} · {formatBytes(file.size)}{#if pdfPages} · {pdfPages} page{pdfPages > 1 ? 's' : ''}{/if} · dans « {file.noteTitle || 'Sans titre'} »</span>
          </div>
        </div>

        <div class="actions">
          {#if total > 1}
            <button class="icon" aria-label="Fichier précédent" title="Précédent — ←" disabled={position <= 0} onclick={() => stepViewer(-1)}><Icon name="chevron-left" size={16} /></button>
            <span class="count">{position + 1} / {total}</span>
            <button class="icon" aria-label="Fichier suivant" title="Suivant — →" disabled={position >= total - 1} onclick={() => stepViewer(1)}><Icon name="chevron-right" size={16} /></button>
          {/if}
          {#if kind === 'image' || kind === 'pdf'}
            <button class="icon" aria-label="Zoom arrière" title="Zoom arrière — -" onclick={() => zoomBy(0.8)}><Icon name="zoom-out" size={16} /></button>
            <button class="zoom" title="Taille d'origine" onclick={() => (zoom = 1)}>{Math.round(zoom * 100)} %</button>
            <button class="icon" aria-label="Zoom avant" title="Zoom avant — +" onclick={() => zoomBy(1.25)}><Icon name="zoom-in" size={16} /></button>
          {/if}
          <span class="sep"></span>
          <button class="btn" title="Ouvrir avec l'application du système" onclick={() => void openWithSystem()}><Icon name="external-link" size={14} /> Ouvrir</button>
          <button class="btn" title="Enregistrer une copie" onclick={() => void saveAs()}><Icon name="download" size={14} /> Enregistrer</button>
          <button class="btn" title="Aller à la note" onclick={goToNote}><Icon name="notes" size={14} /> Note</button>
          <button class="icon" aria-label="Fermer" title="Fermer — Échap" onclick={closeViewer}><Icon name="x" size={17} /></button>
        </div>
      </header>

      {#if notice}
        <p class="notice" role="alert">{notice}</p>
      {/if}

      <div class="body" class:centered={kind === 'image' || kind === 'audio' || kind === 'video'}>
        <!-- Le conteneur du PDF doit exister pendant le chargement : pdf.js y dessine. -->
        {#if kind === 'pdf'}
          <div class="pdf" bind:this={pdfEl} hidden={loading || !!error}></div>
        {/if}
        {#if loading}
          <p class="state">Chargement…</p>
        {:else if error}
          <p class="state error" role="alert">{error}</p>
        {:else if kind === 'image'}
          <div class="scroll"><img src={blobUrl} alt={file.filename} style:width={zoom === 1 ? undefined : `${zoom * 100}%`} class:fit={zoom === 1} /></div>
        {:else if kind === 'pdf'}
          <!-- affiché ci-dessus -->
        {:else if kind === 'audio'}
          <!-- svelte-ignore a11y_media_has_caption -->
          <audio controls src={blobUrl}></audio>
        {:else if kind === 'video'}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video controls src={blobUrl}></video>
        {:else if kind === 'markdown'}
          <article class="markdown">{@html renderMarkdown(text)}</article>
        {:else if kind === 'table'}
          <div class="scroll">
            <table>
              <tbody>
                {#each rows as row, i (i)}
                  <tr>{#each row as cell, j (j)}{#if i === 0}<th>{cell}</th>{:else}<td>{cell}</td>{/if}{/each}</tr>
                {/each}
              </tbody>
            </table>
            {#if rows.length > MAX_ROWS}<p class="hint">Aperçu limité aux {MAX_ROWS} premières lignes.</p>{/if}
          </div>
        {:else if kind === 'text'}
          <pre class="text">{text}</pre>
          {#if partial}<p class="hint">Fichier volumineux : début affiché seulement. « Ouvrir » le lit en entier.</p>{/if}
        {:else if kind === 'archive'}
          <ul class="archive">
            {#each entries as e (e.name)}
              <li class:dir={e.isDir}>
                <Icon name={e.isDir ? 'folder' : 'file'} size={14} />
                <span class="n">{e.name}</span>
                {#if !e.isDir}<span class="s">{formatBytes(e.size)}</span>{/if}
              </li>
            {/each}
          </ul>
          {#if archiveTruncated}<p class="hint">Liste limitée aux premières entrées.</p>{/if}
          <p class="hint">Le contenu n'est pas extrait : « Enregistrer » copie l'archive, puis ouvrez-la avec votre gestionnaire d'archives.</p>
        {:else}
          <div class="state">
            <Icon name="file" size={34} />
            <p>NoteCore ne sait pas afficher ce type de fichier.</p>
            <p class="hint">Ouvrez-le avec l'application de votre système, ou enregistrez-en une copie.</p>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 160;
    display: grid;
    place-items: center;
    padding: 1rem;
    background: var(--scrim);
    backdrop-filter: blur(4px);
  }
  .viewer {
    display: flex;
    flex-direction: column;
    width: min(70rem, 100%);
    height: min(52rem, 100%);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
    outline: none;
    animation: pop 0.16s var(--ease);
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: scale(0.99);
    }
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    padding: 0.6rem 0.8rem 0.6rem 1rem;
    border-bottom: 1px solid var(--border);
  }
  .title {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    min-width: 0;
    color: var(--accent-text);
  }
  .names {
    display: grid;
    min-width: 0;
  }
  .names strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
    font-size: 0.92rem;
  }
  .names span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-faint);
    font-size: 0.74rem;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-shrink: 0;
  }
  .count {
    color: var(--text-faint);
    font-size: 0.78rem;
    font-variant-numeric: tabular-nums;
  }
  .sep {
    width: 1px;
    height: 18px;
    margin: 0 0.3rem;
    background: var(--border);
  }
  .icon,
  .btn,
  .zoom {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    height: 30px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .icon {
    width: 30px;
  }
  .btn,
  .zoom {
    padding: 0 0.6rem;
  }
  .icon:hover:not(:disabled),
  .btn:hover,
  .zoom:hover {
    background: var(--hover);
    color: var(--text);
  }
  .icon:disabled {
    opacity: 0.35;
    cursor: default;
  }
  .notice {
    margin: 0;
    padding: 0.5rem 1rem;
    background: var(--danger-soft);
    color: var(--danger);
    font-size: 0.82rem;
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    background: var(--app-bg);
  }
  .body.centered {
    display: grid;
    place-items: center;
  }
  .scroll {
    min-height: 100%;
    overflow: auto;
    padding: 1rem;
    display: grid;
    justify-items: center;
    align-content: start;
  }
  .scroll img {
    max-width: none;
    height: auto;
    border-radius: var(--radius-sm);
    background: var(--code-bg);
  }
  .scroll img.fit {
    max-width: 100%;
    max-height: calc(52rem - 6rem);
    object-fit: contain;
  }
  .pdf[hidden] {
    display: none;
  }
  .pdf {
    height: 100%;
    overflow: auto;
    padding: 1rem;
    display: grid;
    justify-items: center;
    align-content: start;
    gap: 0.8rem;
  }
  .pdf :global(.pdf-page) {
    background: white;
    box-shadow: var(--shadow-md);
    border-radius: 2px;
    overflow: hidden;
  }
  audio {
    width: min(32rem, 90%);
  }
  video {
    max-width: 100%;
    max-height: 100%;
  }
  .state {
    display: grid;
    justify-items: center;
    gap: 0.5rem;
    padding: 3rem 1rem;
    color: var(--text-soft);
    text-align: center;
  }
  .state.error {
    color: var(--danger);
  }
  .state p {
    margin: 0;
  }
  .hint {
    margin: 0.8rem 1.2rem;
    color: var(--text-faint);
    font-size: 0.8rem;
  }
  .text {
    margin: 0;
    padding: 1.1rem 1.4rem;
    font-family: var(--font-mono);
    font-size: 0.84rem;
    line-height: 1.55;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    color: var(--text);
    tab-size: 2;
  }
  .markdown {
    max-width: 48rem;
    margin: 0 auto;
    padding: 1.5rem 1.8rem;
    font-family: var(--editor-font);
    line-height: 1.7;
  }
  .markdown :global(pre) {
    padding: 0.8em 1em;
    overflow-x: auto;
    border-radius: var(--radius);
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.85em;
  }
  .markdown :global(img) {
    max-width: 100%;
  }
  table {
    border-collapse: collapse;
    font-size: 0.84rem;
  }
  th,
  td {
    max-width: 22rem;
    padding: 0.35rem 0.7rem;
    border: 1px solid var(--border);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
  }
  th {
    position: sticky;
    top: 0;
    background: var(--code-bg);
    font-weight: 650;
  }
  .archive {
    margin: 0;
    padding: 0.6rem 0.8rem;
    list-style: none;
    font-size: 0.86rem;
  }
  .archive li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.25rem 0.4rem;
    border-radius: var(--radius-sm);
  }
  .archive li:hover {
    background: var(--hover);
  }
  .archive .n {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: 0.8rem;
  }
  .archive .s {
    color: var(--text-faint);
    font-size: 0.76rem;
  }
  .archive li.dir {
    color: var(--text-soft);
  }
  @media (max-width: 860px) {
    .btn {
      padding: 0 0.4rem;
    }
  }
</style>
