<script lang="ts">
  /**
   * Zone A — Barre latérale (cahier des charges §4.2)
   * Identité, création de note, recherche (palette), vues, dossiers (suppression
   * sans perte de notes, §3.2), étiquettes. Les réglages et les opérations sur
   * les données vivent dans la fenêtre Paramètres.
   */
  import Icon from './Icon.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import Brand from './Brand.svelte';
  import { tagTint } from '../utils/tags';
  import {
    allTags,
    createFolder,
    createNote,
    currentView,
    deleteFolder,
    favoritesCount,
    folderError,
    folders,
    goHome,
    loaded,
    notes,
    renameFolder,
    restoreBackup,
    restorePreview,
    selectedNoteId
  } from '../stores/workspace';
  import { paletteOpen, settingsOpen } from '../stores/ui';
  import { reviewStats, startReview } from '../stores/review';
  import { theme, setThemePref } from '../stores/appTheme';
  import { formatFullDate } from '../utils/format';
  import type { RestorePreview, View } from '../api/types';

  let creatingFolder = $state(false);
  let newFolderName = $state('');
  let folderPendingDelete = $state<string | null>(null);
  let editingFolderId = $state<string | null>(null);
  let folderEditName = $state('');

  let folderInput = $state<HTMLInputElement | null>(null);
  let folderRenameInput = $state<HTMLInputElement | null>(null);

  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);

  /** Identité de la sauvegarde, présentée avant l'écrasement des données. */
  function describeBackup(preview: RestorePreview): string {
    const parts: string[] = [];
    if (preview.createdAt) parts.push(`créée le ${formatFullDate(preview.createdAt)}`);
    if (preview.notes > 0) parts.push(`${preview.notes} note${preview.notes > 1 ? 's' : ''}`);
    return parts.length > 0 ? ` (${parts.join(', ')})` : '';
  }

  /** Nombre de notes par dossier, calculé une fois par changement de liste. */
  const folderCounts = $derived.by(() => {
    const counts = new Map<string, number>();
    for (const n of $notes) {
      if (n.folderId) counts.set(n.folderId, (counts.get(n.folderId) ?? 0) + 1);
    }
    return counts;
  });

  function isActive(view: View, kind: string, arg?: string): boolean {
    if (view.kind === 'folder' && kind === 'folder') return view.folderId === arg;
    if (view.kind === 'tag' && kind === 'tag') return view.tag === arg;
    return view.kind === kind;
  }

  function startCreateFolder() {
    creatingFolder = true;
    newFolderName = '';
    folderError.set(null);
    setTimeout(() => folderInput?.focus(), 0);
  }

  async function submitFolder() {
    const name = newFolderName.trim();
    if (!name) {
      creatingFolder = false;
      newFolderName = '';
      return;
    }
    // En cas d'échec, le champ reste ouvert et l'erreur s'affiche dessous :
    // la saisie n'est pas perdue et l'erreur n'est pas confondue avec un
    // problème d'enregistrement de la note ouverte.
    if (await createFolder(name)) {
      creatingFolder = false;
      newFolderName = '';
    } else {
      setTimeout(() => folderInput?.focus(), 0);
    }
  }

  function startRenameFolder(folderId: string, name: string): void {
    editingFolderId = folderId;
    folderEditName = name;
    folderError.set(null);
    setTimeout(() => folderRenameInput?.focus(), 0);
  }

  async function submitRenameFolder(): Promise<void> {
    const id = editingFolderId;
    const name = folderEditName.trim();
    if (!id || !name) {
      editingFolderId = null;
      folderEditName = '';
      return;
    }
    if (await renameFolder(id, name)) {
      editingFolderId = null;
      folderEditName = '';
    } else {
      setTimeout(() => folderRenameInput?.focus(), 0);
    }
  }

  function confirmDeleteFolder() {
    if (folderPendingDelete) void deleteFolder(folderPendingDelete);
    folderPendingDelete = null;
  }

  const viewLabels = [
    { kind: 'all', icon: 'notes', label: 'Toutes les notes' },
    { kind: 'favorites', icon: 'star', label: 'Favoris' },
    { kind: 'recent', icon: 'clock', label: 'Récentes' }
  ] as const;

  function toggleTheme(): void {
    setThemePref($theme.theme === 'dark' ? 'light' : 'dark');
  }
</script>

<aside class="sidebar">
  {#if !$loaded}
    <div class="loading">Chargement…</div>
  {:else}
    <div class="brand">
      <Brand size={30} />
    </div>

    <div class="top">
      <button
        class="new-note"
        title="Créer une note — Ctrl+N"
        aria-keyshortcuts="Control+N Meta+N"
        onclick={() => void createNote($currentView.kind === 'folder' ? $currentView.folderId : null)}
      >
        <Icon name="plus" size={15} stroke={2.2} />
        Nouvelle note
      </button>

      <button
        class="finder"
        title="Rechercher et exécuter des commandes"
        aria-keyshortcuts="Control+K Meta+K"
        onclick={() => paletteOpen.set(true)}
      >
        <Icon name="search" size={14} />
        <span>Rechercher…</span>
        <kbd>{isMac ? '⌘' : 'Ctrl'} K</kbd>
      </button>
    </div>

    <div class="scroll">
      <nav class="nav" aria-label="Navigation">
        <button
          class="nav-item"
          class:active={$selectedNoteId === null}
          aria-current={$selectedNoteId === null ? 'page' : undefined}
          onclick={() => void goHome()}
        >
          <Icon name="home" size={16} />
          <span class="nav-label">Accueil</span>
        </button>
        <button
          class="nav-item review"
          title="Lancer une révision"
          onclick={() => void startReview()}
        >
          <Icon name="layers" size={16} />
          <span class="nav-label">Réviser</span>
          {#if ($reviewStats?.due ?? 0) + ($reviewStats?.new ?? 0) > 0}
            <span class="due" aria-label="cartes à voir">{($reviewStats?.due ?? 0) + ($reviewStats?.new ?? 0)}</span>
          {/if}
        </button>
        {#each viewLabels as item (item.kind)}
          <button
            class="nav-item"
            class:active={isActive($currentView, item.kind) && $selectedNoteId !== null}
            class:current-view={isActive($currentView, item.kind)}
            aria-current={isActive($currentView, item.kind) ? 'page' : undefined}
            onclick={() => currentView.set({ kind: item.kind })}
          >
            <Icon name={item.icon} size={16} />
            <span class="nav-label">{item.label}</span>
            <span class="count">{item.kind === 'favorites' ? $favoritesCount : $notes.length}</span>
          </button>
        {/each}
      </nav>

      <div class="section">
        <div class="section-head">
          <span class="section-title">Dossiers</span>
          <button class="mini-btn" title="Nouveau dossier" aria-label="Nouveau dossier" onclick={startCreateFolder}>
            <Icon name="plus" size={13} stroke={2.2} />
          </button>
        </div>

        {#if creatingFolder}
          <input
            class="folder-input"
            bind:this={folderInput}
            bind:value={newFolderName}
            placeholder="Nom du dossier"
            aria-label="Nom du nouveau dossier"
            onkeydown={(e) => {
              if (e.key === 'Enter') submitFolder();
              if (e.key === 'Escape') {
                creatingFolder = false;
                newFolderName = '';
              }
            }}
            onblur={submitFolder}
          />
        {/if}

        <ul class="list">
          {#each $folders as folder (folder.id)}
            <li class="row">
              {#if editingFolderId === folder.id}
                <input
                  class="folder-input"
                  bind:this={folderRenameInput}
                  bind:value={folderEditName}
                  aria-label={`Renommer le dossier ${folder.name}`}
                  onkeydown={(e) => {
                    if (e.key === 'Enter') void submitRenameFolder();
                    if (e.key === 'Escape') {
                      editingFolderId = null;
                      folderEditName = '';
                    }
                  }}
                  onblur={() => void submitRenameFolder()}
                />
              {:else}
                <button
                  class="nav-item folder"
                  class:current-view={isActive($currentView, 'folder', folder.id)}
                  aria-current={isActive($currentView, 'folder', folder.id) ? 'page' : undefined}
                  onclick={() => currentView.set({ kind: 'folder', folderId: folder.id })}
                  ondblclick={() => startRenameFolder(folder.id, folder.name)}
                  title="Double-clic pour renommer"
                >
                  <Icon name="folder" size={16} />
                  <span class="nav-label">{folder.name}</span>
                  <span class="count">{folderCounts.get(folder.id) ?? 0}</span>
                </button>
                <span class="row-actions">
                  <button
                    class="row-btn"
                    title={`Renommer « ${folder.name} »`}
                    aria-label={`Renommer le dossier ${folder.name}`}
                    onclick={() => startRenameFolder(folder.id, folder.name)}
                  >
                    <Icon name="edit" size={13} />
                  </button>
                  <button
                    class="row-btn danger"
                    title={`Supprimer « ${folder.name} » (les notes sont conservées)`}
                    aria-label={`Supprimer le dossier ${folder.name}`}
                    onclick={() => (folderPendingDelete = folder.id)}
                  >
                    <Icon name="trash" size={13} />
                  </button>
                </span>
              {/if}
            </li>
          {/each}
          {#if $folders.length === 0 && !creatingFolder}
            <li class="list-empty">Aucun dossier</li>
          {/if}
        </ul>

        {#if $folderError}
          <p class="folder-error" role="alert">
            <span>{$folderError}</span>
            <button
              class="error-dismiss"
              title="Masquer le message"
              aria-label="Masquer le message"
              onclick={() => folderError.set(null)}
            >
              <Icon name="x" size={12} />
            </button>
          </p>
        {/if}
      </div>

      <div class="section">
        <div class="section-head">
          <span class="section-title">Étiquettes</span>
        </div>
        <div class="tag-cloud">
          {#each $allTags as tag (tag)}
            <button
              class="tag-chip tint-{tagTint(tag)}"
              class:active={isActive($currentView, 'tag', tag)}
              aria-pressed={isActive($currentView, 'tag', tag)}
              onclick={() =>
                currentView.set(isActive($currentView, 'tag', tag) ? { kind: 'all' } : { kind: 'tag', tag })}
            >
              {tag}
            </button>
          {/each}
          {#if $allTags.length === 0}
            <span class="list-empty">Aucune étiquette</span>
          {/if}
        </div>
      </div>
    </div>

    <div class="footer">
      <button class="footer-btn grow" onclick={() => settingsOpen.set(true)} title="Paramètres — Ctrl+,">
        <Icon name="settings" size={16} />
        Paramètres
      </button>
      <button
        class="footer-btn"
        onclick={toggleTheme}
        title={$theme.theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
        aria-label="Basculer le thème"
      >
        <Icon name={$theme.theme === 'dark' ? 'sun' : 'moon'} size={16} />
      </button>
    </div>
  {/if}
</aside>

{#if folderPendingDelete}
  <ConfirmDialog
    title="Supprimer ce dossier ?"
    message="Les notes du dossier ne seront pas supprimées : elles resteront accessibles, sans dossier de rattachement."
    confirmLabel="Supprimer le dossier"
    onconfirm={confirmDeleteFolder}
    oncancel={() => (folderPendingDelete = null)}
  />
{/if}

{#if $restorePreview}
  <ConfirmDialog
    title="Restaurer cette sauvegarde ?"
    message={`La sauvegarde « ${$restorePreview.name} »${describeBackup($restorePreview)} remplacera toutes vos données actuelles. Une copie de sécurité de vos données actuelles est conservée automatiquement. Cette action ne peut pas être annulée.`}
    confirmLabel="Restaurer"
    onconfirm={() => void restoreBackup()}
    oncancel={() => restorePreview.set(null)}
  />
{/if}

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--sidebar-bg);
    border-right: 1px solid var(--border);
  }

  .loading {
    padding: 1.2rem;
    color: var(--text-soft);
    font-size: 0.85rem;
  }

  .brand {
    padding: 1.1rem 1.1rem 0.7rem;
  }
  .top {
    display: grid;
    gap: 0.5rem;
    padding: 0.4rem 0.8rem 0.7rem;
  }

  .new-note {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.45rem;
    height: 36px;
    border: none;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-size: 0.88rem;
    font-weight: 600;
    cursor: pointer;
    box-shadow: var(--shadow-sm);
    transition:
      background 0.12s var(--ease),
      transform 0.12s var(--ease);
  }
  .new-note:hover {
    background: var(--accent-strong);
  }
  .new-note:active {
    transform: translateY(1px);
  }

  .finder {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    height: 34px;
    padding: 0 0.6rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text-faint);
    font: inherit;
    font-size: 0.84rem;
    text-align: left;
    cursor: pointer;
    transition: border-color 0.12s var(--ease);
  }
  .finder:hover {
    border-color: var(--text-faint);
    color: var(--text-soft);
  }
  .finder span {
    flex: 1;
  }
  kbd {
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--border);
    border-radius: 5px;
    background: var(--surface);
    font-family: var(--font-ui);
    font-size: 0.68rem;
    color: var(--text-faint);
  }

  .scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0 0.5rem 0.5rem;
  }

  .nav {
    display: grid;
    gap: 1px;
  }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 0.65rem;
    width: 100%;
    height: var(--nav-h);
    padding: 0 0.6rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.88rem;
    text-align: left;
    cursor: pointer;
    transition:
      background 0.1s var(--ease),
      color 0.1s var(--ease);
  }
  .nav-item:hover {
    background: var(--hover);
    color: var(--text);
  }
  .nav-item.active,
  .nav-item.current-view {
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 600;
  }
  /* Une note ouverte : la vue reste indiquée, plus discrètement. */
  .nav-item.current-view:not(.active) {
    background: transparent;
    color: var(--text);
  }
  .nav-label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .due {
    min-width: 1.35rem;
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--accent);
    color: var(--on-accent);
    font-size: 0.7rem;
    font-weight: 650;
    line-height: 1.35rem;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
  .count {
    min-width: 1.4rem;
    color: var(--text-faint);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    text-align: right;
  }
  .nav-item.active .count {
    color: var(--accent-text);
  }

  .section {
    margin-top: 1.1rem;
  }
  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 0.6rem 0.3rem;
  }
  .section-title {
    color: var(--text-faint);
    font-size: 0.68rem;
    font-weight: 650;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .mini-btn {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .mini-btn:hover {
    background: var(--hover);
    color: var(--text);
  }

  .list {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 1px;
  }
  .row {
    position: relative;
    display: flex;
    align-items: center;
  }
  .row .nav-item {
    padding-right: 3.4rem;
  }
  .row-actions {
    position: absolute;
    right: 0.25rem;
    display: none;
    gap: 1px;
  }
  .row:hover .row-actions,
  .row:focus-within .row-actions {
    display: inline-flex;
  }
  .row:hover .count,
  .row:focus-within .count {
    visibility: hidden;
  }
  .row-btn {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
  }
  .row-btn:hover {
    background: var(--chip-bg);
    color: var(--text);
  }
  .row-btn.danger:hover {
    background: var(--danger-soft);
    color: var(--danger);
  }

  .list-empty {
    padding: 0.2rem 0.6rem;
    color: var(--text-faint);
    font-size: 0.8rem;
  }

  .folder-input {
    width: 100%;
    height: 30px;
    margin-bottom: 2px;
    padding: 0 0.6rem;
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
  }
  .folder-input:focus {
    outline: 2px solid var(--accent-soft);
    outline-offset: 0;
  }

  .folder-error {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.4rem;
    margin: 0.4rem 0.3rem 0;
    padding: 0.45rem 0.6rem;
    border-radius: var(--radius-sm);
    background: var(--danger-soft);
    color: var(--danger);
    font-size: 0.78rem;
    line-height: 1.4;
  }
  .error-dismiss {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: inherit;
    cursor: pointer;
  }

  .tag-cloud {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
    padding: 0.15rem 0.6rem 0;
  }
  .tag-chip {
    padding: 0.15rem 0.6rem;
    border: 1px solid transparent;
    border-radius: 999px;
    background: var(--tint-bg);
    color: var(--tint-ink);
    font: inherit;
    font-size: 0.76rem;
    cursor: pointer;
    transition: border-color 0.1s var(--ease);
  }
  .tag-chip:hover {
    border-color: var(--tint-line);
  }
  .tag-chip.active {
    border-color: var(--tint-ink);
    font-weight: 600;
  }

  .footer {
    display: flex;
    gap: 0.3rem;
    padding: 0.6rem 0.5rem;
    border-top: 1px solid var(--border);
  }
  .footer-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: 32px;
    padding: 0 0.6rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.86rem;
    cursor: pointer;
  }
  .footer-btn.grow {
    flex: 1;
  }
  .footer-btn:hover {
    background: var(--hover);
    color: var(--text);
  }
</style>
