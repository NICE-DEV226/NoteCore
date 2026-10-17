<script lang="ts">
  /**
   * Zone C — Éditeur (cdc.md §4.2)
   * Titre modifiable, éditeur Markdown, commandes de mise en forme
   * essentielles (§3.4), état de sauvegarde (§3.5), métadonnées
   * discrètes (dates §3.1, dossier §3.2, étiquettes).
   */
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';
  import EmptyState from './EmptyState.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import MarkdownPreview from '../editor/MarkdownPreview.svelte';
  import { formatFullDate, formatRelative, countWords } from '../utils/format';
  import { tagTint } from '../utils/tags';
  import { api } from '../api/client';
  import {
    deleteNote,
    exportSelectedNote,
    flushSave,
    retrySave,
    saveState,
    selectedNote,
    updateSelected,
    folders
  } from '../stores/workspace';

  type Mode = 'edit' | 'preview';

  let mode = $state<Mode>('edit');
  let contentEl = $state<HTMLTextAreaElement | null>(null);
  let pendingDelete = $state(false);

  const note = $derived($selectedNote);

  const SAVE_LABELS: Record<string, string> = {
    saved: 'Enregistré',
    dirty: 'Modifications non enregistrées',
    saving: 'Enregistrement…',
    error: "Erreur d'enregistrement"
  };

  /** Insers une mise en forme autour de la sélection courante (§3.4). */
  function wrap(before: string, after: string, placeholder: string): void {
    const el = contentEl;
    if (!el || !note) return;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? 0;
    const selected = note.content.slice(start, end) || placeholder;
    const next =
      note.content.slice(0, start) + before + selected + after + note.content.slice(end);
    updateSelected({ content: next });
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(start + before.length, start + before.length + selected.length);
    });
  }

  /** Préfixe la ligne courante (titres, listes). */
  function prefixLine(marker: string): void {
    const el = contentEl;
    if (!el || !note) return;
    const start = el.selectionStart ?? 0;
    const lineStart = note.content.lastIndexOf('\n', Math.max(0, start - 1)) + 1;
    const next = note.content.slice(0, lineStart) + marker + note.content.slice(lineStart);
    updateSelected({ content: next });
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(start + marker.length, start + marker.length);
    });
  }

  /** Tabulation dans le textarea (le focus ne sort pas du champ). */
  function insertTab(): void {
    const el = contentEl;
    if (!el || !note) return;
    const start = el.selectionStart ?? 0;
    const next = note.content.slice(0, start) + '  ' + note.content.slice(el.selectionEnd ?? start);
    updateSelected({ content: next });
    void tick().then(() => el.setSelectionRange(start + 2, start + 2));
  }

  /** Insère un fragment au curseur, en conservant la sélection. */
  function insertText(text: string): void {
    const el = contentEl;
    if (!note) return;
    if (!el) {
      // Mode aperçu : le textarea n'est pas monté, on ajoute en fin de
      // note plutôt que de perdre silencieusement le fragment.
      updateSelected({ content: note.content + text });
      return;
    }
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? start;
    const next = note.content.slice(0, start) + text + note.content.slice(end);
    updateSelected({ content: next });
    void tick().then(() => {
      el.focus();
      // Pour un bloc (ex. code, séparateur) on place le curseur après.
      el.setSelectionRange(start + text.length, start + text.length);
    });
  }

  /** Retire les marqueurs de mise en forme de la sélection (§3.4). */
  function clearFormat(): void {
    const el = contentEl;
    if (!el || !note) return;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? start;
    const cleaned = note.content
      .slice(start, end)
      .replace(/\*\*([^*]+)\*\*/g, '$1')
      .replace(/==([^=]+)==/g, '$1')
      .replace(/(^|[^*])\*([^*\n]+)\*/g, '$1$2')
      .replace(/`([^`]+)`/g, '$1')
      .replace(/^#{1,6}\s+/gm, '')
      .replace(/^>\s?/gm, '')
      .replace(/^\s*([-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?/gm, '');
    const next = note.content.slice(0, start) + cleaned + note.content.slice(end);
    updateSelected({ content: next });
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(start, start + cleaned.length);
    });
  }

  async function onDelete() {
    if (!note) return;
    const id = note.id;
    pendingDelete = false;
    await deleteNote(id);
    // après suppression, une autre note peut prendre le relais
  }

  // --- pièces jointes locales (ADR-006) ---

  /** Borne miroir du backend : refuser tôt évite un aller-retour inutile. */
  const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024;

  let fileInput = $state<HTMLInputElement | null>(null);
  let attachNotice = $state('');
  let attachBusy = $state(false);

  /** Légende sûre pour un lien/une image Markdown. */
  function altText(name: string): string {
    return name.replace(/[[\]\n\r]/g, ' ').trim() || 'pièce jointe';
  }

  /** Lit un fichier en base64 (sans le préfixe data:). */
  function readFileAsBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => {
        const result = String(reader.result ?? '');
        const comma = result.indexOf(',');
        resolve(comma >= 0 ? result.slice(comma + 1) : '');
      };
      reader.onerror = () => reject(reader.error ?? new Error('lecture impossible'));
      reader.readAsDataURL(file);
    });
  }

  /** Choisit un fichier local et l'insère comme pièce jointe de la note. */
  async function onFilesPicked(e: Event): Promise<void> {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = ''; // permet de re-choisir le même fichier
    if (!note || files.length === 0) return;

    attachNotice = '';
    attachBusy = true;
    try {
      for (const file of files) {
        if (file.size > MAX_ATTACHMENT_BYTES) {
          attachNotice = `« ${file.name} » dépasse la taille maximale (10 Mio).`;
          continue;
        }
        const dataBase64 = await readFileAsBase64(file);
        const att = await api.addAttachment(note.id, file.name, file.type, dataBase64);
        const snippet = att.mime.startsWith('image/')
          ? `![${altText(att.filename)}](attachment:${att.id})`
          : `[${altText(att.filename)}](attachment:${att.id})`;
        insertText(snippet);
      }
    } catch (err) {
      attachNotice = err instanceof Error ? err.message : "L'ajout de la pièce jointe a échoué.";
    } finally {
      attachBusy = false;
    }
  }

  async function onFolderMove(e: Event) {
    const value = (e.target as HTMLSelectElement).value;
    updateSelected({ folderId: value === '' ? null : value });
  }

  function addTag(input: HTMLInputElement) {
    if (!note) return;
    const tag = input.value.trim().toLowerCase();
    if (!tag) return;
    const current = note.tags ?? [];
    if (current.includes(tag)) return;
    updateSelected({ tags: [...current, tag] });
    input.value = '';
  }

  function removeTag(tag: string) {
    if (!note) return;
    updateSelected({ tags: (note.tags ?? []).filter((t) => t !== tag) });
  }
</script>

<section class="editor">
  {#if !note}
    <EmptyState
      icon="edit"
      title="Aucune note sélectionnée"
      message="Choisissez une note dans la liste pour commencer à écrire."
    />
  {:else}
    <!-- Barre supérieure : mise en forme et état de sauvegarde -->
    <div class="toolbar">
      <div class="formatting" role="group" aria-label="Mise en forme">
        <button title="Titre 1" aria-label="Titre 1" onclick={() => prefixLine('# ')}>H1</button>
        <button title="Titre 2" aria-label="Titre 2" onclick={() => prefixLine('## ')}>H2</button>
        <button title="Titre 3" aria-label="Titre 3" onclick={() => prefixLine('### ')}>H3</button>
        <span class="sep" aria-hidden="true"></span>
        <button title="Gras (Ctrl+B)" aria-label="Gras" onclick={() => wrap('**', '**', 'texte')}
          ><strong>G</strong></button
        >
        <button title="Italique (Ctrl+I)" aria-label="Italique" onclick={() => wrap('*', '*', 'texte')}
          ><em>I</em></button
        >
        <button title="Mise en évidence" aria-label="Mise en évidence" onclick={() => wrap('==', '==', 'évidence')}
          >==</button
        >
        <button title="Code en ligne" aria-label="Code en ligne" onclick={() => wrap('`', '`', 'code')}
          >`</button
        >
        <span class="sep" aria-hidden="true"></span>
        <button title="Liste à puces" aria-label="Liste à puces" onclick={() => prefixLine('- ')}>–</button>
        <button title="Liste ordonnée" aria-label="Liste ordonnée" onclick={() => prefixLine('1. ')}>1.</button>
        <button title="Case à cocher" aria-label="Case à cocher" onclick={() => prefixLine('- [ ] ')}>[ ]</button>
        <button title="Citation" aria-label="Citation" onclick={() => prefixLine('> ')}>&gt;</button>
        <span class="sep" aria-hidden="true"></span>
        <button title="Lien" aria-label="Lien" onclick={() => wrap('[', '](https://)', 'texte')}>Lien</button>
        <button title="Image depuis une URL" aria-label="Image depuis une URL" onclick={() => wrap('![', '](https://)', 'légende')}
          >Image</button
        >
        <button
          title="Joindre un fichier ou une image depuis l'ordinateur"
          aria-label="Joindre un fichier ou une image"
          disabled={attachBusy}
          onclick={() => {
            mode = 'edit'; // voir la référence s'insérer
            fileInput?.click();
          }}
        >
          {attachBusy ? 'Ajout…' : 'Joindre'}
        </button>
        <input
          bind:this={fileInput}
          type="file"
          multiple
          class="file-input"
          onchange={onFilesPicked}
        />
        <button title="Bloc de code" aria-label="Bloc de code" onclick={() => wrap('\n```\n', '\n```\n', 'code')}
          >{'{ }'}</button
        >
        <button title="Séparateur horizontal" aria-label="Séparateur horizontal" onclick={() => insertText('\n---\n')}
          >—</button
        >
        <span class="sep" aria-hidden="true"></span>
        <button title="Effacer la mise en forme" aria-label="Effacer la mise en forme" onclick={clearFormat}
          >Effacer</button
        >
      </div>

      <div class="right">
        {#if mode === 'edit'}
          <button class="mode" title="Aperçu du rendu Markdown" onclick={() => { mode = 'preview'; }}>
            <Icon name="eye" size={14} />
            Aperçu
          </button>
        {:else}
          <button class="mode" title="Revenir à la saisie" onclick={() => { mode = 'edit'; void flushSave(); }}>
            <Icon name="edit" size={14} />
            Éditer
          </button>
        {/if}
        <button
          class="mode"
          title="Exporter la note en Markdown"
          onclick={() => void exportSelectedNote()}
        >
          <Icon name="download" size={14} />
          Exporter
        </button>
        <button class="trash" title="Supprimer la note (avec confirmation)" onclick={() => (pendingDelete = true)}>
          <Icon name="trash" size={14} />
        </button>
      </div>
    </div>

    <div class="body">
      <!-- Titre modifiable -->
      <input
        class="title"
        value={note.title}
        placeholder="Titre de la note"
        aria-label="Titre de la note"
        oninput={(e) => updateSelected({ title: (e.target as HTMLInputElement).value })}
        onblur={() => void flushSave()}
      />

      <!-- Métadonnées discrètes -->
      <div class="meta-row">
        <select
          class="folder-select"
          value={note.folderId ?? ''}
          onchange={onFolderMove}
          aria-label="Dossier de la note"
        >
          <option value="">Sans dossier</option>
          {#each $folders as folder (folder.id)}
            <option value={folder.id}>{folder.name}</option>
          {/each}
        </select>

        <div class="tags">
          {#each note.tags ?? [] as tag (tag)}
            <span class="tag tint-{tagTint(tag)}">
              {tag}
              <button class="tag-x" title={`Retirer « ${tag} »`} onclick={() => removeTag(tag)}>
                <Icon name="x" size={10} />
              </button>
            </span>
          {/each}
          <input
            class="tag-input"
            placeholder="+ étiquette"
            aria-label="Ajouter une étiquette"
            onkeydown={(e) => {
              if (e.key === 'Enter') addTag(e.currentTarget);
            }}
          />
        </div>
      </div>

      <!-- Contenu : saisie ou aperçu -->
      <div class="content">
        {#if mode === 'edit'}
          <textarea
            bind:this={contentEl}
            value={note.content}
            placeholder="Écrivez ici… Markdown pris en charge : # titre, **gras**, *italique*, `code`, ```bloc de code, ==évidence==, listes, [liens](url), ![images](url)."
            aria-label="Contenu de la note"
            oninput={(e) => updateSelected({ content: (e.target as HTMLTextAreaElement).value })}
            onblur={() => void flushSave()}
            onkeydown={(e) => {
              if (e.key === 'Tab') {
                e.preventDefault();
                insertTab();
                return;
              }
              // Gras / italique à la frappe (§3.4) : les raccourcis
              // globaux (Ctrl+K, Ctrl+N, Ctrl+S) ne sont pas concernés.
              if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey) {
                const key = e.key.toLowerCase();
                if (key === 'b') {
                  e.preventDefault();
                  wrap('**', '**', 'texte');
                } else if (key === 'i') {
                  e.preventDefault();
                  wrap('*', '*', 'texte');
                }
              }
            }}
          ></textarea>
        {:else}
          <div class="preview-holder">
            <MarkdownPreview content={note.content} />
          </div>
        {/if}
      </div>
    </div>

    <!-- Barre d'état : sauvegarde (§3.5) + informations secondaires -->
    <div class="statusbar">
      <div
        class="save"
        class:is-error={$saveState === 'error'}
        class:is-saving={$saveState === 'saving'}
        role="status"
        aria-live="polite"
      >
        {#if $saveState === 'error'}
          <button
            class="retry"
            title="Réessayer maintenant"
            onclick={() => void retrySave()}
          >
            <Icon name="alert" size={13} />
            {SAVE_LABELS.error} — réessayer
          </button>
        {:else}
          <span class="dot" data-state={$saveState}></span>
          {SAVE_LABELS[$saveState] ?? ''}
        {/if}
      </div>

      <div class="details">
        {#if attachNotice}
          <span class="attach-notice" role="alert">{attachNotice}</span>
        {/if}
        <span>{countWords(note.content)} mots</span>
        <span title={`Créée le ${formatFullDate(note.createdAt)}`}>
          créée {formatRelative(note.createdAt)}
        </span>
        <span>modifiée {formatRelative(note.updatedAt)}</span>
      </div>
    </div>
  {/if}
</section>

{#if pendingDelete && note}
  <ConfirmDialog
    title="Supprimer cette note ?"
    message={`« ${note.title || 'Sans titre'} » sera définitivement supprimée. Cette action est irréversible.`}
    confirmLabel="Supprimer"
    onconfirm={() => void onDelete()}
    oncancel={() => (pendingDelete = false)}
  />
{/if}

<style>
  .editor {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    background: var(--app-bg);
  }

  .toolbar {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 0.4rem 0.6rem;
    padding: 0.5rem 1rem;
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }

  .formatting {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 1px;
  }
  .formatting button {
    min-width: 28px;
    height: 28px;
    padding: 0 0.45rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
    white-space: nowrap;
  }
  .formatting button:hover {
    background: var(--hover);
    color: var(--text);
  }
  .formatting button:disabled {
    opacity: 0.55;
    cursor: default;
  }

  /* L'entrée de fichier est déclenchée par le bouton « Joindre » :
     elle reste dans le DOM (accessibilité) mais invisible. */
  .file-input {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
    border: 0;
  }

  /* Séparateurs de groupes : purement visuels. */
  .sep {
    width: 1px;
    align-self: stretch;
    margin: 0.15rem 0.35rem;
    background: var(--border);
  }

  .right {
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .mode,
  .trash {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    height: 30px;
    padding: 0 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .mode:hover,
  .trash:hover {
    background: var(--hover);
    color: var(--text);
  }
  .trash:hover {
    color: var(--danger);
    background: var(--danger-soft);
  }

  .body {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    padding: 0 0 0 0;
    max-width: var(--editor-width);
    width: 100%;
    margin: 0 auto;
  }

  .title {
    margin: 1.2rem 1.6rem 0.2rem;
    border: none;
    background: transparent;
    color: var(--text);
    font-family: var(--editor-font);
    font-size: calc(var(--editor-font-size) * 1.55);
    font-weight: 650;
    letter-spacing: -0.005em;
    line-height: 1.25;
    outline: none;
  }
  .title::placeholder {
    color: var(--text-faint);
  }

  .meta-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.6rem 1rem;
    margin: 0.2rem 1.6rem 0.6rem;
  }

  .folder-select {
    padding: 0.25rem 0.5rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-soft);
    font: inherit;
    font-size: 0.78rem;
    cursor: pointer;
  }

  .tags {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .tag {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.18rem 0.55rem;
    border: 1px solid var(--tint-line);
    border-radius: 999px;
    background: var(--tint-bg);
    color: var(--tint-ink);
    font-size: 0.75rem;
  }

  .tag-x {
    display: grid;
    place-items: center;
    width: 14px;
    height: 14px;
    border: none;
    border-radius: 50%;
    padding: 0;
    background: transparent;
    color: var(--tint-ink);
    opacity: 0.55;
    cursor: pointer;
  }
  .tag-x:hover {
    opacity: 1;
  }

  .tag-input {
    border: none;
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.78rem;
    width: 7.5rem;
    outline: none;
  }
  .tag-input::placeholder {
    color: var(--text-faint);
  }

  .content {
    flex: 1;
    display: flex;
    min-height: 0;
    margin: 0 1.6rem 1rem;
  }

  textarea {
    flex: 1;
    width: 100%;
    resize: none;
    border: none;
    background: transparent;
    color: var(--text);
    font-family: var(--editor-font);
    font-size: var(--editor-font-size);
    line-height: 1.8;
    letter-spacing: 0.002em;
    padding: 0.6rem 0 2rem;
    outline: none;
  }
  textarea::placeholder {
    color: var(--text-faint);
  }

  .preview-holder {
    flex: 1;
    overflow-y: auto;
    padding: 0.8rem 0.2rem 2rem;
    background: var(--surface-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding-left: 1.2rem;
    padding-right: 1.2rem;
  }

  .statusbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.45rem 1.6rem;
    border-top: 1px solid var(--border);
    background: var(--surface);
    font-size: 0.76rem;
    color: var(--text-faint);
  }

  .save {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--text-soft);
  }

  .retry {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    border: none;
    background: var(--danger-soft);
    color: var(--danger);
    font: inherit;
    font-size: inherit;
    padding: 0.3rem 0.7rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }
  .retry:hover {
    background: var(--danger);
    color: var(--on-danger);
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--success);
  }
  .dot[data-state='dirty'] {
    background: var(--warning);
  }
  .dot[data-state='saving'] {
    background: var(--accent);
  }

  .details {
    display: flex;
    gap: 1rem;
    white-space: nowrap;
  }

  .attach-notice {
    color: var(--danger);
    white-space: normal;
  }
</style>
