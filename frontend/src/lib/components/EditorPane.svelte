<script lang="ts">
  /**
   * Zone C — Éditeur (cahier des charges §4.2)
   * Titre modifiable, éditeur Markdown, commandes de mise en forme
   * essentielles (§3.4), état de sauvegarde (§3.5), métadonnées
   * discrètes (dates §3.1, dossier §3.2, étiquettes).
   */
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import Home from './Home.svelte';
  import Files from './Files.svelte';
  import { filesOpen, openAttachmentViewer } from '../stores/files';
  import { shareDialog } from '../stores/ui';
  import MarkdownPreview from '../editor/MarkdownPreview.svelte';
  import { editorMode, focusMode, type EditorMode } from '../stores/ui';
  import { display } from '../stores/display';
  import { startReview } from '../stores/review';
  import { nextClozeNumber } from '../utils/cards';
  import { formatFullDate, formatRelative, countWords } from '../utils/format';
  import { tagTint } from '../utils/tags';
  import { api } from '../api/client';
  import type { Attachment, NoteCards } from '../api/types';
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

  /** Titre attribué par le backend à une note créée sans titre. */
  const DEFAULT_TITLE = 'Sans titre';

  const mode = $derived($editorMode);
  let contentEl = $state<HTMLTextAreaElement | null>(null);
  let pendingDelete = $state(false);

  const note = $derived($selectedNote);

  /** Les commandes de mise en forme n'agissent qu'en mode Édition : en
   *  aperçu, le champ de saisie n'est pas monté et une commande serait
   *  sans effet (ou insérée au mauvais endroit). */
  const editing = $derived(mode !== 'preview');

  const SAVE_LABELS: Record<string, string> = {
    saved: 'Enregistré',
    dirty: 'Modifications non enregistrées',
    saving: 'Enregistrement…',
    error: "Erreur d'enregistrement"
  };

  // --- annulation / rétablissement ---------------------------------------
  // Historique propre à l'éditeur : l'annulation native du champ ne couvre
  // pas les modifications faites par la barre d'outils (écriture
  // programmatique) et ne survit pas au rendu piloté par le store.

  type Snapshot = { value: string };
  const HISTORY_LIMIT = 200;
  /** Frappes rapprochées : une seule étape d'annulation. */
  const GROUP_MS = 700;

  let historyNoteId = '';
  let undoStack: Snapshot[] = [];
  let redoStack: Snapshot[] = [];
  let lastEditAt = 0;

  function ensureHistory(): void {
    if (!note || note.id === historyNoteId) return;
    historyNoteId = note.id;
    undoStack = [];
    redoStack = [];
    lastEditAt = 0;
  }

  /** À appeler AVANT de remplacer le contenu. `group` fusionne les frappes. */
  function recordBeforeChange(group: boolean): void {
    if (!note) return;
    ensureHistory();
    const now = Date.now();
    if (!group || undoStack.length === 0 || now - lastEditAt > GROUP_MS) {
      undoStack.push({ value: note.content });
      if (undoStack.length > HISTORY_LIMIT) undoStack.shift();
    }
    redoStack = [];
    lastEditAt = now;
  }

  /** Remplace le contenu en conservant une étape d'annulation. */
  function applyContent(next: string, group = false): void {
    if (!note || next === note.content) return;
    recordBeforeChange(group);
    updateSelected({ content: next });
  }

  function restore(from: Snapshot[], to: Snapshot[]): void {
    const el = contentEl;
    if (!el || !note) return;
    ensureHistory();
    const target = from.pop();
    if (!target) return;
    const current = note.content;
    to.push({ value: current });
    updateSelected({ content: target.value });
    lastEditAt = 0; // la frappe suivante ouvre une nouvelle étape

    // Curseur placé à la fin de la zone modifiée.
    let suffix = 0;
    const max = Math.min(current.length, target.value.length);
    while (suffix < max && current[current.length - 1 - suffix] === target.value[target.value.length - 1 - suffix]) {
      suffix++;
    }
    const caret = target.value.length - suffix;
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(caret, caret);
    });
  }

  function undo(): void {
    restore(undoStack, redoStack);
  }

  function redo(): void {
    restore(redoStack, undoStack);
  }

  /** Applique OU retire une mise en forme autour de la sélection (§3.4).
   *  Une barre d'outils qui ne sait qu'ajouter oblige à effacer à la main :
   *  la même commande sert d'interrupteur. */
  function wrap(before: string, after: string, placeholder: string): void {
    const el = contentEl;
    if (!el || !note) return;
    const content = note.content;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? 0;
    const selected = content.slice(start, end);

    // 1. La sélection porte déjà le balisage complet : on le retire.
    if (
      selected.length >= before.length + after.length &&
      selected.startsWith(before) &&
      selected.endsWith(after)
    ) {
      const inner = selected.slice(before.length, selected.length - after.length);
      replaceRange(el, content, start, end, inner, start, start + inner.length);
      return;
    }

    // 2. Le balisage encadre la sélection : on le retire aussi.
    const outerStart = start - before.length;
    if (
      outerStart >= 0 &&
      content.slice(outerStart, start) === before &&
      content.slice(end, end + after.length) === after
    ) {
      replaceRange(
        el,
        content,
        outerStart,
        end + after.length,
        selected,
        outerStart,
        outerStart + selected.length
      );
      return;
    }

    // 3. Sinon on applique ; une sélection vide reçoit le texte d'exemple,
    //    laissé sélectionné pour être remplacé directement à la frappe.
    const text = selected || placeholder;
    const next = content.slice(0, start) + before + text + after + content.slice(end);
    applyContent(next);
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(start + before.length, start + before.length + text.length);
    });
  }

  /** Remplace [from, to) par text puis restaure une sélection dessus. */
  function replaceRange(
    el: HTMLTextAreaElement,
    content: string,
    from: number,
    to: number,
    text: string,
    selStart: number,
    selEnd: number
  ): void {
    applyContent(content.slice(0, from) + text + content.slice(to));
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(selStart, selEnd);
    });
  }

  /** Bornes des lignes touchées par la sélection courante. */
  function selectedLines(content: string, start: number, end: number) {
    const lineStart = start === 0 ? 0 : content.lastIndexOf('\n', start - 1) + 1;
    const nextBreak = content.indexOf('\n', end);
    return { lineStart, blockEnd: nextBreak === -1 ? content.length : nextBreak };
  }

  /** Ajoute — ou retire, si toutes les lignes de la sélection le portent
   *  déjà — un préfixe sur chacune d'elles (titres, listes, citation). */
  function prefixLine(marker: string): void {
    const el = contentEl;
    if (!el || !note) return;
    const content = note.content;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? 0;
    const { lineStart, blockEnd } = selectedLines(content, start, end);

    const lines = content.slice(lineStart, blockEnd).split('\n');
    const allPrefixed = lines.every((l) => l.startsWith(marker));
    const nextLines = allPrefixed
      ? lines.map((l) => l.slice(marker.length))
      : lines.map((l) => (l.startsWith(marker) ? l : marker + l));
    const nextBlock = nextLines.join('\n');
    applyContent(content.slice(0, lineStart) + nextBlock + content.slice(blockEnd));
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(lineStart, lineStart + nextBlock.length);
    });
  }

  /** Indente (Tab) ou désindente (Maj+Tab) les lignes sélectionnées :
   *  la touche ne fait plus sortir le focus du champ de saisie. */
  function insertTab(outdent: boolean): void {
    const el = contentEl;
    if (!el || !note) return;
    const content = note.content;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? 0;
    const { lineStart, blockEnd } = selectedLines(content, start, end);

    const lines = content.slice(lineStart, blockEnd).split('\n');
    const nextLines = outdent
      ? lines.map((l) => l.replace(/^ {1,2}/, ''))
      : lines.map((l) => '  ' + l);
    const nextBlock = nextLines.join('\n');
    applyContent(content.slice(0, lineStart) + nextBlock + content.slice(blockEnd));
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(lineStart, lineStart + nextBlock.length);
    });
  }

  /** Insère un fragment au curseur, en conservant la sélection. */
  function insertText(text: string): void {
    const el = contentEl;
    if (!note) return;
    if (!el) {
      // Mode aperçu : le textarea n'est pas monté, on ajoute en fin de
      // note plutôt que de perdre silencieusement le fragment.
      applyContent(note.content + text);
      return;
    }
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? start;
    const next = note.content.slice(0, start) + text + note.content.slice(end);
    applyContent(next);
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
    applyContent(next);
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(start, start + cleaned.length);
    });
  }

  const MODES: Array<{ value: EditorMode; label: string; icon: string }> = [
    { value: 'edit', label: 'Éditer', icon: 'edit' },
    { value: 'split', label: 'Scinder', icon: 'columns' },
    { value: 'preview', label: 'Aperçu', icon: 'eye' }
  ];

  function setMode(next: EditorMode): void {
    editorMode.set(next);
    if (next === 'preview') void flushSave();
  }

  // --- cartes de révision (ADR-007) ---

  /** Cache la sélection : « {{c1::sélection}} », au premier numéro libre. */
  function addCloze(): void {
    if (!note) return;
    const n = nextClozeNumber(note.content);
    wrap(`{{c${n}::`, '}}', 'réponse');
  }

  /** Insère « Question :: Réponse » sur sa propre ligne, « Question » sélectionné. */
  function addQuestion(): void {
    const el = contentEl;
    if (!el || !note) return;
    const content = note.content;
    const start = el.selectionStart ?? content.length;
    const atLineStart = start === 0 || content[start - 1] === '\n';
    const lead = atLineStart ? '' : '\n';
    const text = `${lead}Question :: Réponse\n`;
    applyContent(content.slice(0, start) + text + content.slice(el.selectionEnd ?? start));
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(start + lead.length, start + lead.length + 'Question'.length);
    });
  }

  let cardInfo = $state<NoteCards | null>(null);
  $effect(() => {
    const id = note?.id;
    // Les cartes suivent le texte enregistré : on relit une fois la sauvegarde faite.
    if (!id || $saveState !== 'saved') return;
    let cancelled = false;
    api
      .noteCards(id)
      .then((info) => {
        if (!cancelled) cardInfo = info;
      })
      .catch(() => {
        if (!cancelled) cardInfo = null;
      });
    return () => {
      cancelled = true;
    };
  });

  const wordCount = $derived(note ? countWords(note.content) : 0);
  const readingMinutes = $derived(Math.max(1, Math.round(wordCount / 220)));

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

  let imageInput = $state<HTMLInputElement | null>(null);
  let anyInput = $state<HTMLInputElement | null>(null);
  let titleEl = $state<HTMLInputElement | null>(null);
  let attachNotice = $state('');
  let attachBusy = $state(false);

  /** Pièces jointes de la note ouverte, listées pour être supprimables :
   *  sans cette liste, une image importée puis retirée du texte
   *  resterait dans la base sans aucun moyen de la voir ni de l'enlever. */
  let attachments = $state<Attachment[]>([]);
  let attachmentError = $state('');
  let attachmentPendingDelete = $state<Attachment | null>(null);

  // Une note nouvellement créée (titre et contenu vides) reçoit le focus
  // sur son titre : la première frappe n'est jamais perdue (CDC §4.1).
  let lastFocusedNoteId = '';
  $effect(() => {
    const current = note;
    if (!current || current.id === lastFocusedNoteId) return;
    lastFocusedNoteId = current.id;
    // Le backend donne « Sans titre » à une note neuve : c'est ce titre par
    // défaut, et non un titre vide, qui désigne une note à peine créée.
    const title = current.title.trim();
    if ((title !== '' && title !== DEFAULT_TITLE) || current.content.trim() !== '') return;
    const active = document.activeElement;
    if (active instanceof HTMLTextAreaElement || active instanceof HTMLInputElement) return;
    void tick().then(() => {
      titleEl?.focus();
      titleEl?.select(); // la première frappe remplace le titre par défaut
    });
  });

  // Charge les pièces jointes à chaque changement de note sélectionnée.
  $effect(() => {
    const id = note?.id ?? '';
    attachmentError = '';
    if (!id) {
      attachments = [];
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const list = await api.listAttachments(id);
        if (!cancelled) attachments = list;
      } catch (err) {
        if (!cancelled) {
          attachmentError =
            err instanceof Error ? err.message : 'Les pièces jointes sont illisibles.';
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  /** Extensions d'image reconnues localement : certaines WebView
   *  laissent `File.type` vide. Le backend affine ensuite le type. */
  const IMAGE_EXT = /\.(png|jpe?g|gif|webp|bmp|avif|svgz?|ico|tiff?)$/i;

  function looksLikeImage(file: File): boolean {
    return file.type.startsWith('image/') || IMAGE_EXT.test(file.name);
  }

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

  /** Importe des fichiers du PC comme pièces jointes de la note.
   *  En édition, la référence est insérée au curseur ; en aperçu, elle
   *  est ajoutée en fin de note et l'image apparaît aussitôt. */
  async function importFiles(files: File[], kind: 'image' | 'any'): Promise<void> {
    if (!note || files.length === 0) return;

    attachNotice = '';
    attachBusy = true;
    try {
      for (const file of files) {
        if (kind === 'image' && !looksLikeImage(file)) {
          attachNotice = `« ${file.name} » n'est pas une image reconnue.`;
          continue;
        }
        if (file.size > MAX_ATTACHMENT_BYTES) {
          attachNotice = `« ${file.name} » dépasse la taille maximale (10 Mio).`;
          continue;
        }
        const dataBase64 = await readFileAsBase64(file);
        // Le type retenu vient du backend (empreinte binaire) : une image
        // importée est insérée comme image, jamais comme un simple lien.
        const att = await api.addAttachment(note.id, file.name, file.type, dataBase64);
        attachments = [...attachments, att];
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

  async function onFilesPicked(e: Event, kind: 'image' | 'any'): Promise<void> {
    const input = e.currentTarget as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    input.value = ''; // permet de re-choisir le même fichier
    await importFiles(files, kind);
  }

  /** Retire du texte les références Markdown à une pièce jointe : une
   *  référence orpheline afficherait une image cassée dans l'aperçu. */
  function stripAttachmentRefs(content: string, attachmentId: string): string {
    const id = attachmentId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const reference = new RegExp(`!?\\[[^\\]]*\\]\\(attachment:${id}\\)`, 'g');
    return content.replace(reference, '').replace(/[ \t]+\n/g, '\n');
  }

  /** Supprime la pièce jointe confirmée, dans la base et dans le texte. */
  async function removeAttachment(): Promise<void> {
    const att = attachmentPendingDelete;
    const current = note;
    attachmentPendingDelete = null;
    if (!att || !current) return;

    attachmentError = '';
    attachBusy = true;
    try {
      await api.deleteAttachment(att.id);
      attachments = attachments.filter((a) => a.id !== att.id);
      const cleaned = stripAttachmentRefs(current.content, att.id);
      if (cleaned !== current.content) applyContent(cleaned);
    } catch (err) {
      attachmentError =
        err instanceof Error ? err.message : 'La suppression de la pièce jointe a échoué.';
    } finally {
      attachBusy = false;
    }
  }

  /** Libellé court d'une pièce jointe : taille lisible, nom complet en info. */
  function attachmentSize(bytes: number): string {
    if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} Mio`;
    if (bytes >= 1024) return `${Math.round(bytes / 1024)} Kio`;
    return `${bytes} o`;
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
    {#if $filesOpen}
      <Files />
    {:else}
      <Home />
    {/if}
  {:else}
    <div class="topbar">
      <div class="save" class:is-error={$saveState === 'error'} role="status" aria-live="polite">
        {#if $saveState === 'error'}
          <button class="retry" title="Réessayer maintenant" onclick={() => void retrySave()}>
            <Icon name="alert" size={13} />
            {SAVE_LABELS.error} — réessayer
          </button>
        {:else}
          <span class="dot" data-state={$saveState}></span>
          <span class="save-text">{SAVE_LABELS[$saveState] ?? ''}</span>
        {/if}
      </div>

      <div class="right">
        <div class="segmented" role="group" aria-label="Mode d'affichage">
          {#each MODES as m (m.value)}
            <button
              class:active={mode === m.value}
              aria-pressed={mode === m.value}
              title={m.label}
              onclick={() => setMode(m.value)}
            >
              <Icon name={m.icon} size={14} />
              <span class="seg-label">{m.label}</span>
            </button>
          {/each}
        </div>
        <button
          class="icon-btn"
          class:on={$focusMode}
          aria-pressed={$focusMode}
          title="Mode concentration — Ctrl+\"
          aria-label="Mode concentration"
          onclick={() => focusMode.update((v) => !v)}
        >
          <Icon name="focus" size={16} />
        </button>
        <button
          class="icon-btn"
          title="Partager cette note sans connexion (fichier ou réseau local)"
          aria-label="Partager la note"
          onclick={() => shareDialog.set({ scope: 'note', id: note?.id ?? '' })}
        >
          <Icon name="share" size={16} />
        </button>
        <button
          class="icon-btn"
          title="Exporter la note en Markdown (.md + pièces jointes)"
          aria-label="Exporter la note"
          onclick={() => void exportSelectedNote()}
        >
          <Icon name="download" size={16} />
        </button>
        <button
          class="icon-btn danger"
          title="Supprimer la note (avec confirmation)"
          aria-label="Supprimer la note"
          onclick={() => (pendingDelete = true)}
        >
          <Icon name="trash" size={16} />
        </button>
      </div>
    </div>

    {#if editing}
      <div class="formatbar" role="toolbar" aria-label="Mise en forme">
        <button title="Annuler — Ctrl+Z" aria-label="Annuler" onclick={undo}><Icon name="undo" size={15} /></button>
        <button title="Rétablir — Ctrl+Maj+Z" aria-label="Rétablir" onclick={redo}><Icon name="redo" size={15} /></button>
        <span class="sep" aria-hidden="true"></span>
        <button class="txt" title="Titre 1" aria-label="Titre 1" onclick={() => prefixLine('# ')}>H1</button>
        <button class="txt" title="Titre 2" aria-label="Titre 2" onclick={() => prefixLine('## ')}>H2</button>
        <button class="txt" title="Titre 3" aria-label="Titre 3" onclick={() => prefixLine('### ')}>H3</button>
        <span class="sep" aria-hidden="true"></span>
        <button title="Gras — Ctrl+B" aria-label="Gras" onclick={() => wrap('**', '**', 'texte')}><Icon name="bold" size={15} /></button>
        <button title="Italique — Ctrl+I" aria-label="Italique" onclick={() => wrap('*', '*', 'texte')}><Icon name="italic" size={15} /></button>
        <button title="Mise en évidence" aria-label="Mise en évidence" onclick={() => wrap('==', '==', 'évidence')}><Icon name="highlight" size={15} /></button>
        <button title="Code en ligne" aria-label="Code en ligne" onclick={() => wrap('`', '`', 'code')}><Icon name="code" size={15} /></button>
        <span class="sep" aria-hidden="true"></span>
        <button title="Liste à puces" aria-label="Liste à puces" onclick={() => prefixLine('- ')}><Icon name="list" size={15} /></button>
        <button title="Liste numérotée" aria-label="Liste numérotée" onclick={() => prefixLine('1. ')}><Icon name="list-ordered" size={15} /></button>
        <button title="Case à cocher" aria-label="Case à cocher" onclick={() => prefixLine('- [ ] ')}><Icon name="check-square" size={15} /></button>
        <button title="Citation" aria-label="Citation" onclick={() => prefixLine('> ')}><Icon name="quote" size={15} /></button>
        <span class="sep" aria-hidden="true"></span>
        <button title="Lien" aria-label="Lien" onclick={() => wrap('[', '](https://)', 'texte')}><Icon name="link" size={15} /></button>
        <button title="Bloc de code" aria-label="Bloc de code" onclick={() => wrap('\n```\n', '\n```\n', 'code')}><Icon name="code-block" size={15} /></button>
        <button title="Séparateur horizontal" aria-label="Séparateur horizontal" onclick={() => insertText('\n---\n')}><Icon name="minus" size={15} /></button>
        <span class="sep" aria-hidden="true"></span>
        <button
          title="Insérer une image depuis l'ordinateur"
          aria-label="Insérer une image depuis l'ordinateur"
          disabled={attachBusy}
          onclick={() => imageInput?.click()}><Icon name="image" size={15} /></button
        >
        <button
          title="Joindre un fichier depuis l'ordinateur"
          aria-label="Joindre un fichier depuis l'ordinateur"
          disabled={attachBusy}
          onclick={() => anyInput?.click()}><Icon name="paperclip" size={15} /></button
        >
        <span class="sep" aria-hidden="true"></span>
        <button title="Carte à trous : cacher la sélection (révision)" aria-label="Cacher la sélection pour la révision" onclick={addCloze}><Icon name="cloze" size={15} /></button>
        <button title="Carte question / réponse" aria-label="Insérer une carte question et réponse" onclick={addQuestion}><Icon name="layers" size={15} /></button>
        <span class="sep" aria-hidden="true"></span>
        <button title="Effacer la mise en forme" aria-label="Effacer la mise en forme" onclick={clearFormat}><Icon name="eraser" size={15} /></button>
      </div>
    {:else}
      <div class="formatbar quiet">
        <span class="preview-hint">Aperçu en lecture seule — passez en « Éditer » ou « Scinder » pour écrire.</span>
      </div>
    {/if}

    <input
      bind:this={imageInput}
      type="file"
      accept="image/*"
      multiple
      class="file-input"
      tabindex="-1"
      aria-hidden="true"
      onchange={(e) => void onFilesPicked(e, 'image')}
    />
    <input
      bind:this={anyInput}
      type="file"
      multiple
      class="file-input"
      tabindex="-1"
      aria-hidden="true"
      onchange={(e) => void onFilesPicked(e, 'any')}
    />

    <div class="document" class:split={mode === 'split'}>
      <div class="head">
        <input
          class="title"
          bind:this={titleEl}
          value={note.title}
          placeholder="Titre de la note"
          aria-label="Titre de la note"
          oninput={(e) => updateSelected({ title: (e.target as HTMLInputElement).value })}
          onblur={() => void flushSave()}
        />

        <div class="meta-row">
          <label class="folder-pick">
            <Icon name="folder" size={14} />
            <select value={note.folderId ?? ''} onchange={onFolderMove} aria-label="Dossier de la note">
              <option value="">Sans dossier</option>
              {#each $folders as folder (folder.id)}
                <option value={folder.id}>{folder.name}</option>
              {/each}
            </select>
          </label>

          <div class="tags">
            {#each note.tags ?? [] as tag (tag)}
              <span class="tag tint-{tagTint(tag)}">
                {tag}
                <button class="tag-x" title={`Retirer « ${tag} »`} aria-label={`Retirer l'étiquette ${tag}`} onclick={() => removeTag(tag)}>
                  <Icon name="x" size={10} stroke={2.4} />
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

        {#if attachments.length > 0 || attachmentError}
          <div class="attachments">
            <span class="attachments-label">
              <Icon name="paperclip" size={12} />
              Pièces jointes
            </span>
            <div class="attachment-list">
              {#each attachments as att (att.id)}
                <span class="attachment" title={`${att.filename} — ${att.mime}`}>
                  <button class="attachment-open" title="Lire ce fichier" aria-label={`Lire ${att.filename}`} onclick={() => void openAttachmentViewer(att.id)}>
                    <span class="attachment-name">{att.filename}</span>
                  </button>
                  <span class="attachment-size">{attachmentSize(att.size)}</span>
                  <button
                    class="attachment-x"
                    title={`Supprimer « ${att.filename} »`}
                    aria-label={`Supprimer la pièce jointe ${att.filename}`}
                    disabled={attachBusy}
                    onclick={() => (attachmentPendingDelete = att)}
                  >
                    <Icon name="x" size={10} stroke={2.4} />
                  </button>
                </span>
              {/each}
            </div>
            {#if attachmentError}
              <span class="attachment-error" role="alert">{attachmentError}</span>
            {/if}
          </div>
        {/if}
      </div>

      <div class="panes">
        {#if editing}
          <div class="pane write">
            <textarea
              bind:this={contentEl}
              value={note.content}
              placeholder="Écrivez ici… Markdown pris en charge : titres, gras, italique, code, listes, tableaux, citations et liens. Les images s'importent depuis l'ordinateur."
              aria-label="Contenu de la note"
              spellcheck={$display.spellcheck}
              oninput={(e) => applyContent((e.target as HTMLTextAreaElement).value, true)}
              onblur={() => void flushSave()}
              onkeydown={(e) => {
                if (e.key === 'Tab') {
                  e.preventDefault();
                  insertTab(e.shiftKey);
                  return;
                }
                if ((e.ctrlKey || e.metaKey) && !e.altKey) {
                  const key = e.key.toLowerCase();
                  // Annuler / rétablir : Ctrl+Z, Ctrl+Maj+Z, Ctrl+Y
                  if (key === 'z') {
                    e.preventDefault();
                    if (e.shiftKey) redo();
                    else undo();
                    return;
                  }
                  if (key === 'y' && !e.shiftKey) {
                    e.preventDefault();
                    redo();
                    return;
                  }
                  // Gras / italique à la frappe (§3.4)
                  if (!e.shiftKey && key === 'b') {
                    e.preventDefault();
                    wrap('**', '**', 'texte');
                  } else if (!e.shiftKey && key === 'i') {
                    e.preventDefault();
                    wrap('*', '*', 'texte');
                  }
                }
              }}
            ></textarea>
          </div>
        {/if}
        {#if mode !== 'edit'}
          <div class="pane read">
            <div class="preview-holder">
              <MarkdownPreview content={note.content} />
            </div>
          </div>
        {/if}
      </div>
    </div>

    <div class="statusbar">
      <div class="details">
        {#if attachNotice}
          <span class="attach-notice" role="alert">{attachNotice}</span>
        {/if}
        <span>{wordCount} mot{wordCount > 1 ? 's' : ''}</span>
        <span>{readingMinutes} min de lecture</span>
        {#if cardInfo && cardInfo.cards > 0}
          <button class="cards-chip" title="Réviser maintenant" onclick={() => void startReview()}>
            <Icon name="layers" size={12} />
            {cardInfo.cards} carte{cardInfo.cards > 1 ? 's' : ''}
            {#if cardInfo.due > 0}· {cardInfo.due} à réviser{/if}
          </button>
        {/if}
      </div>
      <div class="details">
        <span title={`Créée le ${formatFullDate(note.createdAt)}`}>créée {formatRelative(note.createdAt)}</span>
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

{#if attachmentPendingDelete}
  <ConfirmDialog
    title="Supprimer cette pièce jointe ?"
    message={`« ${attachmentPendingDelete.filename} » sera retirée de la note et de ses sauvegardes. La référence correspondante sera également retirée du texte.`}
    confirmLabel="Supprimer"
    onconfirm={() => void removeAttachment()}
    oncancel={() => (attachmentPendingDelete = null)}
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

  /* --- barre supérieure ------------------------------------------------ */
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.5rem 1rem;
    min-height: 46px;
  }
  .save {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    color: var(--text-faint);
    font-size: 0.78rem;
  }
  .save.is-error {
    color: var(--danger);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--success);
    transition: background 0.2s var(--ease);
  }
  .dot[data-state='dirty'] {
    background: var(--warning);
  }
  .dot[data-state='saving'] {
    background: var(--accent);
    animation: pulse 1s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  .retry {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.25rem 0.6rem;
    border: none;
    border-radius: 999px;
    background: var(--danger-soft);
    color: var(--danger);
    font: inherit;
    font-size: 0.78rem;
    font-weight: 600;
    cursor: pointer;
  }

  .right {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  .segmented {
    display: inline-flex;
    padding: 3px;
    gap: 2px;
    margin-right: 0.3rem;
    background: var(--input-bg);
    border-radius: var(--radius);
  }
  .segmented button {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    height: 26px;
    padding: 0 0.65rem;
    border: none;
    border-radius: calc(var(--radius) - 3px);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
    transition:
      background 0.12s var(--ease),
      color 0.12s var(--ease);
  }
  .segmented button:hover {
    color: var(--text);
  }
  .segmented button.active {
    background: var(--surface-elevated);
    color: var(--text);
    box-shadow: var(--shadow-sm);
    font-weight: 600;
  }
  .icon-btn {
    display: grid;
    place-items: center;
    width: 32px;
    height: 32px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
    transition:
      background 0.1s var(--ease),
      color 0.1s var(--ease);
  }
  .icon-btn:hover {
    background: var(--hover);
    color: var(--text);
  }
  .icon-btn.on {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .icon-btn.danger:hover {
    background: var(--danger-soft);
    color: var(--danger);
  }

  /* --- barre de mise en forme ------------------------------------------ */
  .formatbar {
    display: flex;
    align-items: center;
    gap: 1px;
    margin: 0 1rem;
    padding: 0.25rem 0.4rem;
    overflow-x: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
    scrollbar-width: none;
  }
  .formatbar::-webkit-scrollbar {
    display: none;
  }
  .formatbar button {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    min-width: 30px;
    height: 30px;
    padding: 0 0.3rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
    transition:
      background 0.1s var(--ease),
      color 0.1s var(--ease);
  }
  .formatbar button.txt {
    font: inherit;
    font-size: 0.78rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .formatbar button:not(:disabled):hover {
    background: var(--hover);
    color: var(--text);
  }
  .formatbar button:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .formatbar.quiet {
    border-style: dashed;
    box-shadow: none;
    background: transparent;
    min-height: 38px;
  }
  .preview-hint {
    padding: 0 0.5rem;
    color: var(--text-faint);
    font-size: 0.78rem;
    white-space: nowrap;
  }
  .sep {
    flex-shrink: 0;
    width: 1px;
    height: 16px;
    margin: 0 0.3rem;
    background: var(--border);
  }

  /* Les entrées de fichier sont déclenchées par les boutons de la barre. */
  .file-input {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0 0 0 0);
    border: 0;
  }

  /* --- document -------------------------------------------------------- */
  .document {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 0.4rem 1rem 0;
  }
  .head {
    width: 100%;
    max-width: var(--editor-width);
    padding: 1.1rem 0.9rem 0;
  }
  .document.split .head {
    max-width: calc(var(--editor-width) * 2 + 2rem);
  }

  .title {
    display: block;
    width: 100%;
    padding: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font-family: var(--font-heading);
    font-size: var(--title-size);
    font-weight: 750;
    letter-spacing: -0.03em;
    line-height: 1.2;
  }
  .title::placeholder {
    color: var(--text-faint);
    font-weight: 600;
  }

  .meta-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.5rem 0.8rem;
    margin: 0.7rem 0 0.2rem;
  }
  .folder-pick {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.15rem 0.2rem 0.15rem 0.5rem;
    border-radius: var(--radius-sm);
    color: var(--text-soft);
  }
  .folder-pick:hover {
    background: var(--hover);
  }
  .folder-pick select {
    max-width: 11rem;
    border: none;
    outline: none;
    background: transparent;
    color: inherit;
    font: inherit;
    font-size: 0.82rem;
    cursor: pointer;
  }
  .tags {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.3rem;
  }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.08rem 0.3rem 0.08rem 0.65rem;
    border-radius: 999px;
    background: var(--tint-bg);
    color: var(--tint-ink);
    font-size: 0.76rem;
  }
  .tag-x {
    display: grid;
    place-items: center;
    width: 16px;
    height: 16px;
    border: none;
    border-radius: 50%;
    background: transparent;
    color: inherit;
    opacity: 0.6;
    cursor: pointer;
  }
  .tag-x:hover {
    opacity: 1;
    background: oklch(0 0 0 / 0.1);
  }
  .tag-input {
    width: 6.5rem;
    padding: 0.12rem 0.4rem;
    border: none;
    outline: none;
    border-radius: 999px;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.78rem;
  }
  .tag-input::placeholder {
    color: var(--text-faint);
  }
  .tag-input:focus {
    background: var(--input-bg);
  }

  .attachments {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.4rem 0.6rem;
    margin-top: 0.6rem;
  }
  .attachments-label {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    color: var(--text-faint);
    font-size: 0.74rem;
  }
  .attachment-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem;
  }
  .attachment {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    max-width: 16rem;
    padding: 0.15rem 0.3rem 0.15rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface);
    font-size: 0.76rem;
  }
  .attachment-open {
    min-width: 0;
    padding: 0;
    border: none;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .attachment-open:hover {
    color: var(--accent-text);
    text-decoration: underline;
  }
  .attachment-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .attachment-size {
    color: var(--text-faint);
  }
  .attachment-x {
    display: grid;
    place-items: center;
    width: 18px;
    height: 18px;
    border: none;
    border-radius: 50%;
    background: transparent;
    color: var(--text-soft);
    cursor: pointer;
  }
  .attachment-x:hover {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .attachment-error,
  .attach-notice {
    color: var(--danger);
    font-size: 0.78rem;
  }

  /* --- panneaux d'écriture et d'aperçu ---------------------------------- */
  .panes {
    flex: 1;
    min-height: 0;
    width: 100%;
    display: flex;
    justify-content: center;
    gap: 2rem;
  }
  .pane {
    flex: 1;
    min-width: 0;
    max-width: var(--editor-width);
    min-height: 0;
    overflow-y: auto;
  }
  .document.split .pane {
    max-width: none;
  }
  .document.split .read {
    border-left: 1px solid var(--border);
    padding-left: 2rem;
  }

  textarea {
    display: block;
    width: 100%;
    height: 100%;
    min-height: 100%;
    padding: 1rem 0.9rem 3rem;
    border: none;
    outline: none;
    resize: none;
    background: transparent;
    color: var(--text);
    font-family: var(--editor-font);
    font-size: var(--editor-font-size);
    font-weight: var(--editor-weight);
    letter-spacing: var(--editor-spacing);
    text-align: var(--editor-align);
    line-height: var(--editor-line-height);
    tab-size: 2;
  }
  textarea::placeholder {
    color: var(--text-faint);
  }
  .preview-holder {
    padding: 1rem 0.9rem 3rem;
  }

  /* --- barre d'état ----------------------------------------------------- */
  .statusbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.4rem 1.1rem;
    border-top: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-faint);
    font-size: 0.74rem;
  }
  .cards-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.1rem 0.55rem;
    border: none;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font: inherit;
    font-size: 0.72rem;
    font-weight: 600;
    cursor: pointer;
  }
  .cards-chip:hover {
    background: var(--accent);
    color: var(--on-accent);
  }
  .details {
    display: flex;
    align-items: center;
    gap: 0.9rem;
  }

  @media (max-width: 1060px) {
    .seg-label {
      display: none;
    }
  }
</style>
