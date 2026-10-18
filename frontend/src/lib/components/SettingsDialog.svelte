<script lang="ts">
  /**
   * Paramètres : apparence, affichage de l'éditeur et données. Tout ce qui
   * encombrait le pied de la barre latérale vit ici, une seule fois.
   * Mêmes règles d'accessibilité que ConfirmDialog : focus piégé, Échap ferme,
   * le focus revient à l'élément déclencheur.
   */
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { theme, setThemePref } from '../stores/appTheme';
  import FontPicker from './FontPicker.svelte';
  import {
    ACCENT_SWATCHES,
    LIMITS,
    PRESETS,
    applyPreset,
    display,
    resetDisplay,
    setDisplayPref,
    type DisplayPrefs
  } from '../stores/display';
  import { settingsOpen } from '../stores/ui';
  import { chooseBackupToRestore, createBackup, exportAllNotes } from '../stores/workspace';
  import { api } from '../api/client';
  import { refreshReview } from '../stores/review';
  import { openGuide } from '../stores/guide';

  let dialogEl = $state<HTMLDivElement | null>(null);

  const themes = [
    { value: 'light', icon: 'sun', label: 'Clair' },
    { value: 'dark', icon: 'moon', label: 'Sombre' },
    { value: 'system', icon: 'monitor', label: 'Système' }
  ] as const;
  type NumKey = keyof typeof LIMITS;

  const sliders: Array<{ key: NumKey; label: string; fmt: (v: number) => string }> = [
    { key: 'fontSize', label: 'Taille du texte', fmt: (v) => `${v} px` },
    { key: 'lineHeight', label: 'Interligne', fmt: (v) => v.toFixed(2) },
    { key: 'weight', label: 'Graisse', fmt: (v) => String(v) },
    { key: 'letterSpacing', label: 'Espacement des lettres', fmt: (v) => `${v > 0 ? '+' : ''}${v / 100} em` },
    { key: 'paragraphSpacing', label: 'Espace entre paragraphes', fmt: (v) => `${(v / 10).toFixed(1)} em` },
    { key: 'width', label: 'Largeur de la colonne', fmt: (v) => `${v} rem` },
    { key: 'titleSize', label: 'Taille du titre', fmt: (v) => `${(v / 10).toFixed(1)} rem` }
  ];

  const tabs = [
    { id: 'look', label: 'Apparence', icon: 'sliders' },
    { id: 'text', label: 'Texte', icon: 'type' },
    { id: 'review', label: 'Révision', icon: 'layers' },
    { id: 'data', label: 'Données', icon: 'database' },
    { id: 'keys', label: 'Aide', icon: 'command' }
  ] as const;
  let tab = $state<(typeof tabs)[number]['id']>('look');

  // --- révision : réglages enregistrés à chaque changement, bornés par le backend ---
  let newPerDay = $state(20);
  let retention = $state(90);
  let reviewMsg = $state('');

  $effect(() => {
    if (tab !== 'review') return;
    void api.reviewSettings().then((s) => {
      newPerDay = s.newPerDay;
      retention = Math.round(s.retention * 100);
    });
  });

  async function saveReview(): Promise<void> {
    reviewMsg = '';
    try {
      await api.setReviewSettings(newPerDay, retention / 100);
      void refreshReview();
    } catch (err) {
      reviewMsg = err instanceof Error ? err.message : 'Réglage refusé.';
    }
  }

  const radii = [
    { value: 'sharp', label: 'Anguleux' },
    { value: 'soft', label: 'Doux' },
    { value: 'round', label: 'Arrondi' }
  ] as const;

  const shortcuts: Array<[string, string]> = [
    ['Palette de commandes', 'Ctrl K'],
    ['Nouvelle note', 'Ctrl N'],
    ['Enregistrer maintenant', 'Ctrl S'],
    ['Mode concentration', 'Ctrl \\'],
    ['Paramètres', 'Ctrl ,'],
    ['Gras · Italique', 'Ctrl B · Ctrl I'],
    ['Annuler · Rétablir', 'Ctrl Z · Ctrl Maj Z']
  ];

  function close(): void {
    settingsOpen.set(false);
  }

  /** Les actions de données ouvrent leurs propres dialogues système. */
  function run(action: () => Promise<void>): void {
    close();
    void action();
  }

  onMount(() => {
    const restoreTo = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogEl?.querySelector<HTMLElement>('button')?.focus();

    function onKeydown(event: KeyboardEvent): void {
      if (event.key === 'Escape') {
        event.preventDefault();
        close();
        return;
      }
      if (event.key !== 'Tab' || !dialogEl) return;
      const items = Array.from(
        dialogEl.querySelectorAll<HTMLElement>('button, select, [tabindex]:not([tabindex="-1"])')
      ).filter((el) => !el.hasAttribute('disabled'));
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
    document.addEventListener('keydown', onKeydown, true);
    return () => {
      document.removeEventListener('keydown', onKeydown, true);
      restoreTo?.focus();
    };
  });
</script>

<div
  class="overlay"
  role="presentation"
  onclick={(e) => {
    if (e.target === e.currentTarget) close();
  }}
>
  <div
    class="dialog"
    bind:this={dialogEl}
    role="dialog"
    aria-modal="true"
    aria-labelledby="settings-title"
  >
    <header>
      <h2 id="settings-title">Paramètres</h2>
      <button class="close" title="Fermer" aria-label="Fermer les paramètres" onclick={close}>
        <Icon name="x" size={16} />
      </button>
    </header>

    <div class="tabs" role="tablist" aria-label="Catégories">
      {#each tabs as t (t.id)}
        <button
          role="tab"
          id={`tab-${t.id}`}
          aria-selected={tab === t.id}
          aria-controls="settings-panel"
          class:active={tab === t.id}
          onclick={() => (tab = t.id)}
        >
          <Icon name={t.icon} size={15} />
          {t.label}
        </button>
      {/each}
    </div>

    <div class="scroll" id="settings-panel" role="tabpanel" aria-labelledby={`tab-${tab}`}>
      {#if tab === 'look'}
        <section>
          <h3>Thème</h3>
          <div class="segmented wide" role="group" aria-label="Thème">
            {#each themes as t (t.value)}
              <button
                class:active={$theme.pref === t.value}
                aria-pressed={$theme.pref === t.value}
                onclick={() => setThemePref(t.value)}
              >
                <Icon name={t.icon} size={14} />
                {t.label}
              </button>
            {/each}
          </div>
        </section>

        <section>
          <h3>Couleur d'accent</h3>
          <div class="swatches" role="group" aria-label="Couleur d'accent">
            {#each ACCENT_SWATCHES as sw (sw.hue)}
              <button
                class="swatch"
                class:active={$display.accentHue === sw.hue}
                style:--h={sw.hue}
                title={sw.label}
                aria-label={sw.label}
                aria-pressed={$display.accentHue === sw.hue}
                onclick={() => setDisplayPref('accentHue', sw.hue)}
              >
                {#if $display.accentHue === sw.hue}<Icon name="check" size={14} stroke={2.6} />{/if}
              </button>
            {/each}
          </div>
          <div class="slider hue">
            <label for="hue">Teinte personnalisée</label>
            <input
              id="hue"
              type="range"
              min={LIMITS.accentHue.min}
              max={LIMITS.accentHue.max}
              step={LIMITS.accentHue.step}
              value={$display.accentHue}
              oninput={(e) => setDisplayPref('accentHue', Number(e.currentTarget.value))}
            />
            <output>{$display.accentHue}°</output>
          </div>
        </section>

        <section>
          <h3>Interface</h3>
          <FontPicker
            label="Police de l'interface"
            value={$display.uiFont}
            categories={['sans', 'serif', 'system']}
            onchange={(id) => setDisplayPref('uiFont', id)}
          />
          <div class="row">
            <span class="label">Densité des listes</span>
            <div class="segmented" role="group" aria-label="Densité">
              <button
                class:active={$display.density === 'comfortable'}
                aria-pressed={$display.density === 'comfortable'}
                onclick={() => setDisplayPref('density', 'comfortable')}>Aérée</button
              >
              <button
                class:active={$display.density === 'compact'}
                aria-pressed={$display.density === 'compact'}
                onclick={() => setDisplayPref('density', 'compact')}>Compacte</button
              >
            </div>
          </div>
          <div class="row">
            <span class="label">Forme des angles</span>
            <div class="segmented" role="group" aria-label="Forme des angles">
              {#each radii as r (r.value)}
                <button
                  class:active={$display.radius === r.value}
                  aria-pressed={$display.radius === r.value}
                  onclick={() => setDisplayPref('radius', r.value)}>{r.label}</button
                >
              {/each}
            </div>
          </div>
        </section>
      {:else if tab === 'text'}
        <section>
          <h3>Préréglages</h3>
          <div class="presets">
            {#each PRESETS as p (p.id)}
              <button class="preset" onclick={() => applyPreset(p.prefs)}>
                <strong>{p.label}</strong>
                <small>{p.hint}</small>
              </button>
            {/each}
          </div>
        </section>

        <section>
          <h3>Aperçu</h3>
          <div class="sample" style:text-align={$display.align}>
            <h4>Les processus et l'ordonnancement</h4>
            <p>
              Un <strong>processus</strong> est un programme en cours d'exécution. Il a son propre
              espace mémoire, un état et un contexte : <code>fork()</code> en crée un nouveau.
            </p>
            <p>À revoir : la notion de <em>quantum</em> et le tourniquet (round-robin).</p>
          </div>
        </section>

        <section>
          <h3>Polices</h3>
          <FontPicker
            label="Texte des notes"
            value={$display.editorFont}
            onchange={(id) => setDisplayPref('editorFont', id)}
          />
          <FontPicker
            label="Titres"
            value={$display.headingFont}
            allowInterface
            onchange={(id) => setDisplayPref('headingFont', id)}
          />
          <FontPicker
            label="Code"
            value={$display.codeFont}
            categories={['mono', 'system']}
            onchange={(id) => setDisplayPref('codeFont', id)}
          />
        </section>

        <section>
          <h3>Mise en page du texte</h3>
          {#each sliders as sl (sl.key)}
            <div class="slider">
              <label for={`s-${sl.key}`}>{sl.label}</label>
              <input
                id={`s-${sl.key}`}
                type="range"
                min={LIMITS[sl.key].min}
                max={LIMITS[sl.key].max}
                step={LIMITS[sl.key].step}
                value={$display[sl.key as keyof DisplayPrefs] as number}
                oninput={(e) => setDisplayPref(sl.key, Number(e.currentTarget.value))}
              />
              <output>{sl.fmt($display[sl.key as keyof DisplayPrefs] as number)}</output>
            </div>
          {/each}
          <div class="row">
            <span class="label">Alignement</span>
            <div class="segmented" role="group" aria-label="Alignement du texte">
              <button
                class:active={$display.align === 'left'}
                aria-pressed={$display.align === 'left'}
                onclick={() => setDisplayPref('align', 'left')}>À gauche</button
              >
              <button
                class:active={$display.align === 'justify'}
                aria-pressed={$display.align === 'justify'}
                onclick={() => setDisplayPref('align', 'justify')}>Justifié</button
              >
            </div>
          </div>
          <div class="row">
            <span class="label">Correcteur orthographique</span>
            <div class="segmented" role="group" aria-label="Correcteur orthographique">
              <button
                class:active={$display.spellcheck}
                aria-pressed={$display.spellcheck}
                onclick={() => setDisplayPref('spellcheck', true)}>Activé</button
              >
              <button
                class:active={!$display.spellcheck}
                aria-pressed={!$display.spellcheck}
                onclick={() => setDisplayPref('spellcheck', false)}>Désactivé</button
              >
            </div>
          </div>
          <p class="hint foot">
            Réglages locaux : ils ne modifient jamais le contenu de vos notes.
            <button class="link" onclick={resetDisplay}>Rétablir les réglages d'origine</button>
          </p>
        </section>
      {:else if tab === 'review'}
        <section>
          <h3>Rythme</h3>
          <div class="slider">
            <label for="newPerDay">Nouvelles cartes par jour</label>
            <input
              id="newPerDay"
              type="range"
              min="0"
              max="100"
              step="1"
              bind:value={newPerDay}
              onchange={() => void saveReview()}
            />
            <output>{newPerDay}</output>
          </div>
          <div class="slider">
            <label for="retention">Rétention visée</label>
            <input
              id="retention"
              type="range"
              min="70"
              max="99"
              step="1"
              bind:value={retention}
              onchange={() => void saveReview()}
            />
            <output>{retention} %</output>
          </div>
          <p class="hint">
            La rétention visée est la probabilité de se souvenir d'une carte au moment où elle revient.
            Plus elle est haute, plus les révisions sont fréquentes. 90 % est un bon équilibre.
          </p>
          {#if reviewMsg}<p class="hint err" role="alert">{reviewMsg}</p>{/if}
        </section>
        <section>
          <h3>Écrire une carte</h3>
          <dl class="syntax">
            <div>
              <dt><code>{'{{c1::mot caché}}'}</code></dt>
              <dd>Cache le passage. Le numéro regroupe les trous révisés ensemble.</dd>
            </div>
            <div>
              <dt><code>{'{{c1::mot::indice}}'}</code></dt>
              <dd>Même chose, avec un indice affiché à la place.</dd>
            </div>
            <div>
              <dt><code>Question :: Réponse</code></dt>
              <dd>Une ligne = une carte question / réponse.</dd>
            </div>
          </dl>
          <p class="hint">
            Les cartes vivent dans vos notes : modifier le texte met la carte à jour sans perdre la
            progression. Planification : algorithme FSRS, entièrement local.
          </p>
        </section>
      {:else if tab === 'data'}
        <section>
          <h3>Données</h3>
          <p class="hint">
            Tout reste sur cet ordinateur. Une sauvegarde est vérifiée avant d'être restaurée.
          </p>
          <div class="actions">
            <button class="action" onclick={() => run(exportAllNotes)}>
              <Icon name="download" size={16} />
              <span><strong>Exporter les notes</strong><small>Archive Markdown lisible partout</small></span>
            </button>
            <button class="action" onclick={() => run(createBackup)}>
              <Icon name="archive" size={16} />
              <span><strong>Créer une sauvegarde</strong><small>Copie complète et vérifiable</small></span>
            </button>
            <button class="action" onclick={() => run(chooseBackupToRestore)}>
              <Icon name="upload" size={16} />
              <span><strong>Restaurer une sauvegarde</strong><small>Aperçu et confirmation avant tout</small></span>
            </button>
          </div>
        </section>
      {:else}
        <section>
          <h3>Guide</h3>
          <p class="hint">Cinq écrans pour découvrir l'essentiel : écrire, organiser, réviser, sauvegarder.</p>
          <button class="action" onclick={() => { close(); openGuide(); }}>
            <Icon name="notes" size={16} />
            <span><strong>Ouvrir le guide de prise en main</strong><small>Avec une note d'exemple à essayer</small></span>
          </button>
        </section>
        <section>
          <h3>Raccourcis</h3>
          <dl class="keys">
            {#each shortcuts as [label, keys] (label)}
              <div>
                <dt>{label}</dt>
                <dd>
                  {#each keys.split(' ') as k, i (i)}
                    {#if k === '·'}<span class="dot">·</span>{:else}<kbd>{k}</kbd>{/if}
                  {/each}
                </dd>
              </div>
            {/each}
          </dl>
        </section>
      {/if}
    </div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 120;
    display: grid;
    place-items: center;
    background: var(--scrim);
    backdrop-filter: blur(3px);
  }
  .dialog {
    display: flex;
    flex-direction: column;
    width: min(40rem, calc(100% - 2rem));
    height: min(44rem, calc(100vh - 3rem));
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
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
    padding: 1rem 1.2rem 0.8rem;
  }
  h2 {
    margin: 0;
    font-size: 1.05rem;
    font-weight: 650;
    letter-spacing: -0.01em;
  }
  .close {
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
  .close:hover {
    background: var(--hover);
    color: var(--text);
  }
  .scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0 1.2rem 1.2rem;
  }
  section {
    padding: 0.9rem 0;
    border-top: 1px solid var(--border);
  }
  h3 {
    margin: 0 0 0.6rem;
    font-size: 0.72rem;
    font-weight: 650;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    color: var(--text-faint);
  }
  .hint {
    margin: -0.25rem 0 0.7rem;
    color: var(--text-soft);
    font-size: 0.82rem;
    line-height: 1.45;
  }
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.3rem 0;
  }
  .label {
    color: var(--text);
    font-size: 0.88rem;
  }
  .segmented {
    display: inline-flex;
    padding: 3px;
    gap: 2px;
    background: var(--input-bg);
    border-radius: var(--radius);
  }
  .segmented button {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.32rem 0.75rem;
    border: none;
    border-radius: calc(var(--radius) - 3px);
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.82rem;
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
  .actions {
    display: grid;
    gap: 0.4rem;
  }
  .action {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    width: 100%;
    padding: 0.65rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card-bg);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.12s var(--ease),
      background 0.12s var(--ease);
  }
  .action:hover {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .action span {
    display: grid;
    gap: 1px;
  }
  .action strong {
    font-size: 0.88rem;
    font-weight: 600;
  }
  .action small {
    color: var(--text-soft);
    font-size: 0.78rem;
  }
  .keys {
    margin: 0;
    display: grid;
    gap: 0.1rem;
  }
  .keys > div {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.3rem 0;
  }
  dt {
    color: var(--text-soft);
    font-size: 0.85rem;
  }
  dd {
    margin: 0;
    display: inline-flex;
    gap: 0.25rem;
    align-items: center;
  }
  kbd {
    padding: 0.1rem 0.45rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 6px;
    background: var(--chip-bg);
    font-family: var(--font-ui);
    font-size: 0.72rem;
    color: var(--text-soft);
  }
  .dot {
    color: var(--text-faint);
  }

  .tabs {
    display: flex;
    gap: 0.2rem;
    padding: 0 1.2rem;
    border-bottom: 1px solid var(--border);
  }
  .tabs button {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.6rem 0.8rem;
    margin-bottom: -1px;
    border: none;
    border-bottom: 2px solid transparent;
    background: transparent;
    color: var(--text-soft);
    font: inherit;
    font-size: 0.86rem;
    cursor: pointer;
  }
  .tabs button:hover {
    color: var(--text);
  }
  .tabs button.active {
    color: var(--accent-text);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }
  section:first-child {
    border-top: none;
  }
  .segmented.wide {
    width: 100%;
  }
  .segmented.wide button {
    flex: 1;
    justify-content: center;
  }

  .swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-bottom: 0.7rem;
  }
  .swatch {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border: 2px solid transparent;
    border-radius: 50%;
    background: oklch(0.55 0.2 var(--h));
    color: white;
    cursor: pointer;
    transition: transform 0.12s var(--ease);
  }
  .swatch:hover {
    transform: scale(1.08);
  }
  .swatch.active {
    border-color: var(--surface);
    box-shadow: 0 0 0 2px oklch(0.55 0.2 var(--h));
  }

  .slider {
    display: grid;
    grid-template-columns: 11rem 1fr 5rem;
    align-items: center;
    gap: 0.8rem;
    padding: 0.28rem 0;
    font-size: 0.86rem;
  }
  .slider label {
    color: var(--text);
  }
  .slider output {
    color: var(--text-soft);
    font-variant-numeric: tabular-nums;
    text-align: right;
    font-size: 0.8rem;
  }
  input[type='range'] {
    width: 100%;
    accent-color: var(--accent);
    cursor: pointer;
  }
  .slider.hue input {
    background: linear-gradient(
      90deg,
      oklch(0.7 0.18 0),
      oklch(0.7 0.18 60),
      oklch(0.7 0.18 120),
      oklch(0.7 0.18 180),
      oklch(0.7 0.18 240),
      oklch(0.7 0.18 300),
      oklch(0.7 0.18 360)
    );
    height: 6px;
    border-radius: 999px;
    appearance: none;
  }
  .slider.hue input::-webkit-slider-thumb {
    appearance: none;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 2px solid white;
    background: var(--accent);
    box-shadow: var(--shadow-sm);
  }

  .presets {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 0.5rem;
  }
  .preset {
    display: grid;
    gap: 0.1rem;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--card-bg);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.12s var(--ease),
      background 0.12s var(--ease);
  }
  .preset:hover {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .preset strong {
    font-size: 0.86rem;
  }
  .preset small {
    color: var(--text-soft);
    font-size: 0.74rem;
  }

  /* Aperçu : reprend exactement les variables appliquées à l'éditeur. */
  .sample {
    padding: 1rem 1.2rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--app-bg);
    font-family: var(--editor-font);
    font-size: var(--editor-font-size);
    font-weight: var(--editor-weight);
    line-height: var(--editor-line-height);
    letter-spacing: var(--editor-spacing);
  }
  .sample h4 {
    margin: 0 0 0.4em;
    font-family: var(--font-heading);
    font-size: 1.25em;
    letter-spacing: -0.01em;
  }
  .sample p {
    margin: var(--editor-para) 0;
  }
  .sample code {
    padding: 0.05em 0.35em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.88em;
  }
  .err {
    color: var(--danger);
  }
  .syntax {
    margin: 0 0 0.6rem;
    display: grid;
    gap: 0.5rem;
  }
  .syntax > div {
    display: grid;
    grid-template-columns: 13rem 1fr;
    gap: 0.8rem;
    align-items: baseline;
  }
  .syntax dt code {
    padding: 0.1em 0.45em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.8rem;
  }
  .syntax dd {
    margin: 0;
    color: var(--text-soft);
    font-size: 0.84rem;
    line-height: 1.45;
  }
  .foot {
    margin: 0.8rem 0 0;
  }
  .link {
    margin-left: 0.4rem;
    padding: 0;
    border: none;
    background: none;
    color: var(--accent-text);
    font: inherit;
    text-decoration: underline;
    cursor: pointer;
  }
</style>
