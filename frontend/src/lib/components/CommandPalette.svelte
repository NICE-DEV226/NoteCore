<script lang="ts">
  /**
   * Palette de commandes (Ctrl/Cmd+K) : un seul endroit pour retrouver une
   * note (moteur FTS5 du backend, extraits surlignés) et lancer une action.
   * Les résultats sont annoncés au lecteur d'écran ; flèches, Entrée, Échap.
   */
  import { onMount, tick } from 'svelte';
  import Icon from './Icon.svelte';
  import { api } from '../api/client';
  import type { SearchHit } from '../api/types';
  import { focusMode, paletteOpen, settingsOpen } from '../stores/ui';
  import { theme, setThemePref } from '../stores/appTheme';
  import {
    createNote,
    currentView,
    exportAllNotes,
    folders,
    goHome,
    notes,
    selectNote
  } from '../stores/workspace';
  import { snippetParts } from '../utils/format';
  import { startReview } from '../stores/review';
  import { openGuide } from '../stores/guide';

  type Item = {
    id: string;
    kind: 'note' | 'action' | 'folder';
    icon: string;
    label: string;
    hint?: string;
    hit?: SearchHit;
    run: () => void;
  };

  let query = $state('');
  let hits = $state<SearchHit[] | null>(null);
  let active = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);
  let listEl = $state<HTMLUListElement | null>(null);
  let dialogEl = $state<HTMLDivElement | null>(null);
  let seq = 0;

  function close(): void {
    paletteOpen.set(false);
  }

  function pick(run: () => void): void {
    close();
    run();
  }

  const actions: Item[] = [
    { id: 'new', kind: 'action', icon: 'plus', label: 'Nouvelle note', hint: 'Ctrl N', run: () => void createNote() },
    { id: 'review', kind: 'action', icon: 'layers', label: 'Lancer une révision', run: () => void startReview() },
    { id: 'home', kind: 'action', icon: 'home', label: "Aller à l'accueil", run: () => void goHome() },
    {
      id: 'focus',
      kind: 'action',
      icon: 'focus',
      label: 'Basculer le mode concentration',
      hint: 'Ctrl \\',
      run: () => focusMode.update((v) => !v)
    },
    {
      id: 'theme',
      kind: 'action',
      icon: 'moon',
      label: 'Basculer le thème clair / sombre',
      run: () => setThemePref($theme.theme === 'dark' ? 'light' : 'dark')
    },
    { id: 'settings', kind: 'action', icon: 'settings', label: 'Ouvrir les paramètres', hint: 'Ctrl ,', run: () => settingsOpen.set(true) },
    { id: 'guide', kind: 'action', icon: 'notes', label: 'Ouvrir le guide de prise en main', run: () => openGuide() },
    { id: 'export', kind: 'action', icon: 'download', label: 'Exporter toutes les notes', run: () => void exportAllNotes() }
  ];

  const norm = (s: string): string =>
    s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase();

  const items = $derived.by<Item[]>(() => {
    const q = norm(query.trim());
    const out: Item[] = [];

    if (!q) {
      const recent = [...$notes].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 6);
      for (const n of recent) {
        out.push({ id: `n-${n.id}`, kind: 'note', icon: 'notes', label: n.title || 'Sans titre', run: () => void selectNote(n.id) });
      }
      return [...out, ...actions];
    }

    const byId = new Map($notes.map((n) => [n.id, n]));
    for (const h of hits ?? []) {
      const n = byId.get(h.noteId);
      if (!n) continue;
      out.push({
        id: `n-${h.noteId}`,
        kind: 'note',
        icon: 'notes',
        label: n.title || 'Sans titre',
        hit: h,
        run: () => void selectNote(h.noteId)
      });
    }
    for (const f of $folders) {
      if (norm(f.name).includes(q)) {
        out.push({
          id: `f-${f.id}`,
          kind: 'folder',
          icon: 'folder',
          label: `Dossier : ${f.name}`,
          run: () => {
            currentView.set({ kind: 'folder', folderId: f.id });
            void goHome();
          }
        });
      }
    }
    for (const a of actions) if (norm(a.label).includes(q)) out.push(a);
    return out;
  });

  // Recherche backend, débouncée et protégée des réponses en retard.
  $effect(() => {
    const q = query.trim();
    active = 0;
    if (!q) {
      hits = null;
      return;
    }
    const mine = ++seq;
    const timer = setTimeout(async () => {
      try {
        const res = await api.searchNotes(q);
        if (mine === seq) hits = res;
      } catch {
        if (mine === seq) hits = [];
      }
    }, 120);
    return () => clearTimeout(timer);
  });

  function move(delta: number): void {
    if (items.length === 0) return;
    active = (active + delta + items.length) % items.length;
    void tick().then(() => listEl?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }));
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault();
      close();
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      move(1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      move(-1);
    } else if (event.key === 'Enter') {
      event.preventDefault();
      const item = items[active];
      if (item) pick(item.run);
    } else if (event.key === 'k' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      close();
    } else if (event.key === 'Tab') {
      // Le focus reste dans le champ : la navigation se fait aux flèches.
      event.preventDefault();
    }
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    inputEl?.focus();
    return () => restoreTo?.focus();
  });
</script>

<div
  class="overlay"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) close();
  }}
>
  <div class="palette" bind:this={dialogEl} role="dialog" aria-modal="true" aria-label="Palette de commandes">
    <div class="field">
      <Icon name="search" size={17} />
      <input
        bind:this={inputEl}
        bind:value={query}
        onkeydown={onKeydown}
        placeholder="Rechercher une note ou une commande…"
        role="combobox"
        aria-expanded="true"
        aria-controls="palette-list"
        aria-activedescendant={items[active] ? `pal-${items[active].id}` : undefined}
        aria-label="Rechercher une note ou une commande"
        autocomplete="off"
        spellcheck="false"
      />
      <kbd>Échap</kbd>
    </div>

    <ul id="palette-list" role="listbox" bind:this={listEl}>
      {#each items as item, i (item.id)}
        <!-- Combobox : le clavier est géré par le champ (flèches, Entrée),
             les options ne reçoivent jamais le focus. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li
          id={`pal-${item.id}`}
          role="option"
          aria-selected={i === active}
          class:active={i === active}
          onmousemove={() => (active = i)}
          onclick={() => pick(item.run)}
        >
          <span class="ic"><Icon name={item.icon} size={16} /></span>
          <span class="body">
            <span class="label">{item.label}</span>
            {#if item.hit}
              <span class="sub">
                {#each snippetParts(item.hit.excerpt, 120) as part}{#if part.hit}<mark>{part.text}</mark
                    >{:else}{part.text}{/if}{/each}
              </span>
            {/if}
          </span>
          {#if item.hint}<kbd>{item.hint}</kbd>{/if}
          {#if i === active}<Icon name="corner-down-left" size={14} />{/if}
        </li>
      {:else}
        <li class="none" role="presentation">Aucun résultat pour « {query.trim()} ».</li>
      {/each}
    </ul>

    <footer>
      <span><kbd>↑</kbd><kbd>↓</kbd> naviguer</span>
      <span><kbd>↵</kbd> ouvrir</span>
    </footer>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 150;
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding-top: 12vh;
    background: var(--scrim);
    backdrop-filter: blur(3px);
  }
  .palette {
    display: flex;
    flex-direction: column;
    width: min(38rem, calc(100% - 2rem));
    max-height: min(68vh, 34rem);
    background: var(--surface-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
    animation: drop 0.14s var(--ease);
  }
  @keyframes drop {
    from {
      opacity: 0;
      transform: translateY(-8px) scale(0.985);
    }
  }
  .field {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.85rem 1rem;
    border-bottom: 1px solid var(--border);
    color: var(--text-faint);
  }
  input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 1rem;
  }
  kbd {
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 5px;
    background: var(--chip-bg);
    font-family: var(--font-ui);
    font-size: 0.68rem;
    color: var(--text-faint);
  }
  ul {
    margin: 0;
    padding: 0.4rem;
    list-style: none;
    overflow-y: auto;
  }
  li {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.55rem 0.65rem;
    border-radius: var(--radius);
    color: var(--text);
    cursor: pointer;
  }
  li.active {
    background: var(--accent-soft);
  }
  li.active .ic,
  li.active > :global(svg) {
    color: var(--accent-text);
  }
  .ic {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border-radius: 8px;
    background: var(--chip-bg);
    color: var(--text-soft);
    flex-shrink: 0;
  }
  li.active .ic {
    background: var(--surface);
  }
  .body {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 1px;
  }
  .label {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.92rem;
    font-weight: 550;
  }
  .sub {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-soft);
    font-size: 0.8rem;
  }
  .sub :global(mark) {
    background: var(--mark-bg);
    color: inherit;
    border-radius: 3px;
    padding: 0 0.1em;
  }
  .none {
    justify-content: center;
    color: var(--text-faint);
    cursor: default;
    padding: 1.4rem;
  }
  footer {
    display: flex;
    gap: 1rem;
    padding: 0.5rem 1rem;
    border-top: 1px solid var(--border);
    color: var(--text-faint);
    font-size: 0.74rem;
  }
  footer span {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
  }
</style>
