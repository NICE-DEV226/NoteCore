<script lang="ts">
  /**
   * Importer des notes : depuis un fichier (paquet NoteCore, archive ou fichier
   * Markdown) ou depuis un autre appareil du réseau local. Rien n'est jamais
   * écrasé : une note déjà présente est ignorée, une note différente de même
   * titre est ajoutée à côté.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { api } from '../api/client';
  import type { ImportReport, ImportResult } from '../api/types';
  import { importOpen } from '../stores/ui';
  import { reloadData } from '../stores/workspace';
  import { refreshAttachmentIndex } from '../stores/files';

  let tab = $state<'file' | 'device'>('file');
  let url = $state('');
  let passphrase = $state('');
  let pendingId = $state('');
  let wrong = $state(false);
  let busy = $state(false);
  let error = $state('');
  let report = $state<ImportReport | null>(null);
  let dialogEl = $state<HTMLDivElement | null>(null);

  function close(): void {
    if (pendingId) void api.cancelPending().catch(() => undefined);
    importOpen.set(false);
  }

  async function handle(run: () => Promise<ImportResult>): Promise<void> {
    busy = true;
    error = '';
    try {
      const r = await run();
      if (r.cancelled) return;
      if (r.needsPassphrase) {
        pendingId = r.pendingId;
        wrong = r.wrongPassphrase;
        passphrase = '';
        return;
      }
      pendingId = '';
      wrong = false;
      report = r.report;
      await reloadData();
      void refreshAttachmentIndex();
    } catch (err) {
      error = err instanceof Error ? err.message : "L'import a échoué.";
    } finally {
      busy = false;
    }
  }

  const pickFile = () => handle(() => api.importFromFile());
  const receive = () => handle(() => api.importFromUrl(url));
  const unlock = () => handle(() => api.importPending(pendingId, passphrase));

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.focus();
    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.preventDefault();
        close();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });

  const plural = (n: number, one: string, many: string): string => `${n} ${n > 1 ? many : one}`;
</script>

<div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div class="dialog" bind:this={dialogEl} role="dialog" aria-modal="true" aria-labelledby="import-title" tabindex="-1">
    <header>
      <h2 id="import-title"><Icon name="upload" size={17} /> Importer des notes</h2>
      <button class="x" aria-label="Fermer" onclick={close}><Icon name="x" size={16} /></button>
    </header>

    {#if report}
      <div class="body result">
        <div class="badge"><Icon name="check" size={28} stroke={2.4} /></div>
        <h3>{report.imported > 0 ? `${plural(report.imported, 'note importée', 'notes importées')}` : 'Rien de nouveau à importer'}</h3>
        <ul class="facts">
          {#if report.skipped > 0}<li>{plural(report.skipped, 'note déjà présente, ignorée', 'notes déjà présentes, ignorées')}</li>{/if}
          {#if report.renamed > 0}<li>{plural(report.renamed, 'note ajoutée à côté', 'notes ajoutées à côté')} d'une note de même titre (« (importée) »)</li>{/if}
          {#if report.foldersCreated > 0}<li>{plural(report.foldersCreated, 'dossier créé', 'dossiers créés')}</li>{/if}
          {#if report.attachments > 0}<li>{plural(report.attachments, 'fichier joint', 'fichiers joints')}</li>{/if}
          {#if report.attachmentsSkipped > 0}<li class="warn">{plural(report.attachmentsSkipped, 'fichier joint ignoré', 'fichiers joints ignorés')} (trop volumineux ou absents du paquet)</li>{/if}
        </ul>
        {#if report.titles.length}
          <p class="titles">{report.titles.join(' · ')}{report.imported > report.titles.length ? ' …' : ''}</p>
        {/if}
        <p class="muted">Vos notes existantes n'ont pas été modifiées.</p>
        <div class="row end"><button class="btn primary" onclick={close}>Terminer</button></div>
      </div>
    {:else if pendingId}
      <div class="body">
        <div class="lock"><Icon name="lock" size={26} /></div>
        <p class="lead">Ce paquet est protégé. Saisissez son mot de passe.</p>
        <form onsubmit={(e) => { e.preventDefault(); void unlock(); }}>
          <input type="password" bind:value={passphrase} placeholder="Mot de passe" autocomplete="off" aria-label="Mot de passe du paquet" />
          {#if wrong}<p class="error" role="alert">Mot de passe incorrect, ou fichier altéré.</p>{/if}
          {#if error}<p class="error" role="alert">{error}</p>{/if}
          <div class="row end">
            <button type="button" class="btn" onclick={() => { void api.cancelPending(); pendingId = ''; }}>Annuler</button>
            <button type="submit" class="btn primary" disabled={busy || !passphrase}>Ouvrir</button>
          </div>
        </form>
      </div>
    {:else}
      <div class="tabs" role="tablist">
        <button role="tab" aria-selected={tab === 'file'} class:on={tab === 'file'} onclick={() => (tab = 'file')}>Depuis un fichier</button>
        <button role="tab" aria-selected={tab === 'device'} class:on={tab === 'device'} onclick={() => (tab = 'device')}>Depuis un autre appareil</button>
      </div>
      <div class="body">
        {#if tab === 'file'}
          <p class="lead">Choisissez un <strong>paquet NoteCore</strong> (<code>.notecore</code>), une <strong>archive</strong> d'export (<code>.zip</code>) ou des fichiers <strong>Markdown</strong> (<code>.md</code>) venant d'un autre outil.</p>
          <button class="action" disabled={busy} onclick={() => void pickFile()}>
            <Icon name="upload" size={17} />
            <span><strong>Choisir un fichier…</strong><small>Les en-têtes YAML (titre, étiquettes) sont reconnus</small></span>
          </button>
        {:else}
          <p class="lead">Sur l'appareil qui envoie, lancez <strong>Partager → Envoyer sur le réseau local</strong>, puis collez ici l'adresse affichée. Les deux appareils doivent être sur le même réseau ; Internet n'est pas nécessaire.</p>
          <form onsubmit={(e) => { e.preventDefault(); void receive(); }}>
            <div class="field">
              <Icon name="wifi" size={15} />
              <input bind:value={url} placeholder="http://192.168.1.24:41823/s/…" aria-label="Adresse de l'envoi" spellcheck="false" autocomplete="off" />
            </div>
            <div class="row end"><button type="submit" class="btn primary" disabled={busy || !url.trim()}>{busy ? 'Réception…' : 'Recevoir'}</button></div>
          </form>
          <p class="muted small">Seules les adresses de votre réseau local sont acceptées : NoteCore ne contacte jamais Internet.</p>
        {/if}
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        <p class="muted small">Importer n'écrase rien : une note déjà présente est ignorée, une note différente de même titre est ajoutée à côté.</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 130;
    display: grid;
    place-items: center;
    background: var(--scrim);
    backdrop-filter: blur(3px);
  }
  .dialog {
    display: flex;
    flex-direction: column;
    width: min(33rem, calc(100% - 2rem));
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
    padding: 1rem 1.2rem 0.6rem;
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
  .tabs {
    display: flex;
    gap: 0.2rem;
    padding: 0 1.2rem;
    border-bottom: 1px solid var(--border);
  }
  .tabs button {
    padding: 0.55rem 0.8rem;
    margin-bottom: -1px;
    border: none;
    border-bottom: 2px solid transparent;
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.86rem;
    cursor: pointer;
  }
  .tabs button.on {
    border-bottom-color: var(--accent);
    color: var(--accent-text);
    font-weight: 600;
  }
  .body {
    display: grid;
    gap: 0.8rem;
    overflow-y: auto;
    padding: 1rem 1.2rem 1.2rem;
  }
  .lead {
    margin: 0;
    color: var(--text-soft);
    line-height: 1.55;
    font-size: 0.9rem;
  }
  code {
    padding: 0.05em 0.4em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.85em;
  }
  .action {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    padding: 0.8rem 0.9rem;
    border: 1px dashed var(--accent);
    border-radius: var(--radius);
    background: var(--accent-soft);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .action:hover:not(:disabled) {
    background: var(--surface);
  }
  .action:disabled {
    opacity: 0.55;
  }
  .action span {
    display: grid;
    gap: 1px;
  }
  .action small {
    color: var(--text-soft);
    font-size: 0.78rem;
  }
  .field {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    height: 38px;
    padding: 0 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text-faint);
  }
  .field:focus-within {
    border-color: var(--accent);
  }
  .field input,
  form > input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.88rem;
  }
  form {
    display: grid;
    gap: 0.6rem;
  }
  form > input {
    height: 38px;
    padding: 0 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--input-bg);
  }
  form > input:focus {
    border-color: var(--accent);
  }
  .muted {
    margin: 0;
    color: var(--text-faint);
    line-height: 1.5;
    font-size: 0.82rem;
  }
  .small {
    font-size: 0.78rem;
  }
  .error {
    margin: 0;
    padding: 0.5rem 0.8rem;
    border-radius: var(--radius-sm);
    background: var(--danger-soft);
    color: var(--danger);
    font-size: 0.84rem;
    line-height: 1.45;
  }
  .row {
    display: flex;
    gap: 0.5rem;
  }
  .row.end {
    justify-content: flex-end;
  }
  .btn {
    height: 36px;
    padding: 0 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
    font-weight: 600;
    cursor: pointer;
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
  .result {
    justify-items: center;
    text-align: center;
  }
  .badge,
  .lock {
    display: grid;
    place-items: center;
    width: 60px;
    height: 60px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .lock {
    justify-self: center;
  }
  .result h3 {
    margin: 0;
    font-size: 1.25rem;
    letter-spacing: -0.02em;
  }
  .facts {
    display: grid;
    gap: 0.25rem;
    margin: 0;
    padding: 0;
    list-style: none;
    color: var(--text-soft);
    font-size: 0.88rem;
  }
  .facts .warn {
    color: var(--warning);
  }
  .titles {
    margin: 0;
    max-width: 100%;
    color: var(--text-soft);
    font-size: 0.82rem;
    font-style: italic;
  }
  .result .row {
    width: 100%;
  }
</style>
