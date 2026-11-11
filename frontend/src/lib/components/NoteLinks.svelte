<script lang="ts">
  /**
   * Liens d'une note : les `[[liens]]` qu'elle contient (sortants) et les
   * notes qui pointent vers elle (entrants). Calculés sur le store local,
   * sans requête : un clic ouvre la note visée.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { notes, selectNote } from '../stores/workspace';
  import { backlinksFor, outgoingLinks } from '../wiki';
  import type { Note } from '../api/types';

  let { note, onclose }: { note: Note; onclose: () => void } = $props();

  let dialogEl = $state<HTMLDivElement | null>(null);
  const uid = `nl-${Math.random().toString(36).slice(2, 8)}`;

  const outgoing = $derived(outgoingLinks(note, $notes));
  const incoming = $derived(backlinksFor(note, $notes));

  async function open(id: string): Promise<void> {
    await selectNote(id);
    onclose();
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.focus();
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
      <h2 id="{uid}-title"><Icon name="link" size={16} /> Liens de la note</h2>
      <button class="x" aria-label="Fermer" onclick={onclose}><Icon name="x" size={15} /></button>
    </header>

    <div class="body">
      <p class="hint">
        Écrivez <code>[[titre de la note]]</code> (ou <code>[[titre|alias]]</code>) dans le texte pour créer un lien.
        Les liens cliquables apparaissent dans l'aperçu.
      </p>

      <section>
        <h3>Liens sortants <span class="count">{outgoing.length}</span></h3>
        {#if outgoing.length === 0}
          <p class="muted">Aucun lien <code>[[…]]</code> dans cette note.</p>
        {:else}
          <ul class="list">
            {#each outgoing as o (o.note.id)}
              <li>
                <button class="item" onclick={() => void open(o.note.id)}>
                  <span class="what">
                    <strong>{o.note.title || 'Sans titre'}</strong>
                    {#if o.link.label !== o.link.target}<small>« {o.link.label} »</small>{/if}
                  </span>
                  <Icon name="chevron-right" size={13} />
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section>
        <h3>Liens entrants <span class="count">{incoming.length}</span></h3>
        {#if incoming.length === 0}
          <p class="muted">Aucune autre note ne pointe vers celle-ci.</p>
        {:else}
          <ul class="list">
            {#each incoming as n (n.id)}
              <li>
                <button class="item" onclick={() => void open(n.id)}>
                  <span class="what"><strong>{n.title || 'Sans titre'}</strong></span>
                  <Icon name="chevron-right" size={13} />
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
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
    width: min(34rem, calc(100% - 2rem));
    max-height: min(80vh, 40rem);
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
    gap: 0.9rem;
  }
  .hint {
    margin: 0;
    font-size: 0.85rem;
    color: var(--text-soft);
  }
  .hint code {
    font-family: var(--font-mono);
    font-size: 0.85em;
    background: var(--code-bg);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0 0.3em;
  }
  section h3 {
    margin: 0 0 0.45rem;
    font-size: 0.78rem;
    font-weight: 650;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  .count {
    font-weight: 500;
    background: var(--chip-bg);
    border-radius: 999px;
    padding: 0 0.5em;
  }
  .muted { color: var(--text-soft); font-size: 0.85rem; margin: 0; }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .item {
    appearance: none;
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.45rem 0.6rem;
    border: 1px solid transparent;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .item:hover { background: var(--hover); border-color: var(--border); }
  .what {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .what strong {
    font-size: 0.88rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .what small {
    color: var(--text-faint);
    font-size: 0.76rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .item :global(svg) { color: var(--text-faint); flex-shrink: 0; }
</style>
