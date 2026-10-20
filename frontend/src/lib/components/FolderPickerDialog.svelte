<script lang="ts">
  /**
   * Choisir un dossier de destination dans l'arborescence : déplacer des notes
   * ou un dossier sans glisser-déposer (clavier, lecteur d'écran, trackpad).
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { folderTree, folders } from '../stores/workspace';
  import { folderPicker } from '../stores/ui';
  import { visibleNodes, type FolderNode } from '../utils/folders';

  const cfg = $derived($folderPicker);
  const excluded = $derived(new Set(cfg?.exclude ?? []));
  // L'arbre est toujours entièrement déplié ici : on choisit, on ne navigue pas.
  const all = $derived(visibleNodes($folderTree, new Set($folders.map((f) => f.id))));
  let dialogEl = $state<HTMLDivElement | null>(null);

  function close(): void {
    folderPicker.set(null);
  }
  function pick(node: FolderNode | null): void {
    const c = cfg;
    if (!c) return;
    close();
    c.onpick(node ? node.folder.id : null);
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.querySelector<HTMLElement>('button.choice:not(:disabled)')?.focus();
    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.preventDefault();
        close();
        return;
      }
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        const items = [...(dialogEl?.querySelectorAll<HTMLElement>('button.choice:not(:disabled)') ?? [])];
        const i = items.indexOf(document.activeElement as HTMLElement);
        if (items.length === 0) return;
        e.preventDefault();
        items[(i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length].focus();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

{#if cfg}
  <div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
    <div class="dialog" bind:this={dialogEl} role="dialog" aria-modal="true" aria-labelledby="picker-title" tabindex="-1">
      <header>
        <h2 id="picker-title">{cfg.title}</h2>
        <button class="x" aria-label="Fermer" onclick={close}><Icon name="x" size={16} /></button>
      </header>
      <div class="list" role="listbox" aria-label="Dossiers">
        <button class="choice root" role="option" aria-selected={cfg.current === null} class:on={cfg.current === null} onclick={() => pick(null)}>
          <Icon name="home" size={15} />
          <span>{cfg.rootLabel}</span>
        </button>
        {#each all as node (node.folder.id)}
          {@const blocked = excluded.has(node.folder.id)}
          <button
            class="choice"
            role="option"
            aria-selected={cfg.current === node.folder.id}
            class:on={cfg.current === node.folder.id}
            disabled={blocked}
            title={blocked ? 'Un dossier ne peut pas être placé dans lui-même ni dans un de ses sous-dossiers' : undefined}
            style:padding-left="{0.7 + node.depth * 1.1}rem"
            onclick={() => pick(node)}
          >
            <Icon name="folder" size={15} />
            <span>{node.folder.name}</span>
            <small>{node.total}</small>
          </button>
        {/each}
        {#if all.length === 0}
          <p class="none">Aucun dossier pour l'instant.</p>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 140;
    display: grid;
    place-items: center;
    background: var(--scrim);
    backdrop-filter: blur(3px);
  }
  .dialog {
    display: flex;
    flex-direction: column;
    width: min(26rem, calc(100% - 2rem));
    max-height: min(32rem, calc(100vh - 3rem));
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    outline: none;
    animation: pop 0.14s var(--ease);
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(6px) scale(0.985);
    }
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1rem 0.5rem 1.2rem;
  }
  h2 {
    margin: 0;
    font-size: 1rem;
    letter-spacing: -0.01em;
  }
  .x {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
  }
  .x:hover {
    background: var(--hover);
  }
  .list {
    overflow-y: auto;
    padding: 0.3rem 0.5rem 0.7rem;
  }
  .choice {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    width: 100%;
    padding: 0.4rem 0.7rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.88rem;
    text-align: left;
    cursor: pointer;
  }
  .choice span {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .choice small {
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }
  .choice:hover:not(:disabled),
  .choice:focus-visible {
    background: var(--hover);
  }
  .choice.on {
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 600;
  }
  .choice:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .root {
    margin-bottom: 0.2rem;
    border-bottom: 1px solid var(--border);
    border-radius: var(--radius-sm) var(--radius-sm) 0 0;
  }
  .none {
    margin: 0.6rem;
    color: var(--text-faint);
    font-size: 0.84rem;
  }
</style>
