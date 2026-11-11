<script lang="ts">
  /**
   * Fenêtre principale : les trois zones du cahier des charges (§4.2).
   *   Zone A — barre latérale   Zone B — liste des notes   Zone C — éditeur
   * Porte aussi les raccourcis clavier globaux (cahier des charges §3.6) : créer,
   * rechercher et enregistrer une note.
   */
  import Sidebar from '../lib/components/Sidebar.svelte';
  import NoteList from '../lib/components/NoteList.svelte';
  import EditorPane from '../lib/components/EditorPane.svelte';
  import Icon from '../lib/components/Icon.svelte';
  import { fly } from 'svelte/transition';
  import SettingsDialog from '../lib/components/SettingsDialog.svelte';
  import CommandPalette from '../lib/components/CommandPalette.svelte';
  import ReviewSession from '../lib/components/ReviewSession.svelte';
  import Guide from '../lib/components/Guide.svelte';
  import AttachmentViewer from '../lib/components/AttachmentViewer.svelte';
  import ShareDialog from '../lib/components/ShareDialog.svelte';
  import ImportDialog from '../lib/components/ImportDialog.svelte';
  import FolderPickerDialog from '../lib/components/FolderPickerDialog.svelte';
  import FolderTemplateDialog from '../lib/components/FolderTemplateDialog.svelte';
  import ReviewNudge from '../lib/components/ReviewNudge.svelte';
  import SplitPane from '../lib/components/SplitPane.svelte';
  import { viewer } from '../lib/stores/files';
  import { onMount } from 'svelte';
  import { guideOpen, initGuide } from '../lib/stores/guide';
  import { reviewOpen } from '../lib/stores/review';
  import { createNote, flushSave, transferNotice } from '../lib/stores/workspace';
  import { api } from '../lib/api/client';
  import { focusMode, folderPicker, importOpen, paletteOpen, settingsOpen, shareDialog, templateDialog } from '../lib/stores/ui';
  import { splitNoteId, splitOpen } from '../lib/stores/workspace';
  import { updateApplying, updateDismissed, updateInfo, updateProgress, applyUpdate, checkForUpdate } from '../lib/stores/update';
  import { listCollapsed, sidebarCollapsed } from '../lib/stores/ui';

  /**
   * Raccourcis globaux (cahier des charges §3.6). On ne réagit qu'à Ctrl/Cmd pour ne
   * jamais gêner la frappe : N (nouvelle note), K (palette de recherche),
   * S (enregistrer), \\ (concentration), « , » (paramètres). Un raccourci
   * sans modificateur reste réservé au texte.
   */
  // Premier lancement : le guide s'ouvre une fois, puis reste rouvrable.
  // La vérification de mise à jour est un appel unique et silencieux :
  // hors ligne, elle échoue sans rien afficher.
  onMount(() => {
    void initGuide();
    void checkForUpdate();
  });

  // Message affiché dans la bannière après une tentative d'installation
  // qui ne redémarre pas l'application (erreur, mode manuel, Windows).
  let applyMsg = $state('');

  async function installUpdate(): Promise<void> {
    const msg = await applyUpdate();
    if (msg) applyMsg = msg;
  }

  function handleShortcut(event: KeyboardEvent): void {
    // Échap referme le volet secondaire — le réflexe « annuler le scindage ».
    // Une boîte modale ouverte garde la priorité (ses propres Échap).
    if (event.key === 'Escape' && $splitOpen && !$focusMode) {
      if (!document.querySelector('[aria-modal="true"], dialog[open]')) {
        splitNoteId.set(null);
      }
      return;
    }
    if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
    // Une boîte modale ouverte capture l'attention : on lui laisse la main
    // (la palette referme elle-même sur Ctrl+K).
    if (document.querySelector('[aria-modal="true"]')) return;
    switch (event.key.toLowerCase()) {
      case 'n':
        event.preventDefault();
        void createNote();
        break;
      case 'k':
        event.preventDefault();
        paletteOpen.set(true);
        break;
      case '\\':
        event.preventDefault();
        focusMode.update((v) => !v);
        break;
      case ',':
        event.preventDefault();
        settingsOpen.set(true);
        break;
      case 's':
        event.preventDefault();
        void flushSave();
        break;
      case 'b':
        // Convention éditeur : Ctrl+B replie/déplie la barre latérale.
        event.preventDefault();
        sidebarCollapsed.update((v) => !v);
        break;
    }
  }
</script>

<svelte:window onkeydown={handleShortcut} />

<main
  class="workbench"
  class:focus={$focusMode}
  class:side-collapsed={$sidebarCollapsed}
  class:list-collapsed={$listCollapsed}
>
  <Sidebar />
  <NoteList />
  <div class="editor-zone" class:split={$splitOpen && !$focusMode}>
    <EditorPane />
    {#if $splitOpen && !$focusMode}
      <SplitPane />
    {/if}
  </div>
</main>

{#if $focusMode}
  <button class="leave-focus" onclick={() => focusMode.set(false)} title="Quitter le mode concentration — Ctrl+\\">
    <Icon name="panel-left" size={14} />
    Quitter la concentration
  </button>
{/if}

{#if $paletteOpen}
  <CommandPalette />
{/if}

{#if $settingsOpen}
  <SettingsDialog />
{/if}

{#if $reviewOpen}
  <ReviewSession />
{/if}

{#if $guideOpen}
  <Guide />
{/if}

{#if $viewer}
  <AttachmentViewer />
{/if}

{#if $shareDialog}
  <ShareDialog />
{/if}

{#if $importOpen}
  <ImportDialog />
{/if}

{#if $templateDialog}
  <FolderTemplateDialog />
{/if}

<ReviewNudge />

{#if $folderPicker}
  <FolderPickerDialog />
{/if}

{#if $updateInfo && !$updateDismissed}
  <div class="notice update" role="status" transition:fly={{ y: 8, duration: 150 }}>
    <Icon name="download" size={14} />
    {#if $updateApplying}
      <span>Téléchargement et installation… {$updateProgress ?? '…'} %</span>
    {:else if applyMsg}
      <span>{applyMsg}</span>
    {:else}
      <span>NoteCore {$updateInfo.latest} est disponible</span>
      <button class="update-link" onclick={() => void installUpdate()}>Mettre à jour</button>
      <button class="update-link" onclick={() => void api.openExternal($updateInfo!.url)}>Notes de version</button>
    {/if}
    {#if !$updateApplying}
      <button class="update-x" aria-label="Masquer" onclick={() => { updateDismissed.set(true); applyMsg = ''; }}><Icon name="x" size={12} /></button>
    {/if}
  </div>
{/if}

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
    grid-template-columns: 248px minmax(260px, 330px) 1fr;
    height: 100vh;
    overflow: hidden;
  }

  /* Zone d'écriture : un ou deux volets quand le split-screen est ouvert. */
  .editor-zone {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    min-width: 0;
    min-height: 0;
  }
  .editor-zone.split {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  /* Panneaux repliés : chaque colonne garde un rail de 46px avec les
     actions essentielles, au lieu de disparaître — l'éditeur gagne
     l'espace sans perdre l'accès aux fonctions. */
  .workbench.side-collapsed {
    grid-template-columns: 46px minmax(260px, 330px) 1fr;
  }
  .workbench.list-collapsed {
    grid-template-columns: 248px 46px 1fr;
  }
  .workbench.side-collapsed.list-collapsed {
    grid-template-columns: 46px 46px 1fr;
  }

  /* Concentration : seule la zone d'écriture reste visible. */
  .workbench.focus {
    grid-template-columns: 1fr;
  }
  .workbench.focus > :global(aside.sidebar),
  .workbench.focus > :global(section.note-list) {
    display: none;
  }

  .leave-focus {
    position: fixed;
    top: 0.6rem;
    left: 0.8rem;
    z-index: 50;
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.3rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface);
    color: var(--text-soft);
    font: inherit;
    font-size: 0.76rem;
    cursor: pointer;
    opacity: 0.55;
    transition: opacity 0.15s var(--ease);
  }
  .leave-focus:hover,
  .leave-focus:focus-visible {
    opacity: 1;
    color: var(--text);
  }

  /* Fenêtres étroites (demi-écran, mosaïque) : les colonnes latérales se
     resserrent, l'éditeur reste toujours visible et éditable
     (§3.6 « interface adaptée aux tailles de fenêtre courantes »). */
  @media (max-width: 900px) {
    .workbench {
      grid-template-columns: 200px minmax(200px, 250px) 1fr;
    }
    .workbench.side-collapsed {
      grid-template-columns: 46px minmax(200px, 250px) 1fr;
    }
    .workbench.list-collapsed {
      grid-template-columns: 200px 46px 1fr;
    }
    .workbench.side-collapsed.list-collapsed {
      grid-template-columns: 46px 46px 1fr;
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
    font-size: 0.84rem;
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
  .notice.update {
    bottom: 3.2rem;
  }
  .notice.update .update-link {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--accent-text);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
    padding: 0;
  }
  .notice.update .update-link:hover {
    text-decoration: underline;
  }
  .notice.update .update-x {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.15rem;
    border-radius: var(--radius-sm);
    display: grid;
    place-items: center;
  }
  .notice.update .update-x:hover {
    background: var(--hover);
    color: var(--text);
  }
</style>
