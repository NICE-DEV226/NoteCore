<script lang="ts">
  /**
   * Guide de prise en main : cinq écrans courts, au clavier (← → Échap).
   * Il explique l'essentiel, montre où vivent les données et propose de créer
   * une note d'exemple qui contient déjà des cartes de révision.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import Brand from './Brand.svelte';
  import { api } from '../api/client';
  import type { AppInfo } from '../api/types';
  import { closeGuide } from '../stores/guide';
  import { createNote, updateSelected } from '../stores/workspace';

  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
  const mod = isMac ? '⌘' : 'Ctrl';

  const STEPS = [
    { id: 'welcome', label: 'Bienvenue' },
    { id: 'write', label: 'Écrire' },
    { id: 'find', label: 'Organiser' },
    { id: 'review', label: 'Réviser' },
    { id: 'data', label: 'Vos données' }
  ] as const;

  let step = $state(0);
  let info = $state<AppInfo | null>(null);
  let copied = $state(false);
  let dialogEl = $state<HTMLDivElement | null>(null);

  const last = $derived(step === STEPS.length - 1);

  const SAMPLE_TITLE = 'Bienvenue dans NoteCore';
  const SAMPLE = `# Bienvenue dans NoteCore

Ceci est une note d'exemple : modifiez-la ou supprimez-la, elle est à vous.

## Écrire

Le texte est du **Markdown** : *italique*, ==surlignage==, \`code\`, listes, citations, tableaux…

- Le titre se tape en haut, l'enregistrement est automatique.
- Passez en mode « Scinder » pour voir le rendu en direct.

## Réviser

Cachez un mot pour vous tester : le {{c1::noyau}} gère les {{c2::processus}} et la mémoire.

Quelle touche retourne une carte ? :: Espace

Cliquez sur « Réviser » dans la barre latérale pour essayer ces trois cartes.
`;

  /** Chemin à montrer : les dossiers de données de chaque système, en clair. */
  const where = $derived.by(() => {
    if (info?.dataDir) return info.dataDir;
    return info?.os === 'windows' ? '%APPDATA%\\NoteCore' : '~/.config/NoteCore';
  });

  function next(): void {
    if (last) finish();
    else step += 1;
  }
  function prev(): void {
    if (step > 0) step -= 1;
  }
  function finish(): void {
    closeGuide();
  }

  async function createSample(): Promise<void> {
    closeGuide();
    await createNote(null);
    updateSelected({ title: SAMPLE_TITLE, content: SAMPLE, tags: ['guide'] });
  }

  async function copyPath(): Promise<void> {
    try {
      await navigator.clipboard.writeText(where);
      copied = true;
      setTimeout(() => (copied = false), 1800);
    } catch {
      /* presse-papiers indisponible : le chemin reste affiché et sélectionnable */
    }
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.focus();
    void api.appInfo().then((i) => (info = i)).catch(() => undefined);

    function onKeydown(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.preventDefault();
        finish();
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        if (!last) step += 1;
      } else if (e.key === 'ArrowLeft') {
        e.preventDefault();
        prev();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

<div class="overlay" role="presentation">
  <div class="guide" bind:this={dialogEl} role="dialog" aria-modal="true" aria-labelledby="guide-title" tabindex="-1">
    <button class="skip" onclick={finish} title="Fermer le guide — Échap">
      Passer
      <Icon name="x" size={14} />
    </button>

    <div class="stage">
      {#key step}
        <div class="slide">
          {#if step === 0}
            <Brand size={44} />
            <h2 id="guide-title">Un carnet qui reste chez vous.</h2>
            <p class="lead">
              NoteCore range vos cours et vos idées en Markdown, sans compte ni serveur. Tout fonctionne
              hors ligne, et il vous aide à retenir ce que vous écrivez.
            </p>
            <ul class="points">
              <li><Icon name="database" size={16} /><span><strong>Local.</strong> Vos notes sont sur cet ordinateur, nulle part ailleurs.</span></li>
              <li><Icon name="layers" size={16} /><span><strong>Utile pour apprendre.</strong> Transformez une phrase en carte de révision.</span></li>
              <li><Icon name="archive" size={16} /><span><strong>Sûr.</strong> Sauvegardes vérifiées, exports en Markdown lisible partout.</span></li>
            </ul>
          {:else if step === 1}
            <div class="kicker"><Icon name="edit" size={15} /> Écrire</div>
            <h2 id="guide-title">Écrivez, le reste se fait seul.</h2>
            <p class="lead">
              Tapez en Markdown ou utilisez la barre d'outils. L'enregistrement est automatique : l'état
              s'affiche en haut de la note.
            </p>
            <div class="modes" aria-hidden="true">
              <span class="chip on"><Icon name="edit" size={13} /> Éditer</span>
              <span class="chip"><Icon name="columns" size={13} /> Scinder</span>
              <span class="chip"><Icon name="eye" size={13} /> Aperçu</span>
            </div>
            <ul class="points">
              <li><Icon name="type" size={16} /><span>Titres <code>#</code>, <strong>gras</strong> <code>**</code>, <em>italique</em> <code>*</code>, listes, citations, tableaux, code.</span></li>
              <li><Icon name="paperclip" size={16} /><span>Joignez des images et des fichiers depuis la barre d'outils.</span></li>
              <li><Icon name="undo" size={16} /><span><kbd>{mod}</kbd> <kbd>Z</kbd> annule, <kbd>{mod}</kbd> <kbd>B</kbd> met en gras, <kbd>{mod}</kbd> <kbd>\</kbd> masque tout sauf le texte.</span></li>
            </ul>
          {:else if step === 2}
            <div class="kicker"><Icon name="folder" size={15} /> Organiser et retrouver</div>
            <h2 id="guide-title">Retrouvez n'importe quoi en un instant.</h2>
            <p class="lead">
              Classez dans des dossiers, ajoutez des étiquettes, épinglez vos favoris. Et pour tout le reste :
              la palette de commandes.
            </p>
            <div class="palette" aria-hidden="true">
              <Icon name="search" size={15} />
              <span>Rechercher une note ou une commande…</span>
              <kbd>{mod} K</kbd>
            </div>
            <ul class="points">
              <li><Icon name="search" size={16} /><span><kbd>{mod}</kbd> <kbd>K</kbd> cherche dans le titre, le contenu et les étiquettes, et lance des actions.</span></li>
              <li><Icon name="plus" size={16} /><span><kbd>{mod}</kbd> <kbd>N</kbd> crée une note ; <kbd>{mod}</kbd> <kbd>,</kbd> ouvre les paramètres.</span></li>
              <li><Icon name="inbox" size={16} /><span>Les notes sans dossier arrivent dans <strong>À classer</strong>, avec un dossier proposé : un clic pour ranger. Glissez aussi vos notes dans les dossiers.</span></li>
              <li><Icon name="sliders" size={16} /><span>Dans les paramètres : 22 polices, couleurs, tailles, thème clair ou sombre.</span></li>
            </ul>
          {:else if step === 3}
            <div class="kicker"><Icon name="layers" size={15} /> Réviser</div>
            <h2 id="guide-title">Écrivez une carte, NoteCore planifie la révision.</h2>
            <p class="lead">
              Il n'y a rien à recopier : les cartes vivent dans la note. Cachez un mot, ou posez une question.
            </p>
            <div class="example">
              <div>
                <span class="tag">Vous écrivez</span>
                <p>Le <code>{'{{c1::noyau}}'}</code> gère les processus.</p>
                <p><code>Quelle touche retourne une carte ? :: Espace</code></p>
              </div>
              <div class="arrow"><Icon name="chevron-right" size={18} /></div>
              <div>
                <span class="tag">Vous révisez</span>
                <p>Le <span class="blank"></span> gère les processus.</p>
                <p><strong>Quelle touche retourne une carte ?</strong></p>
              </div>
            </div>
            <ul class="points">
              <li><Icon name="play" size={16} /><span>Bouton « Réviser » dans la barre latérale. <kbd>Espace</kbd> retourne, <kbd>1</kbd>–<kbd>4</kbd> répondent.</span></li>
              <li><Icon name="rotate" size={16} /><span>L'algorithme FSRS espace les révisions selon vos réponses, en local.</span></li>
            </ul>
          {:else}
            <div class="kicker"><Icon name="database" size={15} /> Vos données</div>
            <h2 id="guide-title">Tout est dans un seul dossier.</h2>
            <p class="lead">Voici où NoteCore range votre base. Mettre à jour ou désinstaller l'application ne la touche jamais.</p>
            <div class="path">
              <code>{where}</code>
              <button onclick={() => void copyPath()} title="Copier le chemin">
                <Icon name={copied ? 'check' : 'notes'} size={14} />
                {copied ? 'Copié' : 'Copier'}
              </button>
            </div>
            <ul class="points">
              <li><Icon name="archive" size={16} /><span><strong>Sauvegarde :</strong> Paramètres → Données. Une sauvegarde est vérifiée avant d'être restaurée.</span></li>
              <li><Icon name="download" size={16} /><span><strong>Export :</strong> toutes vos notes en Markdown, lisibles sans NoteCore.</span></li>
              <li><Icon name="share" size={16} /><span><strong>Partage :</strong> envoyez des notes à un camarade par fichier ou sur le Wi-Fi local, sans Internet.</span></li>
              <li><Icon name="x" size={16} /><span><strong>Aucune connexion à Internet :</strong> ni compte, ni télémétrie, ni synchronisation.</span></li>
            </ul>
            <button class="sample" onclick={() => void createSample()}>
              <Icon name="plus-circle" size={16} />
              <span><strong>Créer une note d'exemple</strong><small>Avec trois cartes de révision à essayer</small></span>
            </button>
          {/if}
        </div>
      {/key}
    </div>

    <footer>
      <div class="dots" role="tablist" aria-label="Étapes du guide">
        {#each STEPS as s, i (s.id)}
          <button
            role="tab"
            class:active={i === step}
            aria-selected={i === step}
            aria-label={`Étape ${i + 1} : ${s.label}`}
            onclick={() => (step = i)}
          ></button>
        {/each}
      </div>
      <div class="nav">
        {#if step > 0}
          <button class="ghost" onclick={prev}>Précédent</button>
        {/if}
        <button class="primary" onclick={next}>{last ? 'Commencer' : 'Suivant'}</button>
      </div>
    </footer>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 170;
    display: grid;
    place-items: center;
    background: var(--scrim);
    backdrop-filter: blur(4px);
  }
  .guide {
    position: relative;
    display: flex;
    flex-direction: column;
    width: min(40rem, calc(100% - 2rem));
    min-height: min(33rem, calc(100vh - 2rem));
    max-height: calc(100vh - 2rem);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    outline: none;
    animation: pop 0.18s var(--ease);
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(8px) scale(0.985);
    }
  }
  .skip {
    position: absolute;
    top: 0.8rem;
    right: 0.9rem;
    z-index: 2;
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    padding: 0.25rem 0.6rem;
    border: none;
    border-radius: 999px;
    background: transparent;
    color: var(--text-faint);
    font: inherit;
    font-size: 0.78rem;
    cursor: pointer;
  }
  .skip:hover {
    background: var(--hover);
    color: var(--text);
  }

  .stage {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 2.4rem 2.6rem 1rem;
  }
  .slide {
    display: grid;
    gap: 0.9rem;
    animation: slide 0.22s var(--ease);
  }
  @keyframes slide {
    from {
      opacity: 0;
      transform: translateX(10px);
    }
  }
  .kicker {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--accent-text);
    font-size: 0.74rem;
    font-weight: 650;
    letter-spacing: 0.07em;
    text-transform: uppercase;
  }
  h2 {
    margin: 0;
    font-family: var(--font-heading);
    font-size: 1.75rem;
    font-weight: 750;
    letter-spacing: -0.03em;
    line-height: 1.15;
  }
  .lead {
    margin: 0;
    color: var(--text-soft);
    font-size: 0.98rem;
    line-height: 1.55;
  }
  .points {
    display: grid;
    gap: 0.6rem;
    margin: 0.3rem 0 0;
    padding: 0;
    list-style: none;
  }
  .points li {
    display: grid;
    grid-template-columns: 22px 1fr;
    gap: 0.7rem;
    align-items: start;
    color: var(--text);
    font-size: 0.9rem;
    line-height: 1.5;
  }
  .points :global(svg) {
    margin-top: 0.15rem;
    color: var(--accent-text);
  }
  code {
    padding: 0.08em 0.4em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.84em;
  }
  kbd {
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 5px;
    background: var(--chip-bg);
    font-family: var(--font-ui);
    font-size: 0.74rem;
    color: var(--text-soft);
  }

  .modes {
    display: inline-flex;
    gap: 0.3rem;
    padding: 3px;
    width: fit-content;
    background: var(--input-bg);
    border-radius: var(--radius);
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.3rem 0.7rem;
    border-radius: calc(var(--radius) - 3px);
    color: var(--text-soft);
    font-size: 0.8rem;
  }
  .chip.on {
    background: var(--surface-elevated);
    color: var(--text);
    box-shadow: var(--shadow-sm);
    font-weight: 600;
  }

  .palette {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.7rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--input-bg);
    color: var(--text-faint);
    font-size: 0.9rem;
  }
  .palette span {
    flex: 1;
  }

  .example {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: stretch;
    gap: 0.6rem;
  }
  .example > div:not(.arrow) {
    padding: 0.8rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card-bg);
    font-size: 0.86rem;
    line-height: 1.5;
  }
  .example p {
    margin: 0.3rem 0;
  }
  .example .tag {
    color: var(--text-faint);
    font-size: 0.68rem;
    font-weight: 650;
    letter-spacing: 0.07em;
    text-transform: uppercase;
  }
  .arrow {
    display: grid;
    place-items: center;
    color: var(--accent-text);
  }
  .blank {
    display: inline-block;
    min-width: 3.5em;
    height: 1em;
    vertical-align: -0.15em;
    border-bottom: 2px solid var(--accent);
    border-radius: 3px 3px 0 0;
    background: var(--accent-soft);
  }

  .path {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    padding: 0.6rem 0.7rem 0.6rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--code-bg);
  }
  .path code {
    padding: 0;
    background: none;
    overflow-wrap: anywhere;
    user-select: all;
    font-size: 0.86rem;
  }
  .path button {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    flex-shrink: 0;
    padding: 0.3rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.78rem;
    cursor: pointer;
  }
  .path button:hover {
    border-color: var(--accent);
  }

  .sample {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    padding: 0.7rem 0.9rem;
    border: 1px dashed var(--accent);
    border-radius: var(--radius);
    background: var(--accent-soft);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .sample:hover {
    background: var(--surface);
  }
  .sample :global(svg) {
    color: var(--accent-text);
  }
  .sample span {
    display: grid;
    gap: 1px;
  }
  .sample small {
    color: var(--text-soft);
    font-size: 0.78rem;
  }

  footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.9rem 1.6rem 1.2rem;
  }
  .dots {
    display: flex;
    gap: 0.4rem;
  }
  .dots button {
    width: 8px;
    height: 8px;
    padding: 0;
    border: none;
    border-radius: 999px;
    background: var(--border);
    cursor: pointer;
    transition:
      width 0.18s var(--ease),
      background 0.18s var(--ease);
  }
  .dots button.active {
    width: 22px;
    background: var(--accent);
  }
  .nav {
    display: flex;
    gap: 0.5rem;
  }
  .nav button {
    height: 36px;
    padding: 0 1.1rem;
    border-radius: var(--radius);
    font: inherit;
    font-size: 0.88rem;
    font-weight: 600;
    cursor: pointer;
  }
  .ghost {
    border: none;
    background: transparent;
    color: var(--text-soft);
  }
  .ghost:hover {
    background: var(--hover);
    color: var(--text);
  }
  .primary {
    border: none;
    background: var(--accent);
    color: var(--on-accent);
  }
  .primary:hover {
    background: var(--accent-strong);
  }

  @media (max-width: 560px) {
    .stage {
      padding: 2.4rem 1.4rem 1rem;
    }
    .example {
      grid-template-columns: 1fr;
    }
    .arrow {
      transform: rotate(90deg);
    }
  }
</style>
