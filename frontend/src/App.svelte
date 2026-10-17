<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import logoUrl from './lib/assets/logo.svg';
  import Workbench from './routes/Workbench.svelte';
  import { initWorkspace, loaded } from './lib/stores/workspace';
  import { watchSystemThemeChanges, loadPreferences } from './lib/stores/appTheme';
  import { loadDisplayPrefs } from './lib/stores/display';

  type BootState = 'loading' | 'ready' | 'error';
  const MIN_SPLASH_MS = 900;

  let bootState = $state<BootState>('loading');
  let initError = $state<string | null>(null);
  let bootAttempt = 0;

  function startWorkspace(): void {
    const attempt = ++bootAttempt;
    bootState = 'loading';
    initError = null;

    // L'appel Wails peut bloquer le thread JS avant de rendre sa Promise.
    // On laisse d'abord le navigateur peindre le Workbench, puis on lance
    // l'initialisation au frame suivant.
    setTimeout(() => {
      if (attempt !== bootAttempt) return;
      bootState = 'ready';
      // L'interface reste utilisable même si le bridge Wails tarde à répondre.
      loaded.set(true);
      requestAnimationFrame(() => {
        void initWorkspace().catch((err) => {
          if (attempt !== bootAttempt) return;
          console.warn('initWorkspace a échoué :', err);
          initError = err instanceof Error ? err.message : 'Impossible de charger les données.';
          loaded.set(true);
        });
      });
    }, MIN_SPLASH_MS);
  }

  onMount(() => {
    try {
      watchSystemThemeChanges();
    } catch (err) {
      console.warn('Impossible de suivre le thème système :', err);
    }
    // Les préférences persistées (table `settings`) affinent l'amorce
    // locale, sans bloquer l'ouverture de l'espace de travail.
    void loadPreferences();
    void loadDisplayPrefs();
    void startWorkspace();
  });
</script>

{#if bootState === 'ready'}
  <div transition:fade={{ duration: 250 }}>
    <Workbench />
  </div>
{:else}
  <main class="splash" aria-busy={bootState === 'loading'}>
    <section class="splash-content" aria-label="NoteCore">
      <img class="splash-logo" src={logoUrl} alt="Logo NoteCore" />

      {#if initError}
        <p class="splash-status splash-status-error" role="alert">{initError}</p>
        <button class="retry-button" onclick={() => void startWorkspace()}>Réessayer</button>
      {:else}
        <p class="splash-status" role="status">Ouverture de votre espace de travail…</p>
        <span class="splash-bar" aria-hidden="true"><span></span></span>
      {/if}
    </section>
  </main>
{/if}

<style>
  /* Splash : fond blanc fixe (le logo est noir), typographie encre */
  .splash {
    display: grid;
    place-items: center;
    height: 100vh;
    background: #ffffff;
    color: oklch(0.24 0.012 250);
  }

  .splash-content {
    display: grid;
    justify-items: center;
    width: min(24rem, calc(100% - 3rem));
    text-align: center;
  }

  .splash-logo {
    width: min(14rem, 72vw);
    height: auto;
    max-height: 5.5rem;
  }

  .splash-status {
    max-width: 20rem;
    margin: 1.5rem 0 0;
    color: oklch(0.6 0.012 250);
    font-size: 0.9rem;
    line-height: 1.45;
  }

  .splash-bar {
    width: 8rem;
    height: 3px;
    margin-top: 1rem;
    overflow: hidden;
    display: block;
    background: oklch(0.912 0.008 250);
    border-radius: 999px;
  }

  .splash-bar > span {
    display: block;
    height: 100%;
    width: 42%;
    background: var(--accent);
    border-radius: inherit;
    animation: sweep 1.4s ease-in-out infinite;
  }

  .splash-status-error {
    color: var(--danger);
  }

  .retry-button {
    margin-top: 1rem;
    padding: 0.55rem 1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
  }

  .retry-button:hover {
    border-color: var(--accent);
    background: var(--hover);
  }

  @keyframes sweep {
    from { transform: translateX(-120%); }
    to   { transform: translateX(320%); }
  }

  @media (prefers-reduced-motion: reduce) {
    .splash-bar > span { animation: none; }
  }
</style>