<script lang="ts">
  import Icon from './Icon.svelte';

  /** État d'écran vide compréhensible (cahier des charges §4.3). */
  let {
    icon = 'notes',
    title,
    message,
    actionLabel,
    onaction
  }: {
    icon?: 'notes' | 'search' | 'edit';
    title: string;
    message: string;
    actionLabel?: string;
    onaction?: () => void;
  } = $props();
</script>

<div class="empty" role="status">
  <div class="empty-icon"><Icon name={icon} size={28} /></div>
  <h3>{title}</h3>
  <p>{message}</p>
  {#if actionLabel && onaction}
    <button class="empty-action" onclick={onaction}>{actionLabel}</button>
  {/if}
</div>

<style>
  .empty {
    display: grid;
    justify-items: center;
    align-content: center;
    gap: 0.35rem;
    text-align: center;
    padding: 2rem;
    height: 100%;
  }

  .empty-icon {
    display: grid;
    place-items: center;
    width: 48px;
    height: 48px;
    border-radius: var(--radius);
    background: var(--accent-soft);
    color: var(--accent-text);
    margin-bottom: 0.5rem;
  }

  h3 {
    margin: 0;
    font-size: 0.95rem;
    color: var(--text);
  }

  p {
    margin: 0;
    font-size: 0.84rem;
    color: var(--text-soft);
    max-width: 24rem;
    line-height: 1.5;
  }

  .empty-action {
    margin-top: 0.9rem;
    appearance: none;
    border: none;
    cursor: pointer;
    font: inherit;
    font-size: 0.88rem;
    padding: 0.55rem 1rem;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--on-accent);
    transition: background 0.15s ease;
  }
  .empty-action:hover {
    background: var(--accent-strong);
  }
</style>
