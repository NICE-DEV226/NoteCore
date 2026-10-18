<script lang="ts">
  /**
   * Sélecteur de police : chaque choix est écrit dans sa propre police, pour
   * juger avant de choisir. Liste déroulante accessible (bouton + listbox),
   * fermée par Échap ou un clic à l'extérieur.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { FONTS, FONT_CATEGORY_LABELS, type FontCategory, type FontOption } from '../fonts';

  let {
    label,
    value,
    onchange,
    categories = ['serif', 'sans', 'mono', 'script', 'system'],
    allowInterface = false
  }: {
    label: string;
    value: string;
    onchange: (id: string) => void;
    categories?: FontCategory[];
    /** Ajoute « Comme l'interface » (valeur `ui`), pour les titres. */
    allowInterface?: boolean;
  } = $props();

  let open = $state(false);
  let root = $state<HTMLDivElement | null>(null);
  let trigger = $state<HTMLButtonElement | null>(null);
  /** Position de la liste : fixe, pour ne jamais être rognée par un parent défilant. */
  let pos = $state({ top: 0, left: 0, maxHeight: 256 });
  const uid = `fp-${Math.random().toString(36).slice(2, 8)}`;

  const groups = $derived(
    categories.map((c) => ({ category: c, fonts: FONTS.filter((f) => f.category === c) }))
  );
  const current = $derived<FontOption | undefined>(FONTS.find((f) => f.id === value));
  const currentLabel = $derived(value === 'ui' ? "Comme l'interface" : (current?.label ?? 'Police'));

  const LIST_WIDTH = 272;

  function toggle(): void {
    if (!open && trigger) {
      const r = trigger.getBoundingClientRect();
      const below = window.innerHeight - r.bottom - 12;
      const above = r.top - 12;
      const up = below < 220 && above > below;
      const maxHeight = Math.max(160, Math.min(320, up ? above : below));
      pos = {
        top: up ? r.top - maxHeight - 4 : r.bottom + 4,
        left: Math.max(8, Math.min(window.innerWidth - LIST_WIDTH - 8, r.right - LIST_WIDTH)),
        maxHeight
      };
    }
    open = !open;
  }

  function pick(id: string): void {
    onchange(id);
    open = false;
  }

  onMount(() => {
    const away = (e: MouseEvent) => {
      if (open && root && !root.contains(e.target as Node)) open = false;
    };
    document.addEventListener('mousedown', away);
    return () => document.removeEventListener('mousedown', away);
  });
</script>

<div class="picker" bind:this={root}>
  <span class="lbl" id={`${uid}-l`}>{label}</span>
  <button
    class="current"
    bind:this={trigger}
    style:font-family={current?.css}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-labelledby={`${uid}-l`}
    onclick={toggle}
    onkeydown={(e) => {
      if (e.key === 'Escape' && open) {
        e.stopPropagation();
        open = false;
      }
    }}
  >
    <span class="name">{currentLabel}</span>
    <Icon name="chevron-down" size={14} />
  </button>

  {#if open}
    <ul
      class="list"
      style:top="{pos.top}px"
      style:left="{pos.left}px"
      style:max-height="{pos.maxHeight}px"
      role="listbox"
      aria-labelledby={`${uid}-l`}
      tabindex="-1"
      onkeydown={(e) => {
        if (e.key === 'Escape') {
          e.stopPropagation();
          open = false;
        }
      }}
    >
      {#if allowInterface}
        <li>
          <button role="option" aria-selected={value === 'ui'} class:sel={value === 'ui'} onclick={() => pick('ui')}>
            Comme l'interface
          </button>
        </li>
      {/if}
      {#each groups as g (g.category)}
        <li class="group" role="presentation">{FONT_CATEGORY_LABELS[g.category]}</li>
        {#each g.fonts as font (font.id)}
          <li>
            <button
              role="option"
              aria-selected={value === font.id}
              class:sel={value === font.id}
              style:font-family={font.css}
              onclick={() => pick(font.id)}
            >
              <span>{font.label}</span>
              {#if value === font.id}<Icon name="check" size={14} />{/if}
            </button>
          </li>
        {/each}
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.3rem 0;
  }
  .lbl {
    color: var(--text);
    font-size: 0.88rem;
  }
  .current {
    display: inline-flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    min-width: 13rem;
    height: 34px;
    padding: 0 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text);
    font-size: 0.95rem;
    cursor: pointer;
  }
  .current:hover {
    border-color: var(--text-faint);
  }
  .list {
    position: fixed;
    z-index: 200;
    width: 17rem;
    margin: 0;
    padding: 0.3rem;
    overflow-y: auto;
    list-style: none;
    background: var(--surface-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
  }
  .group {
    padding: 0.5rem 0.55rem 0.2rem;
    color: var(--text-faint);
    font-family: var(--font-ui);
    font-size: 0.66rem;
    font-weight: 650;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .list button {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 0.42rem 0.55rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font-size: 1rem;
    text-align: left;
    cursor: pointer;
  }
  .list button:hover {
    background: var(--hover);
  }
  .list button.sel {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
</style>
