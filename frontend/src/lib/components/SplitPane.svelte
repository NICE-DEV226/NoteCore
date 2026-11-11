<script lang="ts">
  /**
   * Volet secondaire du split-screen : une note affichée en lecture seule,
   * à côté de l'éditeur. Le contenu suit le store — une note modifiée dans
   * l'éditeur principal (ou une autre fenêtre de l'app) reste à jour ici.
   * Un clic sur un lien [[…]] ouvre la cible dans l'éditeur principal.
   */
  import Icon from './Icon.svelte';
  import MarkdownPreview from '../editor/MarkdownPreview.svelte';
  import EmptyState from './EmptyState.svelte';
  import { notes, selectNote, splitNote, splitNoteId } from '../stores/workspace';
  import { formatRelative } from '../utils/format';

  const note = $derived($splitNote);

  function close(): void {
    splitNoteId.set(null);
  }
</script>

<aside class="split-pane" aria-label="Volet secondaire">
  <header>
    <div class="pick">
      <Icon name="columns" size={13} />
      <select
        value={note?.id ?? ''}
        aria-label="Note affichée dans le volet secondaire"
        onchange={(e) => splitNoteId.set((e.target as HTMLSelectElement).value || null)}
      >
        <option value="">Choisir une note…</option>
        {#each $notes as n (n.id)}
          <option value={n.id}>{n.title || 'Sans titre'}</option>
        {/each}
      </select>
    </div>
    {#if note}
      <span class="meta">modifiée {formatRelative(note.updatedAt)}</span>
    {/if}
    <button class="x" title="Fermer le volet" aria-label="Fermer le volet secondaire" onclick={close}>
      <Icon name="x" size={14} />
    </button>
  </header>

  {#if note}
    <div class="body">
      <MarkdownPreview content={note.content} onWikiLink={(id) => void selectNote(id)} />
    </div>
  {:else}
    <EmptyState
      icon="notes"
      title="Un second regard"
      message="Choisissez une note : elle reste affichée ici en lecture pendant que vous écrivez dans l'autre volet."
    />
  {/if}
</aside>

<style>
  .split-pane {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    border-left: 1px solid var(--border);
    background: var(--app-bg);
  }
  header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.45rem 0.6rem;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    flex-shrink: 0;
  }
  .pick {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--text-faint);
    min-width: 0;
  }
  .pick select {
    font: inherit;
    font-size: 0.82rem;
    color: var(--text);
    background: var(--input-bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.2rem 0.4rem;
    max-width: 16rem;
  }
  .meta {
    font-size: 0.74rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .x {
    margin-left: auto;
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
    padding: 0.25rem;
    border-radius: var(--radius);
    display: grid;
    place-items: center;
    flex-shrink: 0;
  }
  .x:hover { background: var(--hover); color: var(--text); }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 1.2rem 1.4rem;
  }
</style>
