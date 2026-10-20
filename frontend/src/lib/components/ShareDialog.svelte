<script lang="ts">
  /**
   * Partager des notes SANS connexion Internet (ADR-008). Deux façons :
   * – enregistrer un paquet (clé USB, Bluetooth, dossier partagé, messagerie
   *   locale…) ;
   * – l'envoyer directement à un autre appareil du même réseau (Wi-Fi, point
   *   d'accès, partage de connexion) : lien et QR code, un seul téléchargement.
   */
  import { onMount } from 'svelte';
  import qrcode from 'qrcode-generator';
  import Icon from './Icon.svelte';
  import { api } from '../api/client';
  import type { LanShare, ShareFormat, ShareSaved, ShareScope, ShareStatus } from '../api/types';
  import { shareDialog } from '../stores/ui';
  import { currentView, folders, notes, selectedNote } from '../stores/workspace';
  import { formatBytes } from '../stores/files';

  const initial = $shareDialog;
  let scope = $state<ShareScope>(initial?.scope ?? 'all');
  let format = $state<ShareFormat>('notecore');
  let passphrase = $state('');
  let showPass = $state(false);
  let busy = $state(false);
  let error = $state('');
  let saved = $state<ShareSaved | null>(null);
  let lan = $state<LanShare | null>(null);
  let status = $state<ShareStatus>({ state: 'idle' });
  let remaining = $state(0);
  let copied = $state('');
  let dialogEl = $state<HTMLDivElement | null>(null);

  const note = $derived($selectedNote);
  const folderId = $derived(
    $currentView.kind === 'folder' ? $currentView.folderId : (note?.folderId ?? null)
  );
  const folderName = $derived($folders.find((f) => f.id === folderId)?.name ?? '');
  const folderCount = $derived(folderId ? $notes.filter((n) => n.folderId === folderId).length : 0);

  const options = $derived<Array<{ scope: ShareScope; id: string; label: string; hint: string }>>([
    ...(note ? [{ scope: 'note' as const, id: note.id, label: 'Cette note', hint: note.title || 'Sans titre' }] : []),
    ...(folderId ? [{ scope: 'folder' as const, id: folderId, label: `Le dossier « ${folderName} »`, hint: `${folderCount} note${folderCount > 1 ? 's' : ''}` }] : []),
    { scope: 'all' as const, id: '', label: 'Toutes mes notes', hint: `${$notes.length} note${$notes.length > 1 ? 's' : ''}` }
  ]);
  const chosen = $derived(options.find((o) => o.scope === scope) ?? options[options.length - 1]);

  // Une sélection impossible (note fermée entre-temps) retombe sur « Toutes ».
  $effect(() => {
    if (!options.some((o) => o.scope === scope)) scope = 'all';
  });

  const encryptedAllowed = $derived(format === 'notecore');
  const qrSvg = $derived.by(() => {
    const url = lan?.urls[0];
    if (!url) return '';
    const qr = qrcode(0, 'M');
    qr.addData(url);
    qr.make();
    return qr.createSvgTag({ cellSize: 4, margin: 2, scalable: true });
  });

  function payload() {
    return { scope, id: chosen?.id ?? '', format, passphrase: encryptedAllowed ? passphrase : '' };
  }

  function close(): void {
    if (lan && status.state === 'waiting') void api.shareStop().catch(() => undefined);
    shareDialog.set(null);
  }

  async function saveFile(): Promise<void> {
    busy = true;
    error = '';
    saved = null;
    try {
      const r = await api.shareToFile(payload());
      if (r.saved) saved = r;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Le partage a échoué.';
    } finally {
      busy = false;
    }
  }

  async function startLan(): Promise<void> {
    busy = true;
    error = '';
    try {
      lan = await api.shareStart(payload());
      status = { state: 'waiting' };
    } catch (err) {
      error = err instanceof Error ? err.message : "L'envoi a échoué.";
    } finally {
      busy = false;
    }
  }

  async function stopLan(): Promise<void> {
    await api.shareStop().catch(() => undefined);
    status = { state: 'stopped' };
  }

  async function copy(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      copied = text;
      setTimeout(() => (copied = ''), 1600);
    } catch {
      /* le lien reste sélectionnable */
    }
  }

  // Suivi de l'envoi : état du serveur et compte à rebours.
  $effect(() => {
    if (!lan) return;
    const expires = new Date(lan.expiresAt).getTime();
    const tick = setInterval(async () => {
      remaining = Math.max(0, Math.round((expires - Date.now()) / 1000));
      if (status.state === 'waiting' || status.state === 'idle') {
        try {
          status = await api.shareStatus();
        } catch {
          /* on réessaie au prochain tour */
        }
      }
    }, 1000);
    remaining = Math.max(0, Math.round((expires - Date.now()) / 1000));
    return () => clearInterval(tick);
  });

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

  const mmss = (s: number): string => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
</script>

<div class="overlay" role="presentation" onclick={(e) => e.target === e.currentTarget && close()}>
  <div class="dialog" bind:this={dialogEl} role="dialog" aria-modal="true" aria-labelledby="share-title" tabindex="-1">
    <header>
      <h2 id="share-title"><Icon name="share" size={17} /> Partager sans connexion</h2>
      <button class="x" aria-label="Fermer" onclick={close}><Icon name="x" size={16} /></button>
    </header>

    {#if lan}
      <div class="body lan">
        {#if status.state === 'downloaded'}
          <div class="done"><Icon name="check" size={30} stroke={2.4} /></div>
          <h3>Envoyé</h3>
          <p class="muted">L'autre appareil a reçu {lan.notes} note{lan.notes > 1 ? 's' : ''}. Le lien est désormais fermé.</p>
        {:else if status.state === 'expired' || status.state === 'stopped'}
          <h3>{status.state === 'expired' ? 'Lien expiré' : 'Envoi arrêté'}</h3>
          <p class="muted">Aucune donnée n'est plus partagée. Relancez un envoi pour en créer un nouveau.</p>
        {:else}
          <p class="lead">Sur l'autre appareil, <strong>scannez ce code</strong> ou ouvrez l'adresse. Les deux doivent être sur le même réseau.</p>
          <div class="qr" aria-label="Code QR du lien d'envoi">{@html qrSvg}</div>
          <ul class="urls">
            {#each lan.urls as u (u)}
              <li>
                <code>{u}</code>
                <button onclick={() => void copy(u)}>{copied === u ? 'Copié' : 'Copier'}</button>
              </li>
            {/each}
          </ul>
          <p class="wait"><span class="pulse"></span> En attente de l'autre appareil… <span class="timer">{mmss(remaining)}</span></p>
          <p class="muted small">
            {lan.filename} · {formatBytes(lan.size)}{lan.encrypted ? ' · protégé par mot de passe' : ''}. Avec NoteCore : <strong>Importer → Depuis un autre appareil</strong>, puis collez l'adresse.
            Le lien ne sert qu'une fois.
          </p>
        {/if}
        <div class="row end">
          {#if status.state === 'waiting' || status.state === 'idle'}
            <button class="btn" onclick={() => void stopLan()}>Arrêter l'envoi</button>
          {:else}
            <button class="btn" onclick={() => { lan = null; status = { state: 'idle' }; }}>Nouvel envoi</button>
          {/if}
          <button class="btn primary" onclick={close}>Fermer</button>
        </div>
      </div>
    {:else}
      <div class="body">
        <section>
          <h3>Que partager ?</h3>
          <div class="choices" role="radiogroup" aria-label="Périmètre">
            {#each options as o (o.scope)}
              <button class="choice" class:on={scope === o.scope} role="radio" aria-checked={scope === o.scope} onclick={() => (scope = o.scope)}>
                <strong>{o.label}</strong><small>{o.hint}</small>
              </button>
            {/each}
          </div>
        </section>

        <section>
          <h3>Format</h3>
          <div class="choices two" role="radiogroup" aria-label="Format">
            <button class="choice" class:on={format === 'notecore'} role="radio" aria-checked={format === 'notecore'} onclick={() => (format = 'notecore')}>
              <strong>Paquet NoteCore</strong><small>Notes, dossiers, étiquettes et fichiers joints ; réimportable à l'identique</small>
            </button>
            <button class="choice" class:on={format === 'markdown'} role="radio" aria-checked={format === 'markdown'} onclick={() => (format = 'markdown')}>
              <strong>Markdown (.zip)</strong><small>Lisible partout, sans NoteCore (Obsidian, éditeurs…)</small>
            </button>
          </div>
        </section>

        {#if encryptedAllowed}
          <section>
            <h3>Mot de passe <span class="opt">(facultatif)</span></h3>
            <div class="pass">
              <Icon name="lock" size={15} />
              <input
                type={showPass ? 'text' : 'password'}
                bind:value={passphrase}
                placeholder="Chiffre le paquet : il faudra le saisir à l'import"
                autocomplete="off"
                aria-label="Mot de passe du paquet"
              />
              <button type="button" onclick={() => (showPass = !showPass)}>{showPass ? 'Masquer' : 'Afficher'}</button>
            </div>
            <p class="muted small">AES-256. Conseillé sur un Wi-Fi partagé : le réseau local n'est pas chiffré. Le mot de passe n'est jamais enregistré — s'il est perdu, le paquet est illisible.</p>
          </section>
        {/if}

        {#if error}<p class="error" role="alert">{error}</p>{/if}
        {#if saved}
          <p class="ok" role="status"><Icon name="check" size={14} /> Paquet enregistré : {saved.notes} note{saved.notes > 1 ? 's' : ''}{saved.attachments ? `, ${saved.attachments} fichier${saved.attachments > 1 ? 's' : ''}` : ''}{saved.encrypted ? ', protégé' : ''}.<br /><small>{saved.path}</small></p>
        {/if}

        <div class="actions">
          <button class="action" disabled={busy} onclick={() => void saveFile()}>
            <Icon name="download" size={17} />
            <span><strong>Enregistrer un fichier…</strong><small>Clé USB, Bluetooth, dossier partagé, messagerie locale</small></span>
          </button>
          <button class="action" disabled={busy} onclick={() => void startLan()}>
            <Icon name="wifi" size={17} />
            <span><strong>Envoyer sur le réseau local</strong><small>Lien et QR code ; sans Internet</small></span>
          </button>
        </div>
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
    width: min(37rem, calc(100% - 2rem));
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
    letter-spacing: -0.01em;
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
    padding: 0 1.2rem 1.2rem;
  }
  section {
    padding: 0.8rem 0;
    border-top: 1px solid var(--border);
  }
  h3 {
    margin: 0 0 0.55rem;
    color: var(--text-faint);
    font-size: 0.72rem;
    font-weight: 650;
    letter-spacing: 0.07em;
    text-transform: uppercase;
  }
  .opt {
    text-transform: none;
    letter-spacing: 0;
    font-weight: 500;
  }
  .choices {
    display: grid;
    gap: 0.4rem;
  }
  .choices.two {
    grid-template-columns: 1fr 1fr;
  }
  .choice {
    display: grid;
    gap: 0.15rem;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card-bg);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .choice small {
    color: var(--text-soft);
    font-size: 0.76rem;
    line-height: 1.4;
  }
  .choice:hover {
    border-color: var(--text-faint);
  }
  .choice.on {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .pass {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    height: 36px;
    padding: 0 0.4rem 0 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text-faint);
  }
  .pass:focus-within {
    border-color: var(--accent);
  }
  .pass input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.86rem;
  }
  .pass button {
    padding: 0.2rem 0.6rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.76rem;
    cursor: pointer;
  }
  .muted {
    margin: 0.5rem 0 0;
    color: var(--text-soft);
    line-height: 1.5;
  }
  .small {
    font-size: 0.78rem;
  }
  .actions {
    display: grid;
    gap: 0.5rem;
    margin-top: 0.4rem;
  }
  .action {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    padding: 0.75rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card-bg);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .action:hover:not(:disabled) {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .action:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .action span {
    display: grid;
    gap: 1px;
  }
  .action small {
    color: var(--text-soft);
    font-size: 0.78rem;
  }
  .error {
    margin: 0.6rem 0 0;
    padding: 0.5rem 0.8rem;
    border-radius: var(--radius-sm);
    background: var(--danger-soft);
    color: var(--danger);
    font-size: 0.84rem;
    line-height: 1.45;
  }
  .ok {
    margin: 0.6rem 0 0;
    padding: 0.5rem 0.8rem;
    border-radius: var(--radius-sm);
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 0.84rem;
    line-height: 1.45;
  }
  .ok small {
    color: var(--text-soft);
    overflow-wrap: anywhere;
  }

  .lan {
    display: grid;
    justify-items: center;
    gap: 0.7rem;
    padding-top: 0.4rem;
    text-align: center;
  }
  .lead {
    margin: 0;
    max-width: 26rem;
    color: var(--text-soft);
    line-height: 1.5;
  }
  .qr {
    width: 13rem;
    padding: 0.4rem;
    border-radius: var(--radius);
    background: white;
    box-shadow: var(--shadow-sm);
  }
  .qr :global(svg) {
    display: block;
    width: 100%;
    height: auto;
  }
  .urls {
    display: grid;
    gap: 0.3rem;
    width: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .urls li {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.4rem 0.4rem 0.4rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--code-bg);
  }
  .urls code {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: 0.78rem;
    user-select: all;
  }
  .urls button {
    flex-shrink: 0;
    padding: 0.2rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.76rem;
    cursor: pointer;
  }
  .wait {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0;
    font-size: 0.86rem;
  }
  .timer {
    color: var(--text-faint);
    font-variant-numeric: tabular-nums;
  }
  .pulse {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--accent);
    animation: pulse 1.2s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.3;
    }
  }
  .done {
    display: grid;
    place-items: center;
    width: 64px;
    height: 64px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .lan h3 {
    margin: 0;
    color: var(--text);
    font-size: 1.3rem;
    letter-spacing: -0.02em;
    text-transform: none;
  }
  .row {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.4rem;
  }
  .row.end {
    justify-content: flex-end;
    width: 100%;
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
  @media (max-width: 560px) {
    .choices.two {
      grid-template-columns: 1fr;
    }
  }
</style>
