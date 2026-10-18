<script lang="ts">
  /**
   * Zone B — Liste des notes (cahier des charges §4.2)
   * Recherche (titre, contenu, étiquettes — §3.3), filtres, état vide,
   * extraits et dates de modification (§4.2).
   */
  import Icon from './Icon.svelte';
  import EmptyState from './EmptyState.svelte';
  import { derived } from 'svelte/store';
  import { excerpt, formatRelative, snippetParts } from '../utils/format';
  import { tagTint } from '../utils/tags';
  import { SEARCH_INPUT_ID } from '../constants';
  import {
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
        return $folders.find((f) => f.id === $view.folderId)?.name ?? 'Dossier';
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

  /** Création depuis un état vide : la note rejoint la vue active. */
  function createNoteHere(): void {
    const view = $currentView;
    void createNote(view.kind === 'folder' ? view.folderId : null);
  }
</script>

<section class="note-list">
  <!-- En-tête de section : la vue active et le nombre de notes -->
  <header class="list-head">
    <h2>{$viewTitle}</h2>
    <span class="list-count">{$filteredNotes.length}</span>
  </header>

  <!-- Recherche de la zone B -->
  <div class="search">
    <Icon name="search" size={15} />
    <input
      id={SEARCH_INPUT_ID}
      type="search"
      placeholder="Rechercher (titre, contenu, étiquette)…"
      title="Rechercher — Ctrl+K"
      bind:value={$searchQuery}
      aria-label="Rechercher une note"
      aria-keyshortcuts="Control+K Meta+K"
    />
    {#if $searchQuery}
      <button class="clear" title="Effacer la recherche" onclick={() => searchQuery.set('')}>
        <Icon name="x" size={13} />
      </button>
    {/if}
  </div>

  <div class="list-inner">
    {#if $hasAnyNote && $searchPending}
      <!-- Réponse du moteur de recherche encore attendue : annoncer
           « aucun résultat » ici serait faux (§3.3, §4.3). -->
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
      <ul>
        {#each $filteredNotes as note (note.id)}
          {@const hit = $remoteSearchHits?.get(note.id) ?? null}
          <li class="item-row" class:active={$selectedNoteId === note.id}>
            <button
              class="item"
              aria-current={$selectedNoteId === note.id ? 'true' : undefined}
              onclick={() => void selectNote(note.id)}
            >
              <span class="title">{note.title || 'Sans titre'}</span>
              {#if hit}
                <!-- Extrait de recherche du backend (§3.3) : termes surlignés. -->
                <span class="excerpt">
                  {#each snippetParts(hit.excerpt) as part}{#if part.hit}<mark>{part.text}</mark
                      >{:else}{part.text}{/if}{/each}
                </span>
              {:else}
                <span class="excerpt">{excerpt(note.content) || 'Note vide'}</span>
              {/if}
              <span class="meta">
                <span>{formatRelative(note.updatedAt)}</span>
                {#if (note.tags ?? []).length > 0}
                  <span class="item-tags">
                    {#each (note.tags ?? []).slice(0, 3) as tag (tag)}
                      <span class="mini-tag tint-{tagTint(tag)}">{tag}</span>
                    {/each}
                    {#if (note.tags ?? []).length > 3}
                      <span class="mini-more">+{(note.tags ?? []).length - 3}</span>
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
          </li>
        {/each}
      </ul>
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

  /* En-tête de section : reprend la hiérarchie des intitulés de la
     référence de design (titre + compteur discret). */
  .list-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 1rem 1rem 0.2rem;
  }
  .list-head h2 {
    margin: 0;
    font-size: 0.95rem;
    font-weight: 600;
    letter-spacing: -0.005em;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .list-count {
    flex-shrink: 0;
    font-size: 0.72rem;
    color: var(--text-soft);
    background: var(--chip-bg);
    border-radius: 999px;
    padding: 0.05rem 0.5rem;
  }

  .search {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0.55rem 0.8rem 0.4rem;
    padding: 0.45rem 0.7rem;
    background: var(--input-bg);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text-faint);
  }
  .search:focus-within {
    border-color: var(--accent);
  }

  .search input {
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
    outline: none;
  }
  .search input::placeholder {
    color: var(--text-faint);
  }

  .clear {
    display: grid;
    place-items: center;
    width: 20px;
    height: 20px;
    border: none;
    border-radius: 50%;
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
    overflow-y: auto;
    min-height: 0;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0.4rem 0.6rem 0.8rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  /* Chaque note est une carte : liseré et fond net, léger relief au survol
     (traitement des cartes de la référence de design). Le bouton
     d'épinglage est un frère du bouton de sélection — jamais imbriqué. */
  .item-row {
    position: relative;
    border: 1px solid var(--card-border);
    border-radius: var(--radius);
    background: var(--card-bg);
    transition: background 0.12s ease, border-color 0.12s ease;
  }
  .item-row:hover,
  .item-row:focus-within {
    background: var(--hover);
    border-color: var(--border);
  }
  .item-row.active {
    background: var(--accent-soft);
    border-color: var(--accent);
  }

  .item {
    display: block;
    width: 100%;
    padding: 0.7rem 2.1rem 0.7rem 0.8rem;
    border: none;
    border-radius: var(--radius);
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .item:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
  }

  .title {
    display: block;
    font-weight: 560;
    font-size: 0.9rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Le bouton d'épinglage flotte au-dessus de la carte, hors du bouton
     de sélection. */
  .pin {
    position: absolute;
    top: 0.4rem;
    right: 0.35rem;
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border: none;
    border-radius: 7px;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.1s ease;
  }
  .item-row:hover .pin,
  .item-row:focus-within .pin,
  .pin.pinned {
    opacity: 1;
  }
  .pin:hover {
    background: var(--hover);
  }
  .pin.pinned {
    color: var(--warning);
  }

  .excerpt {
    margin-top: 0.15rem;
    font-size: 0.8rem;
    color: var(--text-soft);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  /* Terme trouvé dans l'extrait de recherche : surligné comme au marqueur. */
  .excerpt mark {
    background: var(--mark-bg);
    color: inherit;
    border-radius: 3px;
    padding: 0 0.12em;
  }

  .meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin-top: 0.35rem;
    font-size: 0.72rem;
    color: var(--text-faint);
  }

  .item-tags {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }

  .mini-tag {
    padding: 0.02rem 0.45rem;
    border: 1px solid var(--tint-line);
    border-radius: 999px;
    background: var(--tint-bg);
    color: var(--tint-ink);
    font-size: 0.68rem;
    line-height: 1.5;
  }

  .mini-more {
    padding: 0.02rem 0.2rem;
    color: var(--text-faint);
    font-size: 0.68rem;
  }
</style>
