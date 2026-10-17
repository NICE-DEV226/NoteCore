<script lang="ts">
  /**
   * Fenêtre de confirmation avant opération destructive
   * (cdc.md §4.3 : « Confirmation avant suppression »).
   *
   * Accessibilité : à l'ouverture, le focus entre dans la boîte et se pose
   * sur l'action la plus sûre (« Annuler ») ; Tab / Maj+Tab restent piégés
   * dans la boîte ; Échap annule ; à la fermeture, le focus revient à
   * l'élément qui avait déclenché la boîte.
   */
  import { onMount } from 'svelte';

  let {
    title,
    message,
    confirmLabel = 'Supprimer',
    onconfirm,
    oncancel
  }: {
    title: string;
    message: string;
    confirmLabel?: string;
    onconfirm: () => void;
    oncancel: () => void;
  } = $props();

  let dialogEl = $state<HTMLDivElement | null>(null);
  let cancelBtn = $state<HTMLButtonElement | null>(null);

  const uid = `dlg-${Math.random().toString(36).slice(2, 8)}`;
  const titleId = `${uid}-title`;
  const descId = `${uid}-desc`;

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;

    // On pose le focus sur « Annuler » : l'action la plus sûre pour une
    // opération destructive.
    cancelBtn?.focus();

    function focusables(): HTMLElement[] {
      if (!dialogEl) return [];
      return Array.from(
        dialogEl.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
      );
    }

    function onKeydown(event: KeyboardEvent): void {
      if (event.key === 'Escape') {
        event.preventDefault();
        oncancel();
        return;
      }
      if (event.key !== 'Tab') return;
      const items = focusables();
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }

    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

<div
  class="overlay"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) oncancel();
  }}
>
  <div
    class="dialog"
    bind:this={dialogEl}
    role="dialog"
    aria-modal="true"
    aria-labelledby={titleId}
    aria-describedby={descId}
    tabindex="-1"
  >
    <h3 id={titleId}>{title}</h3>
    <p id={descId}>{message}</p>
    <div class="actions">
      <button class="btn ghost" bind:this={cancelBtn} onclick={oncancel}>Annuler</button>
      <button class="btn danger" onclick={onconfirm}>{confirmLabel}</button>
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: oklch(0 0 0 / 0.45);
    display: grid;
    place-items: center;
    z-index: 100;
  }

  .dialog {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: 1.4rem;
    max-width: 26rem;
    width: calc(100% - 2rem);
    box-shadow: var(--shadow-lg);
  }
  .dialog:focus {
    outline: none;
  }

  h3 {
    margin: 0 0 0.4rem;
    font-size: 1rem;
  }

  p {
    margin: 0 0 1.2rem;
    color: var(--text-soft);
    font-size: 0.9rem;
    line-height: 1.5;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.6rem;
  }

  .btn {
    appearance: none;
    border: none;
    cursor: pointer;
    font: inherit;
    font-size: 0.88rem;
    padding: 0.5rem 0.95rem;
    border-radius: var(--radius);
    transition: background 0.15s ease;
  }

  .btn.ghost {
    background: transparent;
    color: var(--text);
  }
  .btn.ghost:hover {
    background: var(--hover);
  }

  .btn.danger {
    background: var(--danger);
    color: var(--on-danger);
  }
  .btn.danger:hover {
    background: var(--danger-strong);
  }
</style>
