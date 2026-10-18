<script lang="ts">
  /**
   * Séance de révision en plein écran. Un geste par carte, tout au clavier :
   * Espace pour retourner, 1 à 4 pour répondre, Z pour annuler, Échap pour
   * quitter. Le texte des cartes vient des notes (Markdown rendu par notre
   * moteur : échappé puis balisé, jamais de HTML entrant).
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { renderMarkdown } from '../editor/markdown';
  import { formatUntil } from '../utils/format';
  import {
    answerCard,
    closeReview,
    currentCard,
    reveal,
    revealed,
    reviewBusy,
    reviewError,
    reviewStats,
    tally,
    undoAnswer,
    undoable
  } from '../stores/review';
  import { selectNote } from '../stores/workspace';
  import type { ReviewRating } from '../api/types';

  let dialogEl = $state<HTMLDivElement | null>(null);

  /** Le marqueur de trou devient une pastille ; le reste est du Markdown rendu. */
  function html(md: string): string {
    return renderMarkdown(md).replace(/\[…\]/g, '<span class="blank">&nbsp;&nbsp;&nbsp;&nbsp;</span>');
  }

  const card = $derived($currentCard);
  const isCloze = $derived(card?.kind === 'cloze');
  const frontHtml = $derived(card ? html(card.front) : '');
  const backHtml = $derived(card ? html(card.back) : '');
  const remaining = $derived(($reviewStats?.due ?? 0) + ($reviewStats?.new ?? 0));

  const RATINGS: Array<{ value: ReviewRating; label: string; key: string; tone: string; interval: 'again' | 'hard' | 'good' | 'easy' }> = [
    { value: 1, label: 'À revoir', key: '1', tone: 'again', interval: 'again' },
    { value: 2, label: 'Difficile', key: '2', tone: 'hard', interval: 'hard' },
    { value: 3, label: 'Bien', key: '3', tone: 'good', interval: 'good' },
    { value: 4, label: 'Facile', key: '4', tone: 'easy', interval: 'easy' }
  ];

  function openNote(): void {
    const id = card?.noteId;
    closeReview();
    if (id) void selectNote(id);
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.focus();

    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeReview();
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z') {
        e.preventDefault();
        void undoAnswer();
        return;
      }
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      if (e.key === ' ' || e.key === 'Enter') {
        // Un bouton focalisé garde son comportement : pas de double action.
        if ((e.target as HTMLElement | null)?.tagName === 'BUTTON') return;
        e.preventDefault();
        if (!$revealed) reveal();
      } else if ($revealed && e.key >= '1' && e.key <= '4') {
        e.preventDefault();
        void answerCard(Number(e.key) as ReviewRating);
      } else if (e.key.toLowerCase() === 'z') {
        e.preventDefault();
        void undoAnswer();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

<div class="session" bind:this={dialogEl} role="dialog" aria-modal="true" aria-label="Séance de révision" tabindex="-1">
  <header>
    <button class="icon" title="Quitter la séance — Échap" aria-label="Quitter la séance" onclick={closeReview}>
      <Icon name="x" size={18} />
    </button>
    <div class="progress" aria-live="polite">
      {#if card}
        <span><strong>{remaining}</strong> à voir</span>
        <span class="dotsep">·</span>
        <span>{$tally.answered} répondue{$tally.answered > 1 ? 's' : ''}</span>
      {/if}
    </div>
    <button
      class="icon"
      title="Annuler la dernière réponse — Z"
      aria-label="Annuler la dernière réponse"
      disabled={!$undoable || $reviewBusy}
      onclick={() => void undoAnswer()}
    >
      <Icon name="undo" size={17} />
    </button>
  </header>

  <main>
    {#if $reviewError}
      <p class="error" role="alert">{$reviewError}</p>
    {/if}

    {#if card}
      <button class="source" onclick={openNote} title="Ouvrir la note d'origine">
        <Icon name="notes" size={13} />
        {card.noteTitle || 'Sans titre'}
      </button>

      <article class="card" class:revealed={$revealed}>
        {#if isCloze}
          <div class="text">{@html $revealed ? backHtml : frontHtml}</div>
        {:else}
          <div class="text question">{@html frontHtml}</div>
          {#if $revealed}
            <hr />
            <div class="text answer">{@html backHtml}</div>
          {/if}
        {/if}
      </article>

      {#if !$revealed}
        <button class="reveal" onclick={reveal}>
          Afficher la réponse
          <kbd>Espace</kbd>
        </button>
      {:else}
        <div class="rates" role="group" aria-label="Votre réponse">
          {#each RATINGS as r (r.value)}
            <button class="rate {r.tone}" disabled={$reviewBusy} onclick={() => void answerCard(r.value)}>
              <span class="when">{card.intervals[r.interval]}</span>
              <span class="what">{r.label}</span>
              <kbd>{r.key}</kbd>
            </button>
          {/each}
        </div>
      {/if}
    {:else if !$reviewBusy}
      <div class="done">
        <div class="badge"><Icon name="check" size={28} stroke={2.4} /></div>
        <h2>{$tally.answered > 0 ? 'Séance terminée' : 'Rien à réviser pour l’instant'}</h2>
        {#if $tally.answered > 0}
          <p class="sum">
            {$tally.answered} carte{$tally.answered > 1 ? 's' : ''} revue{$tally.answered > 1 ? 's' : ''}
            {#if $tally.again > 0}· {$tally.again} à revoir{/if}
          </p>
        {:else if ($reviewStats?.total ?? 0) === 0}
          <p class="sum">
            Écrivez une carte dans une note : <code>{'{{c1::mot caché}}'}</code> ou
            <code>Question :: Réponse</code>.
          </p>
        {/if}
        {#if $reviewStats?.nextDue}
          <p class="next">Prochaine révision {formatUntil($reviewStats.nextDue)}.</p>
        {/if}
        <button class="primary" onclick={closeReview}>Retour aux notes</button>
      </div>
    {/if}
  </main>
</div>

<style>
  .session {
    position: fixed;
    inset: 0;
    z-index: 180;
    display: flex;
    flex-direction: column;
    background: var(--app-bg);
    outline: none;
    animation: rise 0.18s var(--ease);
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: scale(1.01);
    }
  }
  header {
    display: grid;
    grid-template-columns: 44px 1fr 44px;
    align-items: center;
    padding: 0.7rem 1rem;
  }
  .icon {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
  }
  .icon:hover:not(:disabled) {
    background: var(--hover);
    color: var(--text);
  }
  .icon:disabled {
    opacity: 0.35;
    cursor: default;
  }
  .progress {
    display: flex;
    justify-content: center;
    gap: 0.5rem;
    color: var(--text-soft);
    font-size: 0.86rem;
  }
  .progress strong {
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }
  .dotsep {
    color: var(--text-faint);
  }

  main {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 1.4rem;
    padding: 0 1.5rem 3rem;
    overflow-y: auto;
  }
  .source {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    max-width: 30rem;
    padding: 0.25rem 0.7rem;
    border: none;
    border-radius: 999px;
    background: var(--chip-bg);
    color: var(--text-soft);
    font: inherit;
    font-size: 0.78rem;
    cursor: pointer;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .source:hover {
    color: var(--accent-text);
  }

  .card {
    width: min(42rem, 100%);
    min-height: 12rem;
    display: grid;
    align-content: center;
    padding: 2.2rem 2.4rem;
    border: 1px solid var(--card-border);
    border-radius: var(--radius-lg);
    background: var(--card-bg);
    box-shadow: var(--shadow-md);
    font-family: var(--editor-font);
    font-size: calc(var(--editor-font-size) * 1.2);
    line-height: 1.6;
    color: var(--text);
    animation: flip 0.16s var(--ease);
  }
  @keyframes flip {
    from {
      opacity: 0.4;
      transform: translateY(4px);
    }
  }
  .text :global(p) {
    margin: 0.4em 0;
  }
  .text :global(ul),
  .text :global(ol) {
    margin: 0.3em 0;
    padding-left: 1.3em;
  }
  .text :global(code) {
    padding: 0.05em 0.35em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.85em;
  }
  /* Le trou : un espace à compléter, puis la réponse mise en évidence. */
  .text :global(.blank) {
    display: inline-block;
    min-width: 4.5em;
    margin: 0 0.1em;
    border-bottom: 2px solid var(--accent);
    background: var(--accent-soft);
    border-radius: 4px 4px 0 0;
    vertical-align: baseline;
  }
  .text :global(mark) {
    padding: 0 0.25em;
    border-radius: 4px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 650;
  }
  .question {
    font-weight: 600;
  }
  hr {
    width: 100%;
    margin: 1.2rem 0;
    border: none;
    border-top: 1px dashed var(--border);
  }

  kbd {
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 5px;
    background: var(--chip-bg);
    font-family: var(--font-ui);
    font-size: 0.7rem;
    color: var(--text-faint);
  }
  .reveal {
    display: inline-flex;
    align-items: center;
    gap: 0.7rem;
    height: 46px;
    padding: 0 1.6rem;
    border: none;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-size: 0.98rem;
    font-weight: 600;
    cursor: pointer;
    box-shadow: var(--shadow-sm);
  }
  .reveal:hover {
    background: var(--accent-strong);
  }
  .reveal kbd {
    background: oklch(1 0 0 / 0.18);
    border-color: oklch(1 0 0 / 0.3);
    color: inherit;
  }

  .rates {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0.6rem;
    width: min(42rem, 100%);
  }
  .rate {
    display: grid;
    gap: 0.15rem;
    justify-items: center;
    padding: 0.7rem 0.4rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    cursor: pointer;
    transition:
      border-color 0.12s var(--ease),
      transform 0.12s var(--ease);
  }
  .rate:hover:not(:disabled) {
    transform: translateY(-1px);
  }
  .rate:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .rate .when {
    color: var(--text-faint);
    font-size: 0.74rem;
    font-variant-numeric: tabular-nums;
  }
  .rate .what {
    font-size: 0.92rem;
    font-weight: 650;
  }
  .rate.again:hover:not(:disabled) {
    border-color: var(--danger);
  }
  .rate.again .what {
    color: var(--danger);
  }
  .rate.hard:hover:not(:disabled) {
    border-color: var(--warning);
  }
  .rate.good:hover:not(:disabled) {
    border-color: var(--success);
  }
  .rate.good .what {
    color: var(--success);
  }
  .rate.easy:hover:not(:disabled) {
    border-color: var(--accent);
  }
  .rate.easy .what {
    color: var(--accent-text);
  }

  .error {
    margin: 0;
    padding: 0.5rem 0.9rem;
    border-radius: var(--radius-sm);
    background: var(--danger-soft);
    color: var(--danger);
    font-size: 0.86rem;
  }

  .done {
    display: grid;
    justify-items: center;
    gap: 0.6rem;
    max-width: 30rem;
    text-align: center;
  }
  .badge {
    display: grid;
    place-items: center;
    width: 64px;
    height: 64px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .done h2 {
    margin: 0.4rem 0 0;
    font-size: 1.5rem;
    letter-spacing: -0.02em;
  }
  .done p {
    margin: 0;
    color: var(--text-soft);
    line-height: 1.5;
  }
  .done code {
    padding: 0.05em 0.4em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.88em;
  }
  .primary {
    margin-top: 0.8rem;
    height: 40px;
    padding: 0 1.3rem;
    border: none;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
  }
  .primary:hover {
    background: var(--accent-strong);
  }

  @media (max-width: 640px) {
    .rates {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
    .card {
      padding: 1.4rem 1.2rem;
    }
  }
</style>
