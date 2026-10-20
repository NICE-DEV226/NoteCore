<script lang="ts">
  /**
   * Vue « Fichiers » : toutes les pièces jointes de toutes les notes, à parcourir
   * sans ouvrir chaque note. Filtre par type et par nom, vignettes des images,
   * lecture dans la visionneuse, retour à la note d'origine.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { api } from '../api/client';
  import {
    attachmentIndex,
    attachmentIndexError,
    formatBytes,
    KIND_LABELS,
    kindOf,
    openAttachmentViewer,
    refreshAttachmentIndex,
    type FileKind
  } from '../stores/files';
  import { folders, selectNote } from '../stores/workspace';
  import { importOpen } from '../stores/ui';
  import { formatRelative } from '../utils/format';
  import type { AttachmentRef } from '../api/types';

  let query = $state('');
  let kindFilter = $state<FileKind | 'all'>('all');
  let loaded = $state(false);
  let thumbs = $state<Record<string, string>>({});

  onMount(() => {
    void refreshAttachmentIndex().finally(() => (loaded = true));
    return () => {
      for (const url of Object.values(thumbs)) URL.revokeObjectURL(url);
    };
  });

  const items = $derived(
    $attachmentIndex.map((a) => ({ ...a, kind: kindOf(a) }))
  );

  /** Types réellement présents, avec leur nombre, pour ne proposer que des filtres utiles. */
  const kinds = $derived.by(() => {
    const counts = new Map<FileKind, number>();
    for (const i of items) counts.set(i.kind, (counts.get(i.kind) ?? 0) + 1);
    return [...counts.entries()].sort((a, b) => b[1] - a[1]);
  });

  const shown = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return items.filter(
      (i) =>
        (kindFilter === 'all' || i.kind === kindFilter) &&
        (!q || i.filename.toLowerCase().includes(q) || i.noteTitle.toLowerCase().includes(q))
    );
  });

  const totalSize = $derived(items.reduce((sum, i) => sum + i.size, 0));
  const folderName = (id?: string | null): string => $folders.find((f) => f.id === id)?.name ?? '';

  const ICONS: Record<FileKind, string> = {
    image: 'image', pdf: 'file-text', markdown: 'file-text', text: 'file-text', table: 'file-text',
    archive: 'file-archive', audio: 'music', video: 'film', other: 'file'
  };

  // Vignettes : chargées quand la carte approche de l'écran, pas toutes d'un coup.
  function thumbnail(node: HTMLElement, att: AttachmentRef) {
    if (kindOf(att) !== 'image' || att.size > 4 * 1024 * 1024) return {};
    const io = new IntersectionObserver((entries) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      io.disconnect();
      void api.readAttachment(att.id).then((d) => {
        if (!d.mime.startsWith('image/')) return;
        const bin = atob(d.dataBase64);
        const bytes = new Uint8Array(new ArrayBuffer(bin.length));
        for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
        thumbs[att.id] = URL.createObjectURL(new Blob([bytes], { type: d.mime }));
      }).catch(() => undefined);
    }, { rootMargin: '200px' });
    io.observe(node);
    return { destroy: () => io.disconnect() };
  }
</script>

<div class="files">
  <div class="inner">
    <header>
      <div>
        <h1>Fichiers</h1>
        <p class="sub">
          {items.length} fichier{items.length > 1 ? 's' : ''} · {formatBytes(totalSize)} · joints à vos notes, stockés dans votre base.
        </p>
      </div>
      <button class="ghost" onclick={() => importOpen.set(true)} title="Importer des notes avec leurs fichiers">
        <Icon name="upload" size={14} /> Importer
      </button>
    </header>

    <div class="tools">
      <div class="search">
        <Icon name="search" size={14} />
        <input bind:value={query} placeholder="Filtrer par nom de fichier ou de note…" aria-label="Filtrer les fichiers" />
        {#if query}<button class="clear" aria-label="Effacer" onclick={() => (query = '')}><Icon name="x" size={13} /></button>{/if}
      </div>
      <div class="chips" role="group" aria-label="Type de fichier">
        <button class:on={kindFilter === 'all'} aria-pressed={kindFilter === 'all'} onclick={() => (kindFilter = 'all')}>Tous</button>
        {#each kinds as [k, n] (k)}
          <button class:on={kindFilter === k} aria-pressed={kindFilter === k} onclick={() => (kindFilter = k)}>{KIND_LABELS[k]} <small>{n}</small></button>
        {/each}
      </div>
    </div>

    {#if $attachmentIndexError}
      <p class="error" role="alert">{$attachmentIndexError}</p>
    {:else if !loaded}
      <p class="empty">Chargement…</p>
    {:else if items.length === 0}
      <div class="empty big">
        <Icon name="paperclip" size={30} />
        <h2>Aucun fichier joint</h2>
        <p>Dans une note, le bouton <strong>Image</strong> ou <strong>Joindre</strong> de la barre d'outils ajoute un fichier. Il apparaîtra ici, lisible sans quitter NoteCore.</p>
      </div>
    {:else if shown.length === 0}
      <p class="empty">Aucun fichier ne correspond à ce filtre.</p>
    {:else}
      <ul class="grid">
        {#each shown as f (f.id)}
          <li>
            <button class="card" onclick={() => void openAttachmentViewer(f.id, shown)} title={`Lire ${f.filename}`}>
              <span class="thumb" use:thumbnail={f}>
                {#if thumbs[f.id]}
                  <img src={thumbs[f.id]} alt="" />
                {:else}
                  <Icon name={ICONS[f.kind]} size={30} stroke={1.5} />
                  <span class="ext">{f.filename.includes('.') ? f.filename.split('.').pop()?.slice(0, 5) : ''}</span>
                {/if}
              </span>
              <span class="name" title={f.filename}>{f.filename}</span>
              <span class="meta">{KIND_LABELS[f.kind]} · {formatBytes(f.size)}</span>
            </button>
            <button class="note" onclick={() => void selectNote(f.noteId)} title="Aller à la note">
              <Icon name="notes" size={12} />
              <span>{f.noteTitle || 'Sans titre'}</span>
              {#if folderName(f.folderId)}<small>· {folderName(f.folderId)}</small>{/if}
              <small class="when">{formatRelative(f.createdAt)}</small>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .files {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    background: var(--app-bg);
  }
  .inner {
    max-width: 64rem;
    margin: 0 auto;
    padding: 2.4rem 2rem 3rem;
  }
  header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }
  h1 {
    margin: 0;
    font-size: 1.9rem;
    font-weight: 750;
    letter-spacing: -0.03em;
  }
  .sub {
    margin: 0.3rem 0 0;
    color: var(--text-soft);
    font-size: 0.88rem;
  }
  .ghost {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    height: 32px;
    padding: 0 0.8rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.82rem;
    cursor: pointer;
  }
  .ghost:hover {
    border-color: var(--accent);
  }
  .tools {
    display: grid;
    gap: 0.7rem;
    margin: 1.4rem 0 1.2rem;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    height: 36px;
    padding: 0 0.7rem;
    border: 1px solid transparent;
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text-faint);
  }
  .search:focus-within {
    border-color: var(--accent);
    background: var(--surface);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.88rem;
  }
  .clear {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .chips button {
    padding: 0.25rem 0.75rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .chips button small {
    margin-left: 0.2rem;
    color: var(--text-faint);
  }
  .chips button:hover {
    border-color: var(--text-faint);
  }
  .chips button.on {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 600;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(11.5rem, 1fr));
    gap: 0.9rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  li {
    min-width: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr); /* sans cela, un nom long élargit la colonne */
    gap: 0.3rem;
    align-content: start;
  }
  .card {
    min-width: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 0.3rem;
    padding: 0.5rem;
    border: 1px solid var(--card-border);
    border-radius: var(--radius-lg);
    background: var(--card-bg);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.12s var(--ease),
      box-shadow 0.12s var(--ease),
      transform 0.12s var(--ease);
  }
  .card:hover {
    border-color: var(--accent);
    box-shadow: var(--shadow-md);
    transform: translateY(-1px);
  }
  .thumb {
    position: relative;
    display: grid;
    place-items: center;
    aspect-ratio: 4 / 3;
    overflow: hidden;
    border-radius: calc(var(--radius-lg) - 4px);
    background: var(--code-bg);
    color: var(--text-faint);
  }
  .thumb img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .ext {
    position: absolute;
    right: 0.5rem;
    bottom: 0.4rem;
    color: var(--text-faint);
    font-size: 0.68rem;
    font-weight: 650;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding: 0 0.3rem;
    font-size: 0.86rem;
    font-weight: 600;
  }
  .meta {
    padding: 0 0.3rem 0.2rem;
    color: var(--text-faint);
    font-size: 0.74rem;
  }
  .note {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.1rem 0.4rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.74rem;
    text-align: left;
    cursor: pointer;
  }
  .note span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .note small {
    color: var(--text-faint);
    white-space: nowrap;
  }
  .note .when {
    margin-left: auto;
  }
  .note:hover {
    color: var(--accent-text);
  }
  .empty {
    margin: 2rem 0;
    color: var(--text-soft);
    text-align: center;
  }
  .empty.big {
    display: grid;
    justify-items: center;
    gap: 0.5rem;
    padding: 3rem 1rem;
  }
  .empty h2 {
    margin: 0.4rem 0 0;
    color: var(--text);
    font-size: 1.15rem;
  }
  .empty p {
    max-width: 28rem;
    margin: 0;
    line-height: 1.55;
  }
  .error {
    padding: 0.6rem 0.9rem;
    border-radius: var(--radius-sm);
    background: var(--danger-soft);
    color: var(--danger);
  }
</style>
