<script lang="ts">
  /**
   * Vue « Corbeille » : notes mises de côté, jamais détruites sans
   * action explicite. Chaque note peut être restaurée (avec ses pièces
   * jointes et ses cartes de révision, telles qu'elles étaient) ou
   * détruite définitivement ; « Vider » détruit tout le contenu.
   * Les notes à la corbeille depuis plus de 30 jours sont purgées au
   * démarrage par le backend.
   */
  import Icon from './Icon.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import {
    deleteTrashedForever,
    emptyTrashAll,
    restoreTrashedNote,
    trashedNotes
  } from '../stores/workspace';
  import { formatRelative } from '../utils/format';
  import type { Note } from '../api/types';

  let pendingForever = $state<Note | null>(null);
  let pendingEmpty = $state(false);
  let busy = $state(false);

  async function restore(n: Note): Promise<void> {
    busy = true;
    try {
      await restoreTrashedNote(n.id);
    } finally {
      busy = false;
    }
  }

  async function destroy(n: Note): Promise<void> {
    pendingForever = null;
    busy = true;
    try {
      await deleteTrashedForever(n.id);
    } finally {
      busy = false;
    }
  }

  async function empty(): Promise<void> {
    pendingEmpty = false;
    busy = true;
    try {
      await emptyTrashAll();
    } finally {
      busy = false;
    }
  }
</script>

<div class="trash">
  <div class="inner">
    <header>
      <div>
        <h1>Corbeille</h1>
        <p class="sub">
          {$trashedNotes.length} note{$trashedNotes.length > 1 ? 's' : ''} mise{$trashedNotes.length > 1 ? 's' : ''} de côté.
          Rien n'est détruit sans action explicite ; une note y reste 30 jours.
        </p>
      </div>
      {#if $trashedNotes.length > 0}
        <button class="ghost danger" disabled={busy} onclick={() => (pendingEmpty = true)}>
          <Icon name="trash" size={14} /> Vider la corbeille
        </button>
      {/if}
    </header>

    {#if $trashedNotes.length === 0}
      <div class="empty big">
        <Icon name="trash" size={30} />
        <h2>La corbeille est vide</h2>
        <p>Une note supprimée arrive ici : elle disparaît de vos listes et de la recherche, mais tout est conservé jusqu'à sa restauration ou sa destruction définitive.</p>
      </div>
    {:else}
      <ul class="list">
        {#each $trashedNotes as n (n.id)}
          <li>
            <div class="row-main">
              <span class="title">{n.title || 'Sans titre'}</span>
              <span class="meta">
                mise de côté {n.deletedAt ? formatRelative(n.deletedAt) : '—'}
                {#if n.tags?.length}· {n.tags.join(', ')}{/if}
              </span>
            </div>
            <div class="row-actions">
              <button class="act" disabled={busy} onclick={() => void restore(n)} title="Restaurer cette note avec tout ce qu'elle contient">
                <Icon name="undo" size={13} /> Restaurer
              </button>
              <button class="act danger" disabled={busy} onclick={() => (pendingForever = n)} title="Détruire définitivement cette note">
                <Icon name="trash" size={13} /> Supprimer
              </button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

{#if pendingForever}
  <ConfirmDialog
    title="Détruire définitivement cette note ?"
    message={`« ${pendingForever.title || 'Sans titre'} » et tout ce qu'elle contient (pièces jointes, cartes de révision) seront détruits. Cette action est irréversible.`}
    confirmLabel="Détruire"
    onconfirm={() => void destroy(pendingForever as Note)}
    oncancel={() => (pendingForever = null)}
  />
{/if}

{#if pendingEmpty}
  <ConfirmDialog
    title="Vider la corbeille ?"
    message={`Les {$trashedNotes.length} note{$trashedNotes.length > 1 ? 's' : ''} à la corbeille seront détruites avec leurs pièces jointes et leurs cartes de révision. Cette action est irréversible.`}
    confirmLabel="Tout détruire"
    onconfirm={() => void empty()}
    oncancel={() => (pendingEmpty = false)}
  />
{/if}

<style>
  .trash {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    background: var(--app-bg);
  }
  .inner {
    max-width: 46rem;
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
    max-width: 34rem;
    line-height: 1.5;
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
    white-space: nowrap;
  }
  .ghost:hover {
    border-color: var(--accent);
  }
  .ghost.danger {
    color: var(--danger);
    border-color: var(--danger-soft);
  }
  .ghost.danger:hover {
    border-color: var(--danger);
  }
  .list {
    margin: 1.4rem 0 0;
    padding: 0;
    list-style: none;
    border-top: 1px solid var(--border);
  }
  li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.75rem 0.4rem;
    border-bottom: 1px solid var(--border);
  }
  .row-main {
    min-width: 0;
    display: grid;
    gap: 0.15rem;
  }
  .title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.92rem;
    font-weight: 600;
  }
  .meta {
    color: var(--text-faint);
    font-size: 0.76rem;
  }
  .row-actions {
    display: flex;
    gap: 0.4rem;
    flex-shrink: 0;
  }
  .act {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.3rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text-soft);
    font: inherit;
    font-size: 0.78rem;
    cursor: pointer;
  }
  .act:hover {
    border-color: var(--accent);
    color: var(--accent-text);
  }
  .act.danger {
    color: var(--danger);
    border-color: var(--danger-soft);
  }
  .act.danger:hover {
    border-color: var(--danger);
  }
  .act:disabled {
    opacity: 0.5;
    cursor: default;
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
    color: var(--text-faint);
  }
  .empty h2 {
    margin: 0.4rem 0 0;
    color: var(--text);
    font-size: 1.15rem;
  }
  .empty p {
    max-width: 30rem;
    margin: 0;
    line-height: 1.55;
    color: var(--text-soft);
  }
</style>
