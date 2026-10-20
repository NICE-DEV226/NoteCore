<script lang="ts">
  /**
   * « Nouvelle matière » : créer d'un coup une structure de dossiers à partir d'un
   * modèle (Cours, TD, TP, Révisions…), modifiable avant de valider, et
   * enregistrable comme modèle personnel. Tout ou rien ; les dossiers déjà
   * présents sont réutilisés — appliquer deux fois le même modèle est sans danger.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { api } from '../api/client';
  import { applyFolderTemplate, folders } from '../stores/workspace';
  import { folderPicker, templateDialog } from '../stores/ui';
  import { folderPath, pathLabel } from '../utils/folders';
  import {
    BUILTIN_TEMPLATES,
    CUSTOM_KEY_PREFIX,
    countExisting,
    decodeCustom,
    encodeCustom,
    freeCustomKey,
    MAX_CUSTOM_TEXT,
    parseStructure,
    type FolderTemplate
  } from '../utils/folderTemplates';
  import type { FolderSpec } from '../api/types';

  const initial = $templateDialog;
  let parentId = $state<string | null>(initial?.parentId ?? null);
  let selected = $state<FolderTemplate>(BUILTIN_TEMPLATES[0]);
  let text = $state(BUILTIN_TEMPLATES[0].text);
  let name = $state('');
  let custom = $state<FolderTemplate[]>([]);
  let customKeys = $state<string[]>([]);
  let saveName = $state('');
  let saving = $state(false);
  let busy = $state(false);
  let error = $state('');
  let note = $state('');
  let dialogEl = $state<HTMLDivElement | null>(null);
  let nameEl = $state<HTMLInputElement | null>(null);

  const parentDepth = $derived(parentId ? folderPath($folders, parentId).length : 0);
  const parsed = $derived(parseStructure(text, name, 8 - parentDepth));
  const needsName = $derived(text.includes('{nom}'));
  const where = $derived(parentId ? pathLabel(folderPath($folders, parentId)) : 'À la racine');

  /** Noms (en minuscules) des enfants du dossier désigné par `path`, sous le parent choisi. */
  function siblingsOf(path: string[]): Set<string> {
    let cur: string | null = parentId;
    for (const key of path) {
      const next: { id: string } | undefined = $folders.find(
        (f) => (f.parentId ?? null) === cur && f.name.toLowerCase() === key
      );
      if (!next) return new Set();
      cur = next.id;
    }
    return new Set($folders.filter((f) => (f.parentId ?? null) === cur).map((f) => f.name.toLowerCase()));
  }
  const existing = $derived(parsed.errors.length === 0 ? countExisting(parsed.tree, siblingsOf) : 0);
  const toCreate = $derived(Math.max(0, parsed.total - existing));

  function choose(t: FolderTemplate): void {
    selected = t;
    text = t.text;
    error = '';
    note = '';
    void Promise.resolve().then(() => nameEl?.focus());
  }

  function pickParent(): void {
    folderPicker.set({
      title: 'Créer dans…',
      exclude: [],
      current: parentId,
      rootLabel: 'À la racine (aucun dossier parent)',
      onpick: (id) => (parentId = id)
    });
  }

  async function create(): Promise<void> {
    if (parsed.errors.length > 0 || busy) return;
    busy = true;
    error = '';
    try {
      await applyFolderTemplate(parentId, parsed.tree);
      templateDialog.set(null);
    } catch (err) {
      error = err instanceof Error ? err.message : 'La création a échoué.';
    } finally {
      busy = false;
    }
  }

  async function loadCustom(): Promise<void> {
    try {
      const settings = await api.getSettings();
      const keys = Object.keys(settings).filter((k) => k.startsWith(CUSTOM_KEY_PREFIX)).sort();
      const list: FolderTemplate[] = [];
      const used: string[] = [];
      for (const k of keys) {
        const t = decodeCustom(k, settings[k]);
        if (t) {
          list.push(t);
          used.push(k);
        }
      }
      custom = list;
      customKeys = used;
    } catch {
      /* pas de modèles personnels plutôt qu'une erreur */
    }
  }

  async function saveTemplate(): Promise<void> {
    note = '';
    error = '';
    const label = saveName.trim();
    if (!label) {
      error = 'Donnez un nom à votre modèle.';
      return;
    }
    if (text.length > MAX_CUSTOM_TEXT) {
      error = `Ce modèle est trop long pour être enregistré (${MAX_CUSTOM_TEXT} caractères au plus).`;
      return;
    }
    const key = freeCustomKey(customKeys);
    if (!key) {
      error = 'Six modèles personnels au plus : supprimez-en un d\'abord.';
      return;
    }
    try {
      await api.setSetting(key, encodeCustom(label, text));
      saving = false;
      saveName = '';
      note = `Modèle « ${label} » enregistré.`;
      await loadCustom();
    } catch (err) {
      error = err instanceof Error ? err.message : "L'enregistrement a échoué.";
    }
  }

  async function removeTemplate(t: FolderTemplate): Promise<void> {
    try {
      await api.setSetting(t.id, '');
      if (selected.id === t.id) choose(BUILTIN_TEMPLATES[0]);
      await loadCustom();
    } catch (err) {
      error = err instanceof Error ? err.message : 'La suppression a échoué.';
    }
  }

  /** Aperçu : la structure lue, à plat avec son indentation. */
  function flatten(nodes: FolderSpec[], depth = 0): Array<{ name: string; depth: number }> {
    return nodes.flatMap((n) => [{ name: n.name, depth }, ...flatten(n.children ?? [], depth + 1)]);
  }
  const preview = $derived(flatten(parsed.tree));

  function close(): void {
    templateDialog.set(null);
  }

  onMount(() => {
    void loadCustom();
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    nameEl?.focus();
    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape' && !document.querySelector('[aria-labelledby="picker-title"]')) {
        e.preventDefault();
        close();
      } else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        void create();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

<div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div class="dialog" bind:this={dialogEl} role="dialog" aria-modal="true" aria-labelledby="tpl-title" tabindex="-1">
    <header>
      <h2 id="tpl-title"><Icon name="layout" size={17} /> Nouvelle matière</h2>
      <button class="x" aria-label="Fermer" onclick={close}><Icon name="x" size={16} /></button>
    </header>

    <div class="body">
      <section>
        <h3>Modèle</h3>
        <div class="chips" role="radiogroup" aria-label="Modèle">
          {#each [...BUILTIN_TEMPLATES, ...custom] as t (t.id)}
            <span class="chip-wrap">
              <button class="chip" class:on={selected.id === t.id} role="radio" aria-checked={selected.id === t.id} title={t.hint} onclick={() => choose(t)}>
                {t.label}
              </button>
              {#if t.custom}
                <button class="chip-x" aria-label={`Supprimer le modèle ${t.label}`} title="Supprimer ce modèle" onclick={() => void removeTemplate(t)}>
                  <Icon name="x" size={10} stroke={2.4} />
                </button>
              {/if}
            </span>
          {/each}
        </div>
        <p class="hint">{selected.hint}.</p>
      </section>

      <div class="grid">
        <section>
          {#if needsName}
            <h3><label for="tpl-name">Nom</label></h3>
            <input id="tpl-name" bind:this={nameEl} bind:value={name} placeholder="Réseaux, Projet NoteCore, Semestre 3…" autocomplete="off" />
          {/if}

          <h3 class="spaced">Créer dans</h3>
          <button class="where" onclick={pickParent} title="Choisir le dossier d'accueil">
            <Icon name="folder" size={14} />
            <span>{where}</span>
            <small>Changer…</small>
          </button>

          <h3 class="spaced"><label for="tpl-text">Structure</label></h3>
          <textarea id="tpl-text" bind:value={text} rows="9" spellcheck="false" aria-describedby="tpl-help"></textarea>
          <p class="hint" id="tpl-help">Une ligne par dossier ; deux espaces de plus = un niveau plus bas. <code>{'{nom}'}</code> est remplacé par le nom saisi.</p>
        </section>

        <section>
          <h3>Aperçu</h3>
          <div class="preview" aria-live="polite">
            {#if parsed.errors.length > 0}
              <ul class="errors" role="alert">
                {#each parsed.errors as e (e)}<li>{e}</li>{/each}
              </ul>
            {:else}
              <ul class="tree">
                {#each preview as row, i (i)}
                  <li style:padding-left="{row.depth * 1.1}rem"><Icon name="folder" size={13} /> {row.name}</li>
                {/each}
              </ul>
            {/if}
          </div>
          {#if parsed.errors.length === 0}
            <p class="count">
              <strong>{toCreate}</strong> dossier{toCreate > 1 ? 's' : ''} à créer{existing > 0 ? ` · ${existing} déjà présent${existing > 1 ? 's' : ''}, réutilisé${existing > 1 ? 's' : ''}` : ''}.
            </p>
          {/if}
        </section>
      </div>

      {#if error}<p class="error" role="alert">{error}</p>{/if}
      {#if note}<p class="ok" role="status">{note}</p>{/if}
    </div>

    <footer>
      {#if saving}
        <div class="save">
          <input bind:value={saveName} placeholder="Nom du modèle" maxlength="30" aria-label="Nom du modèle à enregistrer" onkeydown={(e) => e.key === 'Enter' && void saveTemplate()} />
          <button class="btn" onclick={() => void saveTemplate()}>Enregistrer</button>
          <button class="btn plain" onclick={() => (saving = false)}>Annuler</button>
        </div>
      {:else}
        <button class="btn plain" onclick={() => (saving = true)} title="Garder cette structure pour la réutiliser">Enregistrer comme modèle…</button>
        <span class="grow"></span>
        <button class="btn" onclick={close}>Annuler</button>
        <button class="btn primary" disabled={busy || parsed.errors.length > 0 || toCreate + existing === 0} onclick={() => void create()}>
          {toCreate === 0 && existing > 0 ? 'Ouvrir' : `Créer ${toCreate} dossier${toCreate > 1 ? 's' : ''}`}
        </button>
      {/if}
    </footer>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 125;
    display: grid;
    place-items: center;
    background: var(--scrim);
    backdrop-filter: blur(3px);
  }
  .dialog {
    display: flex;
    flex-direction: column;
    width: min(46rem, calc(100% - 2rem));
    max-height: calc(100vh - 2rem);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    outline: none;
    animation: pop 0.16s var(--ease);
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(6px) scale(0.985);
    }
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.2rem 0.4rem;
  }
  h2 {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0;
    font-size: 1.05rem;
  }
  .x {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
  }
  .x:hover {
    background: var(--hover);
  }
  .body {
    overflow-y: auto;
    padding: 0.3rem 1.2rem 0.6rem;
  }
  h3 {
    margin: 0 0 0.45rem;
    color: var(--text-faint);
    font-size: 0.7rem;
    font-weight: 650;
    letter-spacing: 0.07em;
    text-transform: uppercase;
  }
  h3.spaced {
    margin-top: 0.9rem;
  }
  h3 label {
    cursor: inherit;
  }
  .grid {
    display: grid;
    grid-template-columns: 1.1fr 1fr;
    gap: 1.2rem;
    margin-top: 0.8rem;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .chip-wrap {
    display: inline-flex;
    align-items: center;
  }
  .chip {
    padding: 0.3rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.82rem;
    cursor: pointer;
  }
  .chip:hover {
    border-color: var(--text-faint);
  }
  .chip.on {
    border-color: var(--accent);
    background: var(--accent-soft);
    color: var(--accent-text);
    font-weight: 600;
  }
  .chip-wrap:has(.chip-x) .chip {
    border-radius: 999px 0 0 999px;
    border-right: none;
  }
  .chip-x {
    display: grid;
    place-items: center;
    width: 24px;
    height: 100%;
    min-height: 28px;
    border: 1px solid var(--border);
    border-radius: 0 999px 999px 0;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
  }
  .chip-x:hover {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .hint {
    margin: 0.4rem 0 0;
    color: var(--text-faint);
    font-size: 0.76rem;
    line-height: 1.45;
  }
  input,
  textarea {
    width: 100%;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text);
    font: inherit;
    font-size: 0.9rem;
  }
  textarea {
    resize: vertical;
    font-family: var(--font-mono);
    font-size: 0.82rem;
    line-height: 1.5;
    tab-size: 2;
  }
  input:focus,
  textarea:focus {
    outline: none;
    border-color: var(--accent);
  }
  code {
    padding: 0.05em 0.35em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.9em;
  }
  .where {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.45rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card-bg);
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
    text-align: left;
    cursor: pointer;
  }
  .where span {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .where small {
    color: var(--accent-text);
  }
  .where:hover {
    border-color: var(--accent);
  }
  .preview {
    min-height: 11rem;
    max-height: 16rem;
    overflow-y: auto;
    padding: 0.6rem 0.7rem;
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    background: var(--app-bg);
  }
  .tree,
  .errors {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .tree li {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.15rem 0;
    font-size: 0.86rem;
  }
  .tree :global(svg) {
    color: var(--accent-text);
  }
  .errors li {
    padding: 0.15rem 0;
    color: var(--danger);
    font-size: 0.82rem;
    line-height: 1.4;
  }
  .count {
    margin: 0.5rem 0 0;
    color: var(--text-soft);
    font-size: 0.8rem;
  }
  .error,
  .ok {
    margin: 0.8rem 0 0;
    padding: 0.5rem 0.8rem;
    border-radius: var(--radius-sm);
    font-size: 0.84rem;
  }
  .error {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .ok {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  footer {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.8rem 1.2rem 1rem;
    border-top: 1px solid var(--border);
  }
  .grow {
    flex: 1;
  }
  .save {
    display: flex;
    flex: 1;
    gap: 0.5rem;
  }
  .save input {
    flex: 1;
    padding: 0.35rem 0.7rem;
  }
  .btn {
    height: 34px;
    padding: 0 1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    font-weight: 600;
    white-space: nowrap;
    cursor: pointer;
  }
  .btn.plain {
    border-color: transparent;
    background: transparent;
    color: var(--text-soft);
  }
  .btn.plain:hover {
    color: var(--text);
    background: var(--hover);
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--on-accent);
  }
  .btn:disabled {
    opacity: 0.5;
    cursor: default;
  }
  @media (max-width: 700px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
