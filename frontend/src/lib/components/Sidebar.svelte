<script lang="ts">
  /**
   * Zone A — Barre latérale (cahier des charges §4.2)
   * Identité, création de note, recherche (palette), vues, dossiers (suppression
   * sans perte de notes, §3.2), étiquettes. Les réglages et les opérations sur
   * les données vivent dans la fenêtre Paramètres.
   */
  import Icon from './Icon.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import NoteTemplates from './NoteTemplates.svelte';
  import Brand from './Brand.svelte';
  import { tagTint } from '../utils/tags';
  import {
    allTags,
    createDailyNote,
    createFolder,
    createNote,
    expandedFolders,
    folderTree,
    moveFolder,
    moveNotesTo,
    toggleFolder,
    currentView,
    deleteFolder,
    folderError,
    folders,
    goFiles,
    goHome,
    goTrash,
    loaded,
    notes,
    renameFolder,
    restoreBackup,
    restorePreview,
    selectedNoteId,
    trashOpen,
    trashedNotes
  } from '../stores/workspace';
  import { dragging, folderPicker, paletteOpen, settingsOpen, sidebarCollapsed, templateDialog } from '../stores/ui';
  import { canDropFolder, subtreeIds, visibleNodes } from '../utils/folders';
  import { reviewStats, startReview } from '../stores/review';
  import { filesOpen } from '../stores/files';
  import { theme, setThemePref } from '../stores/appTheme';
  import { formatFullDate } from '../utils/format';
  import type { RestorePreview, View } from '../api/types';

  let creatingFolder = $state(false);
  /** Dossier dans lequel on crée (null = racine). */
  let creatingParent = $state<string | null>(null);
  let newFolderName = $state('');
  let folderPendingDelete = $state<string | null>(null);
  let editingFolderId = $state<string | null>(null);
  let folderEditName = $state('');
  let noteTemplatesOpen = $state(false);
  /** Menus déroulants : création (＋) et données (Fichiers/Corbeille). */
  let createMenuOpen = $state(false);
  let dataMenuOpen = $state(false);

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

  function startCreateFolder(parentId: string | null = null) {
    creatingParent = parentId;
    if (parentId) expandedFolders.update((set) => new Set(set).add(parentId));
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
    if (await createFolder(name, creatingParent)) {
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

  const visible = $derived(visibleNodes($folderTree, $expandedFolders));
  let dropTarget = $state<string | 'root' | null>(null);
  let hoverTimer: ReturnType<typeof setTimeout> | null = null;

  /** Un dépôt est-il possible sur cette cible avec ce qui est glissé ? */
  function acceptsDrop(target: string | null): boolean {
    const d = $dragging;
    if (!d) return false;
    if (d.kind === 'notes') return true;
    return canDropFolder($folders, d.ids[0], target);
  }

  function onDragOver(e: DragEvent, target: string | null): void {
    if (!acceptsDrop(target)) return;
    e.preventDefault(); // autorise le dépôt
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    dropTarget = target ?? 'root';
  }

  function onDragEnterFolder(id: string): void {
    // Survoler un dossier replié un instant le déplie : on peut viser un sous-dossier.
    if (hoverTimer) clearTimeout(hoverTimer);
    hoverTimer = setTimeout(() => {
      if ($dragging && !$expandedFolders.has(id)) toggleFolder(id);
    }, 650);
  }

  async function onDrop(e: DragEvent, target: string | null): Promise<void> {
    e.preventDefault();
    const d = $dragging;
    dropTarget = null;
    dragging.set(null);
    if (hoverTimer) clearTimeout(hoverTimer);
    if (!d || !acceptsDrop(target)) return;
    if (d.kind === 'notes') await moveNotesTo(d.ids, target);
    else await moveFolder(d.ids[0], target);
  }

  function startDragFolder(e: DragEvent, id: string): void {
    e.dataTransfer?.setData('application/x-notecore-folder', id);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
    dragging.set({ kind: 'folder', ids: [id] });
  }

  function endDrag(): void {
    dragging.set(null);
    dropTarget = null;
    if (hoverTimer) clearTimeout(hoverTimer);
  }

  /** Déplacer un dossier sans glisser-déposer : sélecteur de destination. */
  function pickDestination(id: string, name: string): void {
    const blocked = [...subtreeIds($folders, id)];
    const current = $folders.find((f) => f.id === id)?.parentId ?? null;
    folderPicker.set({
      title: `Déplacer « ${name} » dans…`,
      exclude: blocked,
      current,
      rootLabel: 'Racine (aucun dossier parent)',
      onpick: (target) => void moveFolder(id, target)
    });
  }

  /** Vue « Notes » : toutes les listes de notes, filtrées dans la liste. */
  function goNotes(): void {
    currentView.set({ kind: 'all' });
  }

  function toggleTheme(): void {
    setThemePref($theme.theme === 'dark' ? 'light' : 'dark');
  }
</script>

<aside class="sidebar">
  {#if $sidebarCollapsed}
    <!-- Rail : les actions essentielles restent à portée, le reste de
         l'espace revient à l'éditeur. Ctrl+B ou le chevron réaffiche. -->
    <div class="rail">
      <button
        class="rail-btn"
        title="Déplier la barre latérale — Ctrl+B"
        aria-label="Déplier la barre latérale"
        onclick={() => sidebarCollapsed.set(false)}
      >
        <Icon name="chevron-right" size={16} />
      </button>
      <button
        class="rail-btn"
        title="Nouvelle note — Ctrl+N"
        aria-label="Nouvelle note"
        onclick={() => void createNote($currentView.kind === 'folder' ? $currentView.folderId : null)}
      >
        <Icon name="plus" size={16} stroke={2.2} />
      </button>
      <button class="rail-btn" title="Rechercher — Ctrl+K" aria-label="Rechercher" onclick={() => paletteOpen.set(true)}>
        <Icon name="search" size={16} />
      </button>
      <button class="rail-btn" title="Réviser les cartes dues" aria-label="Réviser" onclick={() => void startReview()}>
        <Icon name="layers" size={16} />
      </button>
      <button
        class="rail-btn"
        class:active={$filesOpen || $trashOpen}
        title="Données — fichiers et corbeille"
        aria-label="Données"
        aria-haspopup="menu"
        aria-expanded={dataMenuOpen}
        onclick={() => (dataMenuOpen = !dataMenuOpen)}
      >
        <Icon name="database" size={16} />
      </button>
      {#if dataMenuOpen}
        <div class="menu-backdrop" role="presentation" onclick={() => (dataMenuOpen = false)}></div>
        <div class="menu menu-rail" role="menu">
          <button role="menuitem" class:active={$filesOpen} onclick={() => { dataMenuOpen = false; void goFiles(); }}>
            <Icon name="paperclip" size={15} />
            <span>Fichiers joints</span>
          </button>
          <button role="menuitem" class:active={$trashOpen} onclick={() => { dataMenuOpen = false; void goTrash(); }}>
            <Icon name="trash" size={15} />
            <span>Corbeille</span>
            {#if $trashedNotes.length > 0}
              <span class="count">{$trashedNotes.length}</span>
            {/if}
          </button>
        </div>
      {/if}
      <span class="rail-gap"></span>
      <button class="rail-btn" title="Paramètres — Ctrl+," aria-label="Paramètres" onclick={() => settingsOpen.set(true)}>
        <Icon name="settings" size={16} />
      </button>
      <button
        class="rail-btn"
        title={$theme.theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
        aria-label="Basculer le thème"
        onclick={toggleTheme}
      >
        <Icon name={$theme.theme === 'dark' ? 'sun' : 'moon'} size={16} />
      </button>
    </div>
  {:else}
  {#if !$loaded}
    <div class="loading">Chargement…</div>
  {:else}
    <div class="brand">
      <Brand size={30} />
      <button
        class="collapse"
        title="Replier la barre latérale — Ctrl+B"
        aria-label="Replier la barre latérale"
        onclick={() => sidebarCollapsed.set(true)}
      >
        <Icon name="chevron-left" size={15} />
      </button>
    </div>

    <div class="top">
      <div class="create-wrap">
        <button
          class="new-note"
          title="Créer — Ctrl+N"
          aria-keyshortcuts="Control+N Meta+N"
          aria-haspopup="menu"
          aria-expanded={createMenuOpen}
          onclick={() => (createMenuOpen = !createMenuOpen)}
        >
          <Icon name="plus" size={15} stroke={2.2} />
          Nouvelle note
          <Icon name="chevron-down" size={13} />
        </button>
        {#if createMenuOpen}
          <div class="menu-backdrop" role="presentation" onclick={() => (createMenuOpen = false)}></div>
          <div class="menu" role="menu">
            <button
              role="menuitem"
              onclick={() => { createMenuOpen = false; void createNote($currentView.kind === 'folder' ? $currentView.folderId : null); }}
            >
              <Icon name="edit" size={15} />
              <span>Note vierge<small>Ctrl+N</small></span>
            </button>
            <button
              role="menuitem"
              onclick={() => { createMenuOpen = false; void createDailyNote(); }}
            >
              <Icon name="clock" size={15} />
              <span>Note du jour<small>Journal quotidien</small></span>
            </button>
            <button
              role="menuitem"
              onclick={() => { createMenuOpen = false; noteTemplatesOpen = true; }}
            >
              <Icon name="layout" size={15} />
              <span>À partir d'un modèle<small>Compte rendu, TP, séance…</small></span>
            </button>
          </div>
        {/if}
      </div>

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
          class:active={$selectedNoteId === null && !$filesOpen && !$trashOpen}
          aria-current={$selectedNoteId === null && !$filesOpen && !$trashOpen ? 'page' : undefined}
          onclick={() => void goHome()}
        >
          <Icon name="home" size={16} />
          <span class="nav-label">Accueil</span>
        </button>
        <button
          class="nav-item"
          class:active={['all', 'favorites', 'recent', 'inbox'].includes($currentView.kind) && !$filesOpen && !$trashOpen}
          title="Toutes les notes — les filtres (récentes, favoris, à classer) sont dans la liste"
          onclick={goNotes}
        >
          <Icon name="notes" size={16} />
          <span class="nav-label">Notes</span>
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
      </nav>

      <div class="section">
        <div
          class="section-head root-drop"
          class:drop={dropTarget === 'root'}
          role="presentation"
          ondragover={(e) => onDragOver(e, null)}
          ondragleave={() => (dropTarget = dropTarget === 'root' ? null : dropTarget)}
          ondrop={(e) => void onDrop(e, null)}
        >
          <span class="section-title">Dossiers</span>
          {#if $dragging}<span class="drop-hint">Déposer ici : à la racine</span>{/if}
          <button class="mini-btn" title="Nouvelle matière à partir d'un modèle (Cours, TD, TP…)" aria-label="Nouvelle matière à partir d'un modèle" onclick={() => templateDialog.set({ parentId: null })}>
            <Icon name="layout" size={13} />
          </button>
          <button class="mini-btn" title="Nouveau dossier" aria-label="Nouveau dossier" onclick={() => startCreateFolder(null)}>
            <Icon name="plus" size={13} stroke={2.2} />
          </button>
        </div>

        {#if creatingFolder && creatingParent === null}
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

        <ul class="list tree" role="tree" aria-label="Dossiers">
          {#each visible as node (node.folder.id)}
            {@const folder = node.folder}
            <li
              class="row"
              role="treeitem"
              aria-selected={isActive($currentView, 'folder', folder.id)}
              aria-expanded={node.children.length > 0 ? $expandedFolders.has(folder.id) : undefined}
              aria-level={node.depth + 1}
            >
              {#if editingFolderId === folder.id}
                <input
                  class="folder-input"
                  style:margin-left="{node.depth * 0.9}rem"
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
                <div
                  class="node"
                  class:drop={dropTarget === folder.id}
                  style:padding-left="{node.depth * 0.9}rem"
                  role="presentation"
                  draggable="true"
                  ondragstart={(e) => startDragFolder(e, folder.id)}
                  ondragend={endDrag}
                  ondragover={(e) => onDragOver(e, folder.id)}
                  ondragenter={() => onDragEnterFolder(folder.id)}
                  ondragleave={() => (dropTarget = dropTarget === folder.id ? null : dropTarget)}
                  ondrop={(e) => void onDrop(e, folder.id)}
                >
                  <button
                    class="twisty"
                    class:hidden={node.children.length === 0}
                    tabindex="-1"
                    aria-label={$expandedFolders.has(folder.id) ? 'Replier' : 'Déplier'}
                    onclick={() => toggleFolder(folder.id)}
                  >
                    <Icon name={$expandedFolders.has(folder.id) ? 'chevron-down' : 'chevron-right'} size={13} stroke={2.2} />
                  </button>
                  <button
                    class="nav-item folder"
                    class:current-view={isActive($currentView, 'folder', folder.id)}
                    aria-current={isActive($currentView, 'folder', folder.id) ? 'page' : undefined}
                    onclick={() => currentView.set({ kind: 'folder', folderId: folder.id })}
                    ondblclick={() => startRenameFolder(folder.id, folder.name)}
                    title={`${folder.name} — glissez des notes ici pour les ranger`}
                  >
                    <Icon name="folder" size={16} />
                    <span class="nav-label">{folder.name}</span>
                    <span class="count">{node.total}</span>
                  </button>
                  <span class="row-actions">
                    <button class="row-btn" title="Ajouter une structure (modèle) dans ce dossier" aria-label={`Ajouter une structure dans ${folder.name}`} onclick={() => templateDialog.set({ parentId: folder.id })}>
                      <Icon name="layout" size={13} />
                    </button>
                    <button class="row-btn" title="Nouveau sous-dossier" aria-label={`Nouveau sous-dossier dans ${folder.name}`} onclick={() => startCreateFolder(folder.id)}>
                      <Icon name="folder-plus" size={13} />
                    </button>
                    <button class="row-btn" title="Déplacer ce dossier…" aria-label={`Déplacer le dossier ${folder.name}`} onclick={() => pickDestination(folder.id, folder.name)}>
                      <Icon name="move" size={13} />
                    </button>
                    <button class="row-btn" title={`Renommer « ${folder.name} »`} aria-label={`Renommer le dossier ${folder.name}`} onclick={() => startRenameFolder(folder.id, folder.name)}>
                      <Icon name="edit" size={13} />
                    </button>
                    <button
                      class="row-btn danger"
                      title={`Supprimer « ${folder.name} » (notes et sous-dossiers conservés)`}
                      aria-label={`Supprimer le dossier ${folder.name}`}
                      onclick={() => (folderPendingDelete = folder.id)}
                    >
                      <Icon name="trash" size={13} />
                    </button>
                  </span>
                </div>
              {/if}

              {#if creatingFolder && creatingParent === folder.id}
                <input
                  class="folder-input"
                  style:margin-left="{(node.depth + 1) * 0.9 + 1.2}rem"
                  bind:this={folderInput}
                  bind:value={newFolderName}
                  placeholder={`Sous-dossier de ${folder.name}`}
                  aria-label={`Nom du nouveau sous-dossier de ${folder.name}`}
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
      <div class="data-wrap">
        <button
          class="footer-btn grow"
          aria-haspopup="menu"
          aria-expanded={dataMenuOpen}
          class:active={$filesOpen || $trashOpen}
          onclick={() => (dataMenuOpen = !dataMenuOpen)}
          title="Fichiers joints et corbeille"
        >
          <Icon name="database" size={16} />
          Données
          <Icon name="chevron-up" size={13} />
        </button>
        {#if dataMenuOpen}
          <div class="menu-backdrop" role="presentation" onclick={() => (dataMenuOpen = false)}></div>
          <div class="menu menu-up" role="menu">
            <button
              role="menuitem"
              class:active={$filesOpen}
              onclick={() => { dataMenuOpen = false; void goFiles(); }}
            >
              <Icon name="paperclip" size={15} />
              <span>Fichiers joints</span>
            </button>
            <button
              role="menuitem"
              class:active={$trashOpen}
              onclick={() => { dataMenuOpen = false; void goTrash(); }}
            >
              <Icon name="trash" size={15} />
              <span>Corbeille</span>
              {#if $trashedNotes.length > 0}
                <span class="count">{$trashedNotes.length}</span>
              {/if}
            </button>
          </div>
        {/if}
      </div>
      <button class="footer-btn" onclick={() => settingsOpen.set(true)} title="Paramètres — Ctrl+,">
        <Icon name="settings" size={16} />
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
  {/if}
{#if noteTemplatesOpen}
  <NoteTemplates onclose={() => (noteTemplatesOpen = false)} />
{/if}

</aside>

{#if folderPendingDelete}
  <ConfirmDialog
    title="Supprimer ce dossier ?"
    message="Rien n'est supprimé : les notes et les sous-dossiers de ce dossier remontent d'un niveau, dans le dossier qui le contient (ou à la racine)."
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
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.1rem 0.8rem 0.7rem 1.1rem;
  }
  .collapse {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-faint);
    padding: 0.3rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    display: grid;
    place-items: center;
  }
  .collapse:hover {
    background: var(--hover);
    color: var(--text);
  }

  /* Rail replié : colonne d'icônes, mêmes actions essentielles. */
  .rail {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.25rem;
    padding: 0.7rem 0.3rem;
    flex: 1;
    min-height: 0;
  }
  .rail-btn {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-soft);
    width: 34px;
    height: 34px;
    border-radius: var(--radius-sm);
    display: grid;
    place-items: center;
    cursor: pointer;
    flex-shrink: 0;
  }
  .rail-btn:hover {
    background: var(--hover);
    color: var(--text);
  }
  .rail-gap {
    flex: 1;
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

  /* « Aujourd'hui » : même emplacement, poids secondaire — la note du
     jour est un raccourci, pas l'action principale. */
  .new-note.daily {
    height: 32px;
    margin-top: 0.4rem;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 550;
    box-shadow: none;
  }
  .new-note.daily:hover {
    background: var(--accent);
    color: var(--on-accent);
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
    overflow-x: hidden;
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

  .section {
    margin-top: 1.1rem;
  }
  .section-head {
    display: flex;
    align-items: center;
    gap: 0.1rem;
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
  .row {
    display: block;
  }
  .node {
    position: relative;
    display: flex;
    align-items: center;
    min-width: 0;
    border-radius: var(--radius-sm);
  }
  .node.drop,
  .root-drop.drop {
    background: var(--accent-soft);
    outline: 2px dashed var(--accent);
    outline-offset: -2px;
  }
  .node .nav-item {
    flex: 1;
    min-width: 0;
    padding-right: 7.4rem;
  }
  .twisty {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    width: 18px;
    height: 24px;
    padding: 0;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .twisty:hover {
    background: var(--hover);
    color: var(--text);
  }
  .twisty.hidden {
    visibility: hidden;
  }
  .root-drop {
    border-radius: var(--radius-sm);
  }
  .drop-hint {
    flex: 1;
    margin-left: 0.4rem;
    color: var(--accent-text);
    font-size: 0.7rem;
  }
  .count.todo {
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 650;
  }
  .row-actions {
    position: absolute;
    right: 0.25rem;
    display: none;
    gap: 1px;
  }
  .node:hover .row-actions,
  .node:focus-within .row-actions {
    display: inline-flex;
  }
  .node:hover .count,
  .node:focus-within .count {
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
  .footer-btn.active {
    color: var(--accent-text);
    background: var(--accent-soft);
  }

  /* --- menus déroulants (création, données) ---------------------------- */
  .create-wrap,
  .data-wrap {
    position: relative;
    flex: 1;
    min-width: 0;
  }
  .create-wrap .new-note {
    width: 100%;
  }
  .create-wrap .new-note :global(svg:last-child) {
    margin-left: auto;
    opacity: 0.75;
  }
  .menu-backdrop {
    position: fixed;
    inset: 0;
    z-index: 40;
  }
  .menu {
    position: absolute;
    z-index: 41;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow, 0 4px 16px rgb(0 0 0 / 0.14));
  }
  .menu.menu-up {
    top: auto;
    bottom: calc(100% + 4px);
  }
  /* Menu ouvert depuis le rail : il sort à droite de la barre. */
  .menu.menu-rail {
    top: auto;
    bottom: 0;
    left: calc(100% + 6px);
    right: auto;
    min-width: 11rem;
  }
  .menu button {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.45rem 0.55rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.84rem;
    text-align: left;
    cursor: pointer;
  }
  .menu button:hover {
    background: var(--hover);
  }
  .menu button.active {
    color: var(--accent-text);
    background: var(--accent-soft);
  }
  .menu button span {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .menu button small {
    color: var(--text-faint);
    font-size: 0.7rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .menu .count {
    margin-left: auto;
    font-size: 0.72rem;
    color: var(--text-faint);
  }
</style>
