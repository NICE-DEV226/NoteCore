<script lang="ts">
  /**
   * Fenêtre principale : les trois zones du cahier des charges (§4.2).
   *   Zone A — barre latérale   Zone B — liste des notes   Zone C — éditeur
   * Porte aussi les raccourcis clavier globaux (cdc.md §3.6) : créer,
   * rechercher et enregistrer une note.
   */
  import Sidebar from '../lib/components/Sidebar.svelte';
  import NoteList from '../lib/components/NoteList.svelte';
  import EditorPane from '../lib/components/EditorPane.svelte';
  import Icon from '../lib/components/Icon.svelte';
  import { fly } from 'svelte/transition';
  import { createNote, flushSave, transferNotice } from '../lib/stores/workspace';
  import { SEARCH_INPUT_ID } from '../lib/constants';

  function focusSearch(): void {
    const input = document.getElementById(SEARCH_INPUT_ID);
    if (input instanceof HTMLInputElement) {
      input.focus();
      input.select();
    }
  }

  /**
   * Raccourcis globaux (cdc.md §3.6). On ne réagit qu'à Ctrl/Cmd pour ne
   * jamais gêner la frappe : N (nouvelle note), K (rechercher), S
   * (enregistrer). Un raccourci sans modificateur reste réservé au texte.
   */
  function handleShortcut(event: KeyboardEvent): void {
    if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
    // Une boîte modale ouverte capture l'attention : on lui laisse la main.
    if (document.querySelector('[aria-modal="true"]')) return;
    switch (event.key.toLowerCase()) {
      case 'n':
        event.preventDefault();
        void createNote();
        break;
      case 'k':
        event.preventDefault();
        focusSearch();
        break;
      case 's':
        event.preventDefault();
        void flushSave();
        break;
    }
  }
</script>

<svelte:window onkeydown={handleShortcut} />

<main class="workbench">
  <Sidebar />
  <NoteList />
  <EditorPane />
</main>

{#if $transferNotice}
  <div
    class="notice"
    class:is-error={$transferNotice.kind === 'error'}
    role="status"
    transition:fly={{ y: 8, duration: 150 }}
  >
    <Icon name={$transferNotice.kind === 'error' ? 'alert' : 'check'} size={14} />
    {$transferNotice.text}
  </div>
{/if}

<style>
  .workbench {
    display: grid;
    grid-template-columns: 240px minmax(240px, 320px) 1fr;
    height: 100vh;
    overflow: hidden;
  }

  /* Fenêtres étroites : la liste prend l'espace, l'éditeur se replie
     (§3.6 « interface adaptée aux tailles de fenêtre courantes »). */
  @media (max-width: 900px) {
    .workbench {
      grid-template-columns: 210px 1fr;
    }
    .workbench > :global(section.editor) {
      display: none;
    }
  }

  /* Message de résultat des opérations d'export / sauvegarde. */
  .notice {
    position: fixed;
    right: 1rem;
    bottom: 1rem;
    z-index: 200;
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    max-width: 26rem;
    padding: 0.55rem 0.9rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow-lg);
    color: var(--text);
    font-size: 0.82rem;
  }
  .notice :global(svg) {
    color: var(--success);
  }
  .notice.is-error {
    color: var(--danger);
    border-color: var(--danger-soft);
  }
  .notice.is-error :global(svg) {
    color: var(--danger);
  }
</style>
