<script lang="ts">
  /**
   * Rappel de révision : quand des cartes sont dues, une invite discrète
   * propose une séance. Elle n'apparaît qu'une fois par jour (la date de
   * refus est gardée localement) et disparaît dès qu'une séance est ouverte.
   */
  import { fade } from 'svelte/transition';
  import Icon from './Icon.svelte';
  import { reviewOpen, reviewStats, startReview } from '../stores/review';

  const DISMISS_KEY = 'notecore-review-nudge-dismissed';
  const today = new Date().toISOString().slice(0, 10);

  let dismissed = $state(
    typeof localStorage !== 'undefined' && localStorage.getItem(DISMISS_KEY) === today
  );

  const due = $derived($reviewStats?.due ?? 0);
  const show = $derived(due > 0 && !dismissed && !$reviewOpen);

  function dismiss(): void {
    dismissed = true;
    try {
      localStorage.setItem(DISMISS_KEY, today);
    } catch {
      // Stockage indisponible : l'invite revient à la prochaine ouverture.
    }
  }
</script>

{#if show}
  <div class="nudge" role="status" transition:fade={{ duration: 180 }}>
    <span class="icon"><Icon name="flame" size={17} /></span>
    <div class="text">
      <strong>{due} carte{due > 1 ? 's' : ''} à réviser</strong>
      <small>Quelques minutes suffisent à garder la mémoire fraîche.</small>
    </div>
    <button class="go" onclick={() => void startReview()}>Réviser</button>
    <button class="x" aria-label="Plus tard" title="Plus tard" onclick={dismiss}>
      <Icon name="x" size={14} />
    </button>
  </div>
{/if}

<style>
  .nudge {
    position: fixed;
    right: 1.1rem;
    bottom: 1.1rem;
    z-index: 90;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    max-width: 21rem;
    padding: 0.75rem 0.85rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
  }
  .icon {
    flex: none;
    display: grid;
    place-items: center;
    width: 2rem;
    height: 2rem;
    border-radius: var(--radius);
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .text strong { font-size: 0.86rem; }
  .text small { font-size: 0.76rem; color: var(--text-soft); }
  .go {
    appearance: none;
    border: none;
    font: inherit;
    font-size: 0.82rem;
    font-weight: 600;
    padding: 0.42rem 0.85rem;
    border-radius: var(--radius);
    cursor: pointer;
    background: var(--accent);
    color: var(--on-accent);
    white-space: nowrap;
  }
  .go:hover { filter: brightness(1.08); }
  .x {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: var(--radius);
    display: grid;
    place-items: center;
    flex: none;
  }
  .x:hover { background: var(--hover); color: var(--text); }
</style>
