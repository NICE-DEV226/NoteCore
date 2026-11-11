<script lang="ts">
  /**
   * Historique des versions d'une note : chaque état sauvegardé est
   * listé (date, titre, taille) ; cliquer prévisualise le contenu,
   * « Restaurer » rétablit cet état — réversible, car l'état courant
   * devient lui-même une version.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import MarkdownPreview from '../editor/MarkdownPreview.svelte';
  import { api } from '../api/client';
  import { formatRelative } from '../utils/format';
  import type { Note, NoteVersion, NoteVersionInfo } from '../api/types';

  let {
    noteId,
    onclose,
    onrestored
  }: {
    noteId: string;
    onclose: () => void;
    onrestored: (note: Note) => void;
  } = $props();

  let versions = $state<NoteVersionInfo[]>([]);
  let selected = $state<NoteVersion | null>(null);
  let loading = $state(true);
  let busy = $state(false);
  let error = $state('');
  let dialogEl = $state<HTMLDivElement | null>(null);

  const uid = `vh-${Math.random().toString(36).slice(2, 8)}`;

  async function pick(v: NoteVersionInfo): Promise<void> {
    if (selected?.id === v.id) {
      selected = null;
      return;
    }
    try {
      selected = await api.getNoteVersion(v.id);
    } catch {
      selected = null;
    }
  }

  async function restore(): Promise<void> {
    if (!selected) return;
    busy = true;
    error = '';
    try {
      const note = await api.restoreNoteVersion(noteId, selected.id);
      onrestored(note);
      onclose();
    } catch (e) {
      error = e instanceof Error ? e.message : 'La restauration a échoué.';
    } finally {
      busy = false;
    }
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.focus();
    void api.listNoteVersions(noteId)
      .then((v) => (versions = v))
      .catch((e) => (error = e instanceof Error ? e.message : 'Lecture impossible.'))
      .finally(() => (loading = false));
    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.preventDefault();
        onclose();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

<div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div
    class="dialog"
    bind:this={dialogEl}
    role="dialog"
    aria-modal="true"
    aria-labelledby="{uid}-title"
    tabindex="-1"
  >
    <header>
      <h2 id="{uid}-title"><Icon name="clock" size={16} /> Historique de la note</h2>
      <button class="x" aria-label="Fermer" onclick={onclose}><Icon name="x" size={15} /></button>
    </header>

    {#if loading}
      <p class="muted">Chargement…</p>
    {:else if versions.length === 0}
      <p class="muted">Aucune version pour l'instant : chaque modification enregistrée de cette note sera conservée ici.</p>
    {:else}
      <div class="body">
        <ul class="list">
          {#each versions as v (v.id)}
            <li>
              <button
                class="item"
                class:on={selected?.id === v.id}
                aria-pressed={selected?.id === v.id}
                onclick={() => void pick(v)}
              >
                <span class="when">{formatRelative(v.createdAt)}</span>
                <span class="what">
                  <strong>{v.title || 'Sans titre'}</strong>
                  <small>{v.chars.toLocaleString('fr-FR')} caractères</small>
                </span>
              </button>
            </li>
          {/each}
        </ul>
        {#if selected}
          <div class="preview">
            <div class="preview-head">
              <span class="muted">{formatRelative(selected.createdAt)} — aperçu</span>
              <button class="btn primary" disabled={busy} onclick={() => void restore()}>
                <Icon name="undo" size={14} /> Restaurer cette version
              </button>
            </div>
            <div class="preview-body"><MarkdownPreview content={selected.content} /></div>
          </div>
        {/if}
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        <p class="muted small">Restaurer est sans risque : la version actuelle est conservée dans l'historique.</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--scrim);
    backdrop-filter: blur(3px);
    display: grid;
    place-items: center;
    z-index: 100;
  }
  .dialog {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    width: min(46rem, calc(100% - 2rem));
    max-height: min(80vh, 44rem);
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow-lg);
    overflow: hidden;
  }
  .dialog:focus { outline: none; }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1.1rem;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    margin: 0;
    font-size: 0.98rem;
    display: flex;
    align-items: center;
    gap: 0.45rem;
  }
  .x {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: var(--radius);
    display: grid;
    place-items: center;
  }
  .x:hover { background: var(--hover); color: var(--text); }
  .body {
    padding: 0.9rem 1.1rem 1.1rem;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }
  .muted { color: var(--text-soft); font-size: 0.88rem; margin: 0; }
  .small { font-size: 0.8rem; }
  .error { color: var(--danger); font-size: 0.85rem; margin: 0; }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    max-height: 12rem;
    overflow-y: auto;
  }
  .item {
    appearance: none;
    border: 1px solid transparent;
    background: transparent;
    width: 100%;
    text-align: left;
    font: inherit;
    cursor: pointer;
    display: flex;
    align-items: baseline;
    gap: 0.8rem;
    padding: 0.45rem 0.6rem;
    border-radius: var(--radius);
  }
  .item:hover { background: var(--hover); }
  .item.on { background: var(--accent-soft); border-color: var(--accent); }
  .when {
    flex: none;
    width: 7.5rem;
    font-size: 0.78rem;
    color: var(--text-soft);
  }
  .what { display: flex; flex-direction: column; min-width: 0; }
  .what strong {
    font-size: 0.88rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .what small { font-size: 0.76rem; color: var(--text-soft); }
  .preview {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    overflow: hidden;
    display: flex;
    flex-direction: column;
    max-height: 22rem;
  }
  .preview-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.5rem 0.7rem;
    border-bottom: 1px solid var(--border);
    background: var(--surface-2, var(--hover));
  }
  .preview-body {
    overflow-y: auto;
    padding: 0.8rem 1rem;
    font-size: 0.9rem;
  }
  .btn {
    appearance: none;
    border: none;
    font: inherit;
    font-size: 0.84rem;
    padding: 0.38rem 0.8rem;
    border-radius: var(--radius);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }
  .btn.primary { background: var(--accent); color: var(--on-accent); }
  .btn.primary:hover { filter: brightness(1.08); }
  .btn:disabled { opacity: 0.55; cursor: default; }
</style>
