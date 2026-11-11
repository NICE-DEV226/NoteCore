<script lang="ts">
  /**
   * Modèles de notes : des squelettes prêts à l'emploi (compte rendu,
   * séance, TP…) pour ne pas repartir d'une page blanche. Les modèles
   * personnels sont conservés dans les réglages (`noteTemplates`) ;
   * appliquer un modèle crée une note et y insère le contenu — rien
   * n'est appliqué par-dessus une note existante.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { api } from '../api/client';
  import { createNote, selectedNote, updateSelected } from '../stores/workspace';

  interface Template {
    id: string;
    name: string;
    content: string;
    builtin?: boolean;
  }

  const BUILTINS: Template[] = [
    {
      id: 'builtin-cours',
      name: 'Note de cours',
      builtin: true,
      content: `# <Sujet du cours>\n\n**Date :** \n**Intervenant :** \n\n## Idées principales\n\n- \n\n## Exemples\n\n\`\`\`\n\n\`\`\`\n\n## À retenir\n\n- \n`
    },
    {
      id: 'builtin-compte-rendu',
      name: 'Compte rendu de séance',
      builtin: true,
      content: `# Compte rendu — <date>\n\n## Présents\n\n## Ordre du jour\n\n1. \n\n## Décisions\n\n- \n\n## Actions à mener\n\n- [ ] \n\n## Prochaines étapes\n\n`
    },
    {
      id: 'builtin-tp',
      name: 'Compte-rendu de TP',
      builtin: true,
      content: `# TP <numéro> — <sujet>\n\n**Date :** \n**Binôme :** \n\n## Objectif\n\n## Protocole\n\n## Résultats\n\n| Mesure | Valeur |\n| --- | --- |\n|  |  |\n\n## Analyse\n\n## Conclusion\n\n`
    },
    {
      id: 'builtin-projet',
      name: 'Idée de projet',
      builtin: true,
      content: `# <Nom du projet>\n\n## Problème\n\n## Solution envisagée\n\n## Étapes\n\n- [ ] Cadrage\n- [ ] Prototype\n- [ ] Tests\n\n## Ressources\n\n`
    }
  ];

  const SETTINGS_KEY = 'noteTemplates';

  let {
    onclose
  }: { onclose: () => void } = $props();

  let customs = $state<Template[]>([]);
  let busy = $state(false);
  let error = $state('');
  let saving = $state(false);
  let newName = $state('');
  let dialogEl = $state<HTMLDivElement | null>(null);
  const openNote = $derived($selectedNote);

  const uid = `tpl-${Math.random().toString(36).slice(2, 8)}`;

  async function load(): Promise<void> {
    try {
      const settings = await api.getSettings();
      const raw = settings[SETTINGS_KEY];
      if (!raw) return;
      const parsed = JSON.parse(raw) as { id?: string; name?: string; content?: string }[];
      customs = (Array.isArray(parsed) ? parsed : [])
        .filter((t) => t.name && typeof t.content === 'string')
        .map((t, i) => ({ id: t.id || `custom-${i}`, name: String(t.name), content: String(t.content) }));
    } catch {
      customs = [];
    }
  }

  async function persist(): Promise<void> {
    await api.setSetting(SETTINGS_KEY, JSON.stringify(
      customs.map(({ name, content }) => ({ name, content }))
    ));
  }

  async function apply(t: Template): Promise<void> {
    busy = true;
    error = '';
    try {
      await createNote(null);
      // createNote sélectionne la nouvelle note ; on y dépose le modèle.
      updateSelected({ content: t.content });
      onclose();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Création impossible.';
    } finally {
      busy = false;
    }
  }

  async function saveCurrent(): Promise<void> {
    const name = newName.trim();
    if (!name || !openNote) return;
    saving = true;
    error = '';
    try {
      customs = [...customs.filter((t) => t.name.toLowerCase() !== name.toLowerCase()),
        { id: `custom-${Date.now()}`, name, content: openNote.content }];
      await persist();
      newName = '';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Enregistrement impossible.';
    } finally {
      saving = false;
    }
  }

  async function removeCustom(id: string): Promise<void> {
    customs = customs.filter((t) => t.id !== id);
    try {
      await persist();
    } catch {
      // Le réglage reste cohérent au prochain enregistrement.
    }
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.focus();
    void load();
    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.preventDefault();
        onclose();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

<div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div
    class="dialog"
    bind:this={dialogEl}
    role="dialog"
    aria-modal="true"
    aria-labelledby="{uid}-title"
    tabindex="-1"
  >
    <header>
      <h2 id="{uid}-title"><Icon name="layout" size={16} /> Modèles de notes</h2>
      <button class="x" aria-label="Fermer" onclick={onclose}><Icon name="x" size={15} /></button>
    </header>

    <div class="body">
      <p class="muted">Choisissez un modèle : une nouvelle note est créée avec son contenu. Vos notes existantes ne sont jamais modifiées.</p>

      <ul class="list">
        {#each BUILTINS as t (t.id)}
          <li>
            <button class="item" disabled={busy} onclick={() => void apply(t)}>
              <Icon name="file-text" size={15} />
              <span class="what">
                <strong>{t.name}</strong>
                <small>Modèle intégré</small>
              </span>
              <Icon name="chevron-right" size={14} />
            </button>
          </li>
        {/each}
        {#each customs as t (t.id)}
          <li class="custom">
            <button class="item" disabled={busy} onclick={() => void apply(t)}>
              <Icon name="star" size={15} />
              <span class="what">
                <strong>{t.name}</strong>
                <small>Votre modèle</small>
              </span>
              <Icon name="chevron-right" size={14} />
            </button>
            <button class="del" aria-label="Supprimer le modèle {t.name}" title="Supprimer ce modèle" onclick={() => void removeCustom(t.id)}>
              <Icon name="trash" size={14} />
            </button>
          </li>
        {/each}
      </ul>

      {#if openNote}
        <form class="save-as" onsubmit={(e) => { e.preventDefault(); void saveCurrent(); }}>
          <label for="{uid}-name">Enregistrer la note ouverte comme modèle</label>
          <div class="row">
            <input id="{uid}-name" bind:value={newName} placeholder="Nom du modèle" maxlength="60" />
            <button type="submit" class="btn" disabled={saving || !newName.trim()}>
              {saving ? 'Enregistrement…' : 'Enregistrer'}
            </button>
          </div>
          <p class="muted small">« {openNote.title || 'Sans titre'} » — son contenu actuel servira de modèle.</p>
        </form>
      {/if}

      {#if error}<p class="error" role="alert">{error}</p>{/if}
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: var(--scrim);
    backdrop-filter: blur(3px);
    display: grid;
    place-items: center;
    z-index: 100;
  }
  .dialog {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    width: min(30rem, calc(100% - 2rem));
    max-height: min(80vh, 40rem);
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow-lg);
    overflow: hidden;
  }
  .dialog:focus { outline: none; }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1.1rem;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    margin: 0;
    font-size: 0.98rem;
    display: flex;
    align-items: center;
    gap: 0.45rem;
  }
  .x {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
    padding: 0.3rem;
    border-radius: var(--radius);
    display: grid;
    place-items: center;
  }
  .x:hover { background: var(--hover); color: var(--text); }
  .body {
    padding: 0.9rem 1.1rem 1.1rem;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.9rem;
  }
  .muted { color: var(--text-soft); font-size: 0.86rem; margin: 0; }
  .small { font-size: 0.78rem; }
  .error { color: var(--danger); font-size: 0.85rem; margin: 0; }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .custom {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  .item {
    appearance: none;
    border: 1px solid var(--border);
    background: var(--surface-2, var(--hover));
    flex: 1;
    min-width: 0;
    font: inherit;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.55rem 0.7rem;
    border-radius: var(--radius);
    color: var(--text);
    text-align: left;
  }
  .item:hover:not(:disabled) { border-color: var(--accent); }
  .item:disabled { opacity: 0.6; cursor: default; }
  .what { display: flex; flex-direction: column; min-width: 0; flex: 1; }
  .what strong { font-size: 0.88rem; font-weight: 600; }
  .what small { font-size: 0.76rem; color: var(--text-soft); }
  .del {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    padding: 0.4rem;
    border-radius: var(--radius);
    display: grid;
    place-items: center;
    flex: none;
  }
  .del:hover { background: var(--hover); color: var(--danger); }
  .save-as {
    border-top: 1px solid var(--border);
    padding-top: 0.8rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .save-as label { font-size: 0.84rem; font-weight: 600; }
  .row { display: flex; gap: 0.5rem; }
  .row input {
    flex: 1;
    min-width: 0;
    font: inherit;
    font-size: 0.88rem;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--app-bg);
    color: var(--text);
  }
  .btn {
    appearance: none;
    border: none;
    font: inherit;
    font-size: 0.84rem;
    padding: 0.42rem 0.85rem;
    border-radius: var(--radius);
    cursor: pointer;
    background: var(--accent);
    color: var(--on-accent);
    white-space: nowrap;
  }
  .btn:disabled { opacity: 0.55; cursor: default; }
</style>
