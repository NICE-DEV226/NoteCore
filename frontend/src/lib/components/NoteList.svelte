<script lang="ts">
  /**
   * Zone B — Liste des notes (cahier des charges §4.2)
   * Recherche (titre, contenu, étiquettes — §3.3), filtres, état vide,
   * extraits et dates de modification (§4.2).
   */
  import Icon from './Icon.svelte';
  import EmptyState from './EmptyState.svelte';
  import { derived, get } from 'svelte/store';
  import { dragging, folderPicker } from '../stores/ui';
  import { folderPath, pathLabel } from '../utils/folders';
  import type { Note } from '../api/types';
  import { excerpt, formatRelative, snippetParts } from '../utils/format';
  import { tagTint } from '../utils/tags';
  import { SEARCH_INPUT_ID } from '../constants';
  import {
    acceptAllConfident,
    acceptSuggestion,
    clearSelection,
    inboxSuggestions,
    moveNotesTo,
    selectedIds,
    createNote,
    currentView,
    filteredNotes,
    folders,
    hasAnyNote,
    remoteSearchHits,
    searchPending,
    searchQuery,
    selectNote,
    selectedNoteId,
    togglePin
  } from '../stores/workspace';

  /** Titre lisible de la vue active (en-tête de section de la liste). */
  const viewTitle = derived([currentView, folders], ([$view, $folders]) => {
    switch ($view.kind) {
      case 'all':
        return 'Toutes les notes';
      case 'favorites':
        return 'Favoris';
      case 'recent':
        return 'Récentes';
      case 'folder':
        return pathLabel(folderPath($folders, $view.folderId)) || 'Dossier';
      case 'inbox':
        return 'À classer';
      case 'tag':
        return `#${$view.tag}`;
      default:
        return 'Notes';
    }
  });

  /**
   * État vide propre à la vue active (cahier des charges §4.3) : un
   * dossier vide, une liste de favoris sans étoile et une recherche sans
   * résultat ne sont pas la même situation et ne se disent pas pareil.
   */
  const emptyView = derived([currentView, folders], ([$view, $folders]) => {
    switch ($view.kind) {
      case 'favorites':
        return {
          icon: 'notes' as const,
          title: 'Aucun favori',
          message: 'Ajoutez une étoile à une note pour la retrouver ici.'
        };
      case 'recent':
        return {
          icon: 'notes' as const,
          title: 'Aucune note récente',
          message: 'Vos notes les plus récemment modifiées apparaîtront ici.'
        };
      case 'folder': {
        const name = $folders.find((f) => f.id === $view.folderId)?.name ?? 'Ce dossier';
        return {
          icon: 'notes' as const,
          title: `« ${name} » est vide`,
          message: 'Créez une note dans ce dossier pour le remplir.'
        };
      }
      case 'inbox':
        return {
          icon: 'notes' as const,
          title: 'Tout est rangé',
          message: 'Les notes sans dossier apparaîtront ici, avec des propositions de rangement.'
        };
      case 'tag':
        return {
          icon: 'notes' as const,
          title: `Aucune note #${$view.tag}`,
          message: 'Les notes portant cette étiquette apparaîtront ici.'
        };
      default:
        return {
          icon: 'notes' as const,
          title: 'Aucune note',
          message: 'Créez une note pour commencer.'
        };
    }
  });

  // --- sélection multiple et glisser-déposer -----------------------------------
  let anchorId: string | null = null;

  /** Ordre d'affichage de toutes les notes de la liste (pour la sélection par plage). */
  const orderedIds = $derived($filteredNotes.map((n) => n.id));

  function onItemClick(e: MouseEvent, id: string): void {
    if (e.ctrlKey || e.metaKey) {
      e.preventDefault();
      selectedIds.update((set) => {
        const next = new Set(set);
        if (!next.delete(id)) next.add(id);
        return next;
      });
      anchorId = id;
      return;
    }
    if (e.shiftKey && anchorId) {
      e.preventDefault();
      const a = orderedIds.indexOf(anchorId);
      const b = orderedIds.indexOf(id);
      if (a >= 0 && b >= 0) {
        const [from, to] = a < b ? [a, b] : [b, a];
        selectedIds.set(new Set(orderedIds.slice(from, to + 1)));
      }
      return;
    }
    anchorId = id;
    clearSelection();
    void selectNote(id);
  }

  function onItemDragStart(e: DragEvent, id: string): void {
    // Glisser une note cochée emporte toute la sélection ; sinon, la note seule.
    const ids = get(selectedIds).has(id) ? [...get(selectedIds)] : [id];
    e.dataTransfer?.setData('application/x-notecore-notes', JSON.stringify(ids));
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
    dragging.set({ kind: 'notes', ids });
  }

  function moveSelectionTo(): void {
    const ids = [...$selectedIds];
    folderPicker.set({
      title: `Ranger ${ids.length} note${ids.length > 1 ? 's' : ''} dans…`,
      exclude: [],
      current: null,
      rootLabel: 'Sans dossier (racine)',
      onpick: (target) => void moveNotesTo(ids, target)
    });
  }

  // --- boîte « À classer » : propositions de dossier ---------------------------
  const confidentCount = $derived(
    [...$inboxSuggestions.values()].filter((l) => l[0]?.confidence === 'high').length
  );
  let tidying = $state(false);
  let tidyMessage = $state('');

  async function tidyAll(): Promise<void> {
    tidying = true;
    tidyMessage = '';
    const n = await acceptAllConfident();
    tidyMessage = n > 0 ? `${n} note${n > 1 ? 's' : ''} rangée${n > 1 ? 's' : ''}. Les autres attendent votre décision.` : '';
    tidying = false;
    setTimeout(() => (tidyMessage = ''), 6000);
  }

  const confidenceLabel = { high: 'Très probable', medium: 'Probable', low: 'Possible' } as const;

  /** Création depuis un état vide : la note rejoint la vue active. */
  function createNoteHere(): void {
    const view = $currentView;
    void createNote(view.kind === 'folder' ? view.folderId : null);
  }

  /** Regroupe les notes (déjà triées : épinglées, puis récentes) par période. */
  interface Group {
    label: string;
    notes: Note[];
  }

  function periodLabel(iso: string, now: Date): string {
    const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
    const diffDays = Math.round((day(now) - day(new Date(iso))) / 86_400_000);
    if (diffDays <= 0) return "Aujourd'hui";
    if (diffDays === 1) return 'Hier';
    if (diffDays < 7) return 'Cette semaine';
    if (diffDays < 31) return 'Ce mois-ci';
    return 'Plus ancien';
  }

  function groupNotes(list: Note[], searching: boolean): Group[] {
    if (searching) return [{ label: '', notes: list }];
    const groups: Group[] = [];
    const now = new Date();
    for (const note of list) {
      const label = note.isPinned ? 'Épinglées' : periodLabel(note.updatedAt, now);
      const last = groups[groups.length - 1];
      if (last && last.label === label) last.notes.push(note);
      else groups.push({ label, notes: [note] });
    }
    return groups;
  }

  const groups = $derived(
    groupNotes($filteredNotes, $searchQuery.trim().length > 0 || $currentView.kind === 'recent')
  );
</script>

<section class="note-list">
  <header class="list-head">
    <h2>{$viewTitle}</h2>
    <span class="list-count">{$filteredNotes.length}</span>
    <button class="add" title="Nouvelle note — Ctrl+N" aria-label="Nouvelle note" onclick={createNoteHere}>
      <Icon name="plus" size={15} stroke={2.2} />
    </button>
  </header>

  <div class="search">
    <Icon name="search" size={14} />
    <input
      id={SEARCH_INPUT_ID}
      type="search"
      placeholder="Filtrer cette liste…"
      bind:value={$searchQuery}
      aria-label="Rechercher dans cette liste"
    />
    {#if $searchQuery}
      <button class="clear" title="Effacer la recherche" aria-label="Effacer la recherche" onclick={() => searchQuery.set('')}>
        <Icon name="x" size={13} />
      </button>
    {/if}
  </div>

  {#if $selectedIds.size > 0}
    <div class="selbar" role="status">
      <strong>{$selectedIds.size}</strong> sélectionnée{$selectedIds.size > 1 ? 's' : ''}
      <span class="grow"></span>
      <button onclick={moveSelectionTo}><Icon name="move" size={13} /> Ranger dans…</button>
      <button class="plain" onclick={clearSelection}>Annuler</button>
    </div>
  {:else if $currentView.kind === 'inbox' && $filteredNotes.length > 0}
    <div class="tidy">
      <p>
        <Icon name="sparkles" size={14} />
        {#if confidentCount > 0}
          <strong>{confidentCount}</strong> note{confidentCount > 1 ? 's' : ''} {confidentCount > 1 ? 'ont' : 'a'} un dossier évident.
        {:else}
          Aucune proposition évidente : choisissez note par note.
        {/if}
      </p>
      {#if confidentCount > 0}
        <button disabled={tidying} onclick={() => void tidyAll()}>Ranger les {confidentCount} sûre{confidentCount > 1 ? 's' : ''}</button>
      {/if}
      {#if tidyMessage}<small>{tidyMessage}</small>{/if}
    </div>
  {:else if $filteredNotes.length > 1 && $currentView.kind !== 'recent'}
    <p class="tip">Astuce : Ctrl+clic pour choisir plusieurs notes, puis glissez-les dans un dossier.</p>
  {/if}

  <div class="list-inner">
    {#if $hasAnyNote && $searchPending}
      <EmptyState
        icon="search"
        title="Recherche…"
        message="Le moteur de recherche parcourt le titre, le contenu et les étiquettes de vos notes."
      />
    {:else if !$hasAnyNote}
      <EmptyState
        icon="notes"
        title="Aucune note créée"
        message="Créez votre première note pour commencer à organiser vos cours et vos projets."
        actionLabel="Créer une note"
        onaction={createNoteHere}
      />
    {:else if $filteredNotes.length === 0}
      {#if $searchQuery.trim()}
        <EmptyState
          icon="search"
          title="Aucun résultat"
          message={`Aucune note ne correspond à « ${$searchQuery.trim()} ». Vérifiez l'orthographe ou essayez d'autres mots.`}
          actionLabel="Effacer la recherche"
          onaction={() => searchQuery.set('')}
        />
      {:else}
        <EmptyState
          icon={$emptyView.icon}
          title={$emptyView.title}
          message={$emptyView.message}
          actionLabel="Créer une note"
          onaction={createNoteHere}
        />
      {/if}
    {:else}
      {#each groups as group (group.label)}
        <section class="group">
          {#if group.label}
            <h3 class="group-label">{group.label}</h3>
          {/if}
          <ul>
            {#each group.notes as note (note.id)}
              {@const hit = $remoteSearchHits?.get(note.id) ?? null}
              <li
                class="item-row"
                class:active={$selectedNoteId === note.id}
                class:picked={$selectedIds.has(note.id)}
                draggable="true"
                ondragstart={(e) => onItemDragStart(e, note.id)}
                ondragend={() => dragging.set(null)}
              >
                <button
                  class="item"
                  aria-current={$selectedNoteId === note.id ? 'true' : undefined}
                  aria-pressed={$selectedIds.has(note.id) ? 'true' : undefined}
                  onclick={(e) => onItemClick(e, note.id)}
                >
                  <span class="title">{note.title || 'Sans titre'}</span>
                  {#if hit}
                    <span class="excerpt">
                      {#each snippetParts(hit.excerpt) as part}{#if part.hit}<mark>{part.text}</mark
                          >{:else}{part.text}{/if}{/each}
                    </span>
                  {:else}
                    <span class="excerpt">{excerpt(note.content, 140) || 'Note vide'}</span>
                  {/if}
                  <span class="meta">
                    <span class="when">{formatRelative(note.updatedAt)}</span>
                    {#if (note.tags ?? []).length > 0}
                      <span class="item-tags">
                        {#each (note.tags ?? []).slice(0, 2) as tag (tag)}
                          <span class="mini-tag tint-{tagTint(tag)}">{tag}</span>
                        {/each}
                        {#if (note.tags ?? []).length > 2}
                          <span class="mini-more">+{(note.tags ?? []).length - 2}</span>
                        {/if}
                      </span>
                    {/if}
                  </span>
                </button>
                <button
                  class="pin"
                  class:pinned={note.isPinned}
                  aria-pressed={note.isPinned}
                  title={note.isPinned ? 'Retirer des favoris' : 'Ajouter aux favoris'}
                  onclick={() => void togglePin(note.id)}
                >
                  <Icon name="star" size={14} filled={note.isPinned} />
                </button>
                {#if $currentView.kind === 'inbox'}
                  {@const top = $inboxSuggestions.get(note.id)?.[0]}
                  <div class="suggest" class:none={!top}>
                    {#if top}
                      <span class="why" title={[top.tags.length ? `Étiquettes en commun : ${top.tags.join(', ')}` : '', top.terms.length ? `Mots en commun : ${top.terms.join(', ')}` : ''].filter(Boolean).join(' — ') || 'Vocabulaire proche des notes de ce dossier'}>
                        <Icon name="sparkles" size={12} />
                        <span class="path">{pathLabel(top.path)}</span>
                        <em class={top.confidence}>{confidenceLabel[top.confidence]}</em>
                      </span>
                      <button class="accept" onclick={() => void acceptSuggestion(note.id, top.folderId)}>Ranger</button>
                    {:else}
                      <span class="why muted">Pas de proposition : trop peu de texte ou aucun dossier proche.</span>
                    {/if}
                  </div>
                {/if}
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    {/if}
  </div>
</section>

<style>
  .note-list {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    border-right: 1px solid var(--border);
    background: var(--surface);
  }

  .list-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 1rem 0.9rem 0.5rem 1.1rem;
  }
  .list-head h2 {
    margin: 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 1.05rem;
    font-weight: 700;
    letter-spacing: -0.015em;
    color: var(--text);
  }
  .list-count {
    flex: 1;
    color: var(--text-faint);
    font-size: 0.8rem;
    font-variant-numeric: tabular-nums;
  }
  .add {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: none;
    border-radius: var(--radius-sm);
    background: var(--accent-soft);
    color: var(--accent-text);
    cursor: pointer;
    transition: background 0.12s var(--ease);
  }
  .add:hover {
    background: var(--accent);
    color: var(--on-accent);
  }

  .search {
    position: relative;
    display: flex;
    align-items: center;
    gap: 0.45rem;
    margin: 0 0.8rem 0.4rem;
    padding: 0 0.6rem;
    height: 32px;
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text-faint);
    border: 1px solid transparent;
    transition: border-color 0.12s var(--ease);
  }
  .search:focus-within {
    border-color: var(--accent);
    background: var(--surface);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
  }
  .search input::-webkit-search-cancel-button {
    display: none;
  }
  .clear {
    display: grid;
    place-items: center;
    width: 20px;
    height: 20px;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .clear:hover {
    background: var(--hover);
    color: var(--text);
  }

  .list-inner {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0 0.5rem 1rem;
  }

  .group-label {
    margin: 0.9rem 0.6rem 0.25rem;
    color: var(--text-faint);
    font-size: 0.68rem;
    font-weight: 650;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 1px;
  }

  .item-row {
    position: relative;
    border-radius: var(--radius);
    transition: background 0.1s var(--ease);
  }
  .item-row:hover {
    background: var(--hover);
  }
  .item-row.active {
    background: var(--accent-soft);
  }
  .item-row.active::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0.7rem;
    bottom: 0.7rem;
    width: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--accent);
  }

  .item {
    display: grid;
    gap: 0.2rem;
    width: 100%;
    padding: var(--item-pad) 2.2rem var(--item-pad) 0.85rem;
    border: none;
    border-radius: var(--radius);
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text);
  }
  .excerpt {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    color: var(--text-soft);
    font-size: 0.8rem;
    line-height: 1.4;
  }
  .excerpt :global(mark) {
    background: var(--mark-bg);
    color: inherit;
    border-radius: 3px;
    padding: 0 0.1em;
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.1rem;
    color: var(--text-faint);
    font-size: 0.72rem;
  }
  .item-tags {
    display: inline-flex;
    gap: 0.25rem;
    min-width: 0;
  }
  .mini-tag {
    padding: 0 0.45rem;
    border-radius: 999px;
    background: var(--tint-bg);
    color: var(--tint-ink);
    font-size: 0.68rem;
    line-height: 1.5;
    white-space: nowrap;
  }
  .mini-more {
    color: var(--text-faint);
  }

  .item-row.picked {
    background: var(--accent-soft);
    box-shadow: inset 0 0 0 1px var(--accent);
  }
  .item-row[draggable='true'] {
    cursor: grab;
  }
  .selbar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0 0.8rem 0.4rem;
    padding: 0.35rem 0.5rem 0.35rem 0.7rem;
    border-radius: var(--radius);
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 0.8rem;
  }
  .selbar .grow {
    flex: 1;
  }
  .selbar button {
    white-space: nowrap;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.2rem 0.6rem;
    border: none;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    cursor: pointer;
  }
  .selbar button.plain {
    background: transparent;
    color: var(--accent-text);
  }
  .tip {
    margin: 0 1rem 0.3rem;
    color: var(--text-faint);
    font-size: 0.72rem;
  }
  .tidy {
    display: grid;
    gap: 0.4rem;
    margin: 0 0.8rem 0.5rem;
    padding: 0.6rem 0.7rem;
    border: 1px dashed var(--accent);
    border-radius: var(--radius);
    background: var(--accent-soft);
    font-size: 0.8rem;
  }
  .tidy p {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0;
    color: var(--text);
  }
  .tidy :global(svg) {
    color: var(--accent-text);
  }
  .tidy button {
    justify-self: start;
    padding: 0.25rem 0.8rem;
    border: none;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    cursor: pointer;
  }
  .tidy button:disabled {
    opacity: 0.6;
  }
  .tidy small {
    color: var(--text-soft);
  }
  .suggest {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    margin: -0.15rem 0.6rem 0.5rem 0.85rem;
    padding: 0.3rem 0.4rem 0.3rem 0.5rem;
    border-radius: var(--radius-sm);
    background: var(--surface);
    border: 1px solid var(--border);
    font-size: 0.74rem;
  }
  .suggest.none {
    background: transparent;
    border-style: dashed;
  }
  .why {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-width: 0;
    color: var(--accent-text);
  }
  .why.muted {
    color: var(--text-faint);
  }
  .why .path {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
    font-weight: 600;
  }
  .why em {
    flex-shrink: 0;
    padding: 0 0.4rem;
    border-radius: 999px;
    font-style: normal;
    font-size: 0.66rem;
    font-weight: 650;
    background: var(--chip-bg);
    color: var(--text-soft);
  }
  .why em.high {
    background: var(--accent);
    color: var(--on-accent);
  }
  .why em.medium {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .accept {
    flex-shrink: 0;
    padding: 0.15rem 0.6rem;
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--accent-text);
    font: inherit;
    font-size: 0.74rem;
    font-weight: 600;
    cursor: pointer;
  }
  .accept:hover {
    background: var(--accent);
    color: var(--on-accent);
  }
  .pin {
    position: absolute;
    top: 0.5rem;
    right: 0.45rem;
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text-faint);
    opacity: 0;
    cursor: pointer;
    transition: opacity 0.1s var(--ease);
  }
  .item-row:hover .pin,
  .pin:focus-visible,
  .pin.pinned {
    opacity: 1;
  }
  .pin:hover {
    background: var(--chip-bg);
    color: var(--text);
  }
  .pin.pinned {
    color: var(--warning);
  }
</style>
