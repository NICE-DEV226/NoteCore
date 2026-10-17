<script lang="ts">
  /**
   * Zone A — Barre latérale (cdc.md §4.2)
   * Identité visuelle, création de note, accès aux vues (toutes /
   * favoris / récentes), dossiers avec suppression sans perte de notes
   * (§3.2), étiquettes (§3.2), paramètres.
   */
  import Icon from './Icon.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import logoUrl from '../assets/logo.svg';
  import { tagTint } from '../utils/tags';
  import {
    allTags,
    createFolder,
    deleteFolder,
    renameFolder,
    favoritesCount,
    folders,
    loaded,
    currentView,
    notes
  } from '../stores/workspace';
  import { createNote, exportAllNotes, createBackup, restoreBackup } from '../stores/workspace';
  import { theme, setThemePref } from '../stores/appTheme';
  import { display, setDisplayPref } from '../stores/display';
  import type { View } from '../api/types';

  let creatingFolder = $state(false);
  let newFolderName = $state('');
  let folderPendingDelete = $state<string | null>(null);
  let restorePending = $state(false);
  let editingFolderId = $state<string | null>(null);
  let folderEditName = $state('');

  let folderInput = $state<HTMLInputElement | null>(null);
  let folderRenameInput = $state<HTMLInputElement | null>(null);

  function isActive(view: View, kind: string, arg?: string): boolean {
    if (view.kind === 'folder' && kind === 'folder') return view.folderId === arg;
    if (view.kind === 'tag' && kind === 'tag') return view.tag === arg;
    return view.kind === kind;
  }

  function startCreateFolder() {
    creatingFolder = true;
    newFolderName = '';
    setTimeout(() => folderInput?.focus(), 0);
  }

  function submitFolder() {
    if (newFolderName.trim()) void createFolder(newFolderName);
    creatingFolder = false;
    newFolderName = '';
  }

  function startRenameFolder(folderId: string, name: string): void {
    editingFolderId = folderId;
    folderEditName = name;
    setTimeout(() => folderRenameInput?.focus(), 0);
  }

  async function submitRenameFolder(): Promise<void> {
    const id = editingFolderId;
    const name = folderEditName.trim();
    editingFolderId = null;
    folderEditName = '';
    if (id && name) await renameFolder(id, name);
  }

  function folderNoteCount(id: string): number {
    // accès direct au store sans souscription supplémentaire
    let count = 0;
    for (const n of $notes) if (n.folderId === id) count++;
    return count;
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

</script>

<aside class="sidebar">
  {#if !$loaded}
    <div class="loading">Chargement…</div>
  {:else}
    <!-- Identité -->
    <div class="brand">
      <img class="brand-logo" src={logoUrl} alt="Logo NoteCore" />
    </div>

    <!-- Création -->
    <button
      class="new-note"
      title="Créer une note — Ctrl+N"
      aria-keyshortcuts="Control+N Meta+N"
      onclick={() => void createNote($currentView.kind === 'folder' ? $currentView.folderId : null)}
    >
      <Icon name="plus" size={15} />
      Nouvelle note
    </button>

    <!-- Vues principales -->
    <nav class="views">
      {#each viewLabels as item (item.kind)}
        <button
          class="view"
          class:active={isActive($currentView, item.kind)}
          aria-current={isActive($currentView, item.kind) ? 'page' : undefined}
          onclick={() => currentView.set({ kind: item.kind })}
        >
          <Icon name={item.icon} size={15} />
          <span class="view-label">{item.label}</span>
          {#if item.kind === 'favorites'}
            <span class="count">{$favoritesCount}</span>
          {:else}
            <span class="count">{$notes.length}</span>
          {/if}
        </button>
      {/each}
    </nav>

    <!-- Dossiers -->
    <div class="section">
      <div class="section-head">
        <span class="section-title">Dossiers</span>
        <button class="mini-btn" title="Nouveau dossier" onclick={startCreateFolder}>
          <Icon name="plus" size={12} />
        </button>
      </div>

      {#if creatingFolder}
        <input
          class="folder-input"
          bind:this={folderInput}
          bind:value={newFolderName}
          placeholder="Nom du dossier"
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
          <li>
            {#if editingFolderId === folder.id}
              <input
                class="folder-input folder-rename-input"
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
                class="view folder"
                class:active={isActive($currentView, 'folder', folder.id)}
                aria-current={isActive($currentView, 'folder', folder.id) ? 'page' : undefined}
                onclick={() => currentView.set({ kind: 'folder', folderId: folder.id })}
                ondblclick={() => startRenameFolder(folder.id, folder.name)}
                title="Cliquer pour ouvrir, double-cliquer pour renommer"
              >
                <Icon name="folder" size={15} />
                <span class="view-label">{folder.name}</span>
                <span class="count">{folderNoteCount(folder.id)}</span>
              </button>
            {/if}
            <button
              class="row-trash"
              title={`Supprimer le dossier « ${folder.name} » (les notes ne seront pas supprimées)`}
              onclick={() => (folderPendingDelete = folder.id)}
            >
              <Icon name="trash" size={13} />
            </button>
          </li>
        {/each}
        {#if $folders.length === 0 && !creatingFolder}
          <li class="list-empty">Aucun dossier</li>
        {/if}
      </ul>
    </div>

    <!-- Étiquettes -->
    <div class="section tags-section">
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

    <!-- Paramètres -->
    <div class="footer">
      <div class="transfer-row">
        <button class="transfer-btn" title="Archiver toutes les notes (.zip + manifeste)" onclick={() => void exportAllNotes()}>
          <Icon name="download" size={13} />
          Exporter tout
        </button>
        <button class="transfer-btn" title="Sauvegarde complète vérifiable de la base" onclick={() => void createBackup()}>
          <Icon name="archive" size={13} />
          Sauvegarde…
        </button>
      </div>
      <button class="restore-btn" title="Restaurer une sauvegarde validée" onclick={() => (restorePending = true)}>
        <Icon name="upload" size={13} />
        Restaurer une sauvegarde
      </button>

      <!-- Affichage : police, taille, largeur (réglages locaux, §3.6) -->
      <div class="display-prefs" role="group" aria-label="Affichage de l'éditeur">
        <select
          class="pref-select"
          title="Police d'écriture de l'éditeur"
          aria-label="Police d'écriture de l'éditeur"
          value={$display.font}
          onchange={(e) =>
            setDisplayPref('font', (e.currentTarget.value as 'serif' | 'sans' | 'mono'))}
        >
          <option value="serif">Serif</option>
          <option value="sans">Sans</option>
          <option value="mono">Mono</option>
        </select>
        <select
          class="pref-select"
          title="Taille du texte de l'éditeur"
          aria-label="Taille du texte de l'éditeur"
          value={$display.size}
          onchange={(e) =>
            setDisplayPref('size', (e.currentTarget.value as 'compact' | 'normal' | 'large'))}
        >
          <option value="compact">Petite</option>
          <option value="normal">Normale</option>
          <option value="large">Grande</option>
        </select>
        <select
          class="pref-select"
          title="Largeur de la colonne d'écriture"
          aria-label="Largeur de la colonne d'écriture"
          value={$display.width}
          onchange={(e) =>
            setDisplayPref('width', (e.currentTarget.value as 'narrow' | 'normal' | 'wide'))}
        >
          <option value="narrow">Étroite</option>
          <option value="normal">Moyenne</option>
          <option value="wide">Large</option>
        </select>
      </div>

      <div class="theme-toggle" role="group" aria-label="Thème">
        <button
          class="theme-btn"
          class:active={$theme.pref === 'light'}
          aria-pressed={$theme.pref === 'light'}
          title="Thème clair"
          onclick={() => setThemePref('light')}
        >
          <Icon name="sun" size={14} />
        </button>
        <button
          class="theme-btn"
          class:active={$theme.pref === 'dark'}
          aria-pressed={$theme.pref === 'dark'}
          title="Thème sombre"
          onclick={() => setThemePref('dark')}
        >
          <Icon name="moon" size={14} />
        </button>
        <button
          class="theme-btn"
          class:active={$theme.pref === 'system'}
          aria-pressed={$theme.pref === 'system'}
          title="Conformément au système"
          onclick={() => setThemePref('system')}
        >
          <Icon name="monitor" size={14} />
        </button>
      </div>
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

{#if restorePending}
  <ConfirmDialog
    title="Restaurer cette sauvegarde ?"
    message="Les données actuelles seront remplacées par la sauvegarde validée. Cette action ne peut pas être annulée."
    confirmLabel="Choisir une sauvegarde"
    onconfirm={() => { restorePending = false; void restoreBackup(); }}
    oncancel={() => (restorePending = false)}
  />
{/if}

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    background: var(--sidebar-bg);
    border-right: 1px solid var(--border);
    overflow-y: auto;
    min-height: 0;
  }

  .loading {
    padding: 1.2rem;
    color: var(--text-soft);
    font-size: 0.85rem;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 1rem 1.1rem 0.8rem;
  }
  .brand img {
    flex-shrink: 0;
  }

  /* Sur fond clair : trait noir intégral ; en thème sombre, inversion. */
  .brand-logo {
    height: 40px;
    width: auto;
  }
  :global([data-theme='dark']) .brand-logo {
    filter: invert(1);
    opacity: 0.92;
  }

  .new-note {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.45rem;
    margin: 0.4rem 1rem 0.9rem;
    padding: 0.55rem 0.9rem;
    border: none;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-size: 0.88rem;
    font-weight: 550;
    cursor: pointer;
    transition: background 0.15s ease;
  }
  .new-note:hover {
    background: var(--accent-strong);
  }

  .views,
  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin: 0 0.6rem;
    padding: 0;
    list-style: none;
  }

  .view {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    width: 100%;
    padding: 0.45rem 0.6rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.86rem;
    cursor: pointer;
    text-align: left;
    transition: background 0.12s ease, color 0.12s ease;
  }
  .view:hover {
    background: var(--hover);
    color: var(--text);
  }
  .view.active {
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 550;
  }

  .view-label {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .count {
    font-size: 0.7rem;
    color: var(--text-soft);
    background: var(--chip-bg);
    border-radius: 999px;
    padding: 0.05rem 0.45rem;
  }

  .section {
    margin-top: 1.1rem;
    min-width: 0;
  }

  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 1.4rem 0.35rem;
  }

  .section-title {
    font-size: 0.68rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-faint);
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

  li {
    position: relative;
  }

  .row-trash {
    position: absolute;
    top: 50%;
    right: 0.45rem;
    transform: translateY(-50%);
    display: none;
    place-items: center;
    width: 24px;
    height: 24px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  li:hover .row-trash {
    display: grid;
  }
  .row-trash:hover {
    background: var(--danger-soft);
    color: var(--danger);
  }

  .folder-input {
    margin: 0.2rem 0.6rem 0.4rem;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    width: calc(100% - 1.2rem);
    outline: none;
  }

  .folder-rename-input {
    margin: 0.1rem 2rem 0.1rem 0.6rem;
    width: calc(100% - 2.6rem);
  }

  .list-empty {
    padding: 0.15rem 0.85rem;
    font-size: 0.8rem;
    color: var(--text-faint);
  }

  .tags-section {
    flex: 1;
  }

  .tag-cloud {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    padding: 0 1.1rem;
  }

  .tag-chip {
    border: 1px solid var(--tint-line);
    border-radius: 999px;
    background: var(--tint-bg);
    color: var(--tint-ink);
    font: inherit;
    font-size: 0.76rem;
    padding: 0.22rem 0.7rem;
    cursor: pointer;
    transition: box-shadow 0.12s ease, border-color 0.12s ease;
  }
  .tag-chip:hover {
    border-color: var(--tint-ink);
  }
  .tag-chip.active {
    background: var(--tint-ink);
    border-color: var(--tint-ink);
    color: var(--tint-bg);
    font-weight: 550;
  }

  .footer {
    margin-top: auto;
    padding: 0.8rem 1rem;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 0.55rem;
  }

  /* Export multi-notes et sauvegarde (cdc.md §3.7). */
  .transfer-row {
    display: flex;
    gap: 0.4rem;
  }
  .transfer-btn {
    flex: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    padding: 0.4rem 0.3rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.76rem;
    cursor: pointer;
    white-space: nowrap;
    transition: background 0.12s ease, color 0.12s ease;
  }
  .transfer-btn:hover {
    background: var(--hover);
    color: var(--text);
  }

  .restore-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    padding: 0.4rem 0.3rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.76rem;
    cursor: pointer;
  }
  .restore-btn:hover {
    background: var(--hover);
    color: var(--text);
  }

  .theme-toggle {
    display: flex;
    gap: 2px;
    background: var(--chip-bg);
    border-radius: var(--radius);
    padding: 3px;
  }

  .theme-btn {
    flex: 1;
    display: grid;
    place-items: center;
    padding: 0.35rem;
    border: none;
    border-radius: calc(var(--radius) - 3px);
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .theme-btn:hover {
    color: var(--text);
  }
  .theme-btn.active {
    background: var(--surface);
    color: var(--accent-text);
    box-shadow: var(--shadow-sm);
  }

  /* Préférences d'affichage : trois sélecteurs compacts (§3.6). */
  .display-prefs {
    display: flex;
    gap: 0.35rem;
  }

  .pref-select {
    flex: 1;
    min-width: 0;
    padding: 0.32rem 0.4rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-soft);
    font: inherit;
    font-size: 0.74rem;
    cursor: pointer;
  }
  .pref-select:hover {
    color: var(--text);
    border-color: var(--accent);
  }
</style>
