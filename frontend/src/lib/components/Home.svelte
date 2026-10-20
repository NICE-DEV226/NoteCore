<script lang="ts">
  /**
   * Accueil : ce qu'on voit quand aucune note n'est ouverte. Reprendre où l'on
   * s'est arrêté, retrouver ses notes épinglées, mesurer ce que contient
   * l'espace de travail. Toutes les valeurs viennent des données réelles.
   */
  import Icon from './Icon.svelte';
  import { allTags, createNote, currentView, folders, inboxCount, notes, selectNote } from '../stores/workspace';
  import { paletteOpen } from '../stores/ui';
  import { mastery, reviewStats, startReview } from '../stores/review';
  import { openGuide } from '../stores/guide';
  import { countWords, excerpt, formatRelative, formatUntil } from '../utils/format';
  import { tagTint } from '../utils/tags';

  const hour = new Date().getHours();
  const greeting = hour < 5 ? 'Bonne nuit' : hour < 18 ? 'Bonjour' : 'Bonsoir';
  const todayRaw = new Date().toLocaleDateString('fr-FR', {
    weekday: 'long',
    day: 'numeric',
    month: 'long'
  });
  const today = todayRaw.charAt(0).toUpperCase() + todayRaw.slice(1);

  const recent = $derived(
    [...$notes].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 6)
  );
  const pinned = $derived($notes.filter((n) => n.isPinned).slice(0, 5));
  const totalWords = $derived($notes.reduce((sum, n) => sum + countWords(n.content), 0));

  const tiles = $derived([
    { label: 'Notes', value: $notes.length, icon: 'notes', tint: 'blue' },
    { label: 'Dossiers', value: $folders.length, icon: 'folder', tint: 'green' },
    { label: 'Mots écrits', value: totalWords, icon: 'type', tint: 'yellow' },
    { label: 'Étiquettes', value: $allTags.length, icon: 'tag', tint: 'purple' }
  ]);

  const nf = new Intl.NumberFormat('fr-FR');

  const toSee = $derived(($reviewStats?.due ?? 0) + ($reviewStats?.new ?? 0));
  const hasCards = $derived(($reviewStats?.total ?? 0) > 0);
  const maxActivity = $derived(Math.max(1, ...($reviewStats?.activity ?? [0])));
  const masteryRows = $derived(
    $mastery.filter((r) => r.learned > 0).sort((a, b) => b.due - a.due || a.retention - b.retention).slice(0, 6)
  );
  const tone = (r: number): string => (r >= 0.85 ? 'good' : r >= 0.7 ? 'mid' : 'low');
</script>

<div class="home">
  <div class="inner">
    <header>
      <p class="date">{today}</p>
      <h1>{greeting}.</h1>
      <p class="lead">
        {#if $notes.length === 0}
          Votre carnet est vide. Une première note, et tout commence.
        {:else}
          Reprenez où vous vous êtes arrêté, ou ouvrez n'importe quelle note avec
          <kbd>Ctrl</kbd> <kbd>K</kbd>.
        {/if}
      </p>
      <div class="cta">
        <button class="primary" onclick={() => void createNote()}>
          <Icon name="plus" size={15} stroke={2.2} />
          Nouvelle note
        </button>
        <button class="secondary" onclick={() => paletteOpen.set(true)}>
          <Icon name="search" size={15} />
          Rechercher
        </button>
        <button class="secondary" onclick={openGuide}>
          <Icon name="notes" size={15} />
          Guide
        </button>
      </div>
    </header>

    {#if $inboxCount > 0}
      <button class="inbox-banner" onclick={() => currentView.set({ kind: 'inbox' })}>
        <Icon name="inbox" size={16} />
        <span><strong>{$inboxCount} note{$inboxCount > 1 ? 's' : ''} à classer</strong> — NoteCore propose un dossier pour chacune.</span>
        <Icon name="chevron-right" size={15} />
      </button>
    {/if}

    {#if $notes.length > 0}
      <section class="review" aria-label="Révision">
        {#if hasCards}
          <div class="rcard">
            <div class="rmain">
              <p class="rk"><Icon name="layers" size={14} /> Révision du jour</p>
              {#if toSee > 0}
                <p class="rbig"><strong>{toSee}</strong> carte{toSee > 1 ? 's' : ''} à voir</p>
                <p class="rsub">
                  {$reviewStats?.due ?? 0} à réviser · {$reviewStats?.new ?? 0} nouvelle{($reviewStats?.new ?? 0) > 1 ? 's' : ''}
                </p>
                <button class="primary" onclick={() => void startReview()}>
                  <Icon name="play" size={14} />
                  Commencer
                </button>
              {:else}
                <p class="rbig ok">Tout est à jour</p>
                <p class="rsub">
                  {#if $reviewStats?.nextDue}Prochaine révision {formatUntil($reviewStats.nextDue)}.{:else}Aucune carte en attente.{/if}
                </p>
              {/if}
            </div>
            <div class="ract">
              <div class="streak" title="Jours consécutifs avec au moins une carte revue">
                <Icon name="flame" size={16} />
                <strong>{$reviewStats?.streak ?? 0}</strong>
                <span>jour{($reviewStats?.streak ?? 0) > 1 ? 's' : ''}</span>
              </div>
              <div class="bars" role="img" aria-label="Cartes revues sur les 14 derniers jours">
                {#each $reviewStats?.activity ?? [] as n, i (i)}
                  <span class="bar" class:today={i === 13} style:height="{Math.max(6, (n / maxActivity) * 100)}%" title={`${n} carte${n > 1 ? 's' : ''}`}></span>
                {/each}
              </div>
            </div>
          </div>

          {#if masteryRows.length > 0}
            <ul class="mastery" aria-label="Maîtrise par dossier et étiquette">
              {#each masteryRows as r (r.kind + r.id)}
                <li>
                  <span class="mlabel">
                    <Icon name={r.kind === 'folder' ? 'folder' : 'tag'} size={13} />
                    {r.label}
                  </span>
                  <span class="mtrack"><span class="mfill {tone(r.retention)}" style:width="{Math.round(r.retention * 100)}%"></span></span>
                  <span class="mval">{Math.round(r.retention * 100)} %</span>
                  <span class="mdue">{r.due > 0 ? `${r.due} à revoir` : `${r.learned}/${r.cards}`}</span>
                </li>
              {/each}
            </ul>
          {/if}
        {:else}
          <div class="rcard discover">
            <div class="rmain">
              <p class="rk"><Icon name="layers" size={14} /> Réviser ses notes</p>
              <p class="rbig small">Transformez ce que vous écrivez en cartes.</p>
              <p class="rsub">
                Dans n'importe quelle note, cachez un mot avec <code>{'{{c1::mot}}'}</code> ou écrivez
                <code>Question :: Réponse</code>. NoteCore planifie les révisions pour vous.
              </p>
            </div>
          </div>
        {/if}
      </section>

      <section aria-label="Aperçu de l'espace de travail">
        <div class="tiles">
          {#each tiles as t (t.label)}
            <div class="tile tint-{t.tint}">
              <Icon name={t.icon} size={18} />
              <strong>{nf.format(t.value)}</strong>
              <span>{t.label}</span>
            </div>
          {/each}
        </div>
      </section>

      {#if pinned.length > 0}
        <section>
          <h2>Épinglées</h2>
          <ul class="pins">
            {#each pinned as n (n.id)}
              <li>
                <button onclick={() => void selectNote(n.id)}>
                  <Icon name="star" size={13} filled />
                  <span class="t">{n.title || 'Sans titre'}</span>
                  <span class="w">{formatRelative(n.updatedAt)}</span>
                </button>
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      <section>
        <h2>Reprendre</h2>
        <div class="cards">
          {#each recent as n (n.id)}
            <button class="card" onclick={() => void selectNote(n.id)}>
              <span class="ct">{n.title || 'Sans titre'}</span>
              <span class="cx">{excerpt(n.content, 150) || 'Note vide'}</span>
              <span class="cm">
                <span>{formatRelative(n.updatedAt)}</span>
                {#each (n.tags ?? []).slice(0, 2) as tag (tag)}
                  <span class="tag tint-{tagTint(tag)}">{tag}</span>
                {/each}
              </span>
            </button>
          {/each}
        </div>
      </section>
    {/if}
  </div>
</div>

<style>
  .home {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    background: var(--app-bg);
  }
  .inner {
    max-width: 56rem;
    margin: 0 auto;
    padding: 3rem 2.2rem 3rem;
  }

  .date {
    margin: 0;
    display: block;
    color: var(--text-faint);
    font-size: 0.82rem;
    letter-spacing: 0.01em;
  }
  h1 {
    margin: 0.2rem 0 0.5rem;
    font-size: 2.4rem;
    font-weight: 750;
    letter-spacing: -0.035em;
    line-height: 1.1;
  }
  .lead {
    margin: 0 0 1.3rem;
    max-width: 34rem;
    color: var(--text-soft);
    font-size: 1rem;
    line-height: 1.55;
  }
  kbd {
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--border);
    border-bottom-width: 2px;
    border-radius: 6px;
    background: var(--surface);
    font-family: var(--font-ui);
    font-size: 0.72rem;
  }

  .cta {
    display: flex;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
  .cta button {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    height: 38px;
    padding: 0 1rem;
    border-radius: var(--radius);
    font: inherit;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition:
      background 0.12s var(--ease),
      border-color 0.12s var(--ease);
  }
  .primary {
    border: none;
    background: var(--accent);
    color: var(--on-accent);
    box-shadow: var(--shadow-sm);
  }
  .primary:hover {
    background: var(--accent-strong);
  }
  .secondary {
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
  }
  .secondary:hover {
    border-color: var(--text-faint);
  }

  .inbox-banner {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    width: 100%;
    margin-top: 1.6rem;
    padding: 0.7rem 1rem;
    border: 1px dashed var(--accent);
    border-radius: var(--radius-lg);
    background: var(--accent-soft);
    color: var(--accent-text);
    font: inherit;
    font-size: 0.9rem;
    text-align: left;
    cursor: pointer;
  }
  .inbox-banner span {
    flex: 1;
    color: var(--text);
  }
  .inbox-banner:hover {
    background: var(--surface);
  }
  section {
    margin-top: 2.4rem;
  }
  section.review {
    margin-top: 2rem;
  }

  .rcard {
    display: flex;
    justify-content: space-between;
    gap: 1.5rem;
    padding: 1.3rem 1.5rem;
    border: 1px solid var(--card-border);
    border-radius: var(--radius-lg);
    background: linear-gradient(135deg, var(--accent-soft), var(--card-bg) 60%);
  }
  .rk {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin: 0 0 0.5rem;
    color: var(--accent-text);
    font-size: 0.74rem;
    font-weight: 650;
    letter-spacing: 0.07em;
    text-transform: uppercase;
  }
  .rbig {
    margin: 0;
    font-size: 1.15rem;
    color: var(--text-soft);
  }
  .rbig strong {
    margin-right: 0.2rem;
    color: var(--text);
    font-size: 2.3rem;
    font-weight: 750;
    letter-spacing: -0.03em;
    font-variant-numeric: tabular-nums;
  }
  .rbig.ok {
    color: var(--text);
    font-size: 1.5rem;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .rbig.small {
    color: var(--text);
    font-size: 1.25rem;
    font-weight: 700;
    letter-spacing: -0.02em;
  }
  .rsub {
    margin: 0.35rem 0 0.9rem;
    max-width: 30rem;
    color: var(--text-soft);
    font-size: 0.88rem;
    line-height: 1.5;
  }
  .rsub code {
    padding: 0.05em 0.4em;
    border-radius: 5px;
    background: var(--code-bg);
    font-family: var(--font-mono);
    font-size: 0.88em;
  }
  .rcard .primary {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    height: 38px;
    padding: 0 1.1rem;
    border: none;
    border-radius: var(--radius);
    background: var(--accent);
    color: var(--on-accent);
    font: inherit;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    box-shadow: var(--shadow-sm);
  }
  .rcard .primary:hover {
    background: var(--accent-strong);
  }
  .rcard.discover .rsub {
    margin-bottom: 0;
  }
  .ract {
    display: grid;
    align-content: space-between;
    justify-items: end;
    gap: 0.8rem;
    min-width: 9rem;
  }
  .streak {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--warning);
    font-size: 0.85rem;
  }
  .streak strong {
    color: var(--text);
    font-size: 1.1rem;
  }
  .streak span {
    color: var(--text-soft);
  }
  .bars {
    display: flex;
    align-items: flex-end;
    gap: 3px;
    width: 9rem;
    height: 3rem;
  }
  .bar {
    flex: 1;
    min-height: 3px;
    border-radius: 2px;
    background: var(--accent);
    opacity: 0.35;
  }
  .bar.today {
    opacity: 1;
  }

  .mastery {
    display: grid;
    gap: 0.35rem;
    margin: 0.9rem 0 0;
    padding: 0;
    list-style: none;
  }
  .mastery li {
    display: grid;
    grid-template-columns: 10rem 1fr 3rem 5.5rem;
    align-items: center;
    gap: 0.8rem;
    padding: 0.3rem 0.2rem;
    font-size: 0.84rem;
  }
  .mlabel {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text);
  }
  .mtrack {
    height: 6px;
    border-radius: 999px;
    background: var(--chip-bg);
    overflow: hidden;
  }
  .mfill {
    display: block;
    height: 100%;
    border-radius: inherit;
  }
  .mfill.good {
    background: var(--success);
  }
  .mfill.mid {
    background: var(--warning);
  }
  .mfill.low {
    background: var(--danger);
  }
  .mval {
    color: var(--text-soft);
    font-variant-numeric: tabular-nums;
    text-align: right;
  }
  .mdue {
    color: var(--text-faint);
    font-size: 0.78rem;
    text-align: right;
  }
  h2 {
    margin: 0 0 0.8rem;
    color: var(--text-faint);
    font-size: 0.72rem;
    font-weight: 650;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(9.5rem, 1fr));
    gap: 0.8rem;
  }
  .tile {
    display: grid;
    gap: 0.1rem;
    padding: 1rem 1.1rem 0.95rem;
    border-radius: var(--radius-lg);
    background: var(--tint-bg);
    color: var(--tint-ink);
  }
  .tile strong {
    margin-top: 0.7rem;
    font-size: 1.9rem;
    font-weight: 600;
    letter-spacing: -0.03em;
    font-variant-numeric: tabular-nums;
    line-height: 1.1;
  }
  .tile span {
    font-size: 0.82rem;
    opacity: 0.8;
  }

  .pins {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 2px;
  }
  .pins button {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
    padding: 0.5rem 0.7rem;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.92rem;
    text-align: left;
    cursor: pointer;
  }
  .pins button:hover {
    background: var(--hover);
  }
  .pins :global(svg) {
    color: var(--warning);
  }
  .pins .t {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pins .w {
    color: var(--text-faint);
    font-size: 0.78rem;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: 0.8rem;
  }
  .card {
    display: grid;
    align-content: start;
    gap: 0.4rem;
    min-height: 8.5rem;
    padding: 1rem 1.1rem;
    border: 1px solid var(--card-border);
    border-radius: var(--radius-lg);
    background: var(--card-bg);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.12s var(--ease),
      box-shadow 0.12s var(--ease),
      transform 0.12s var(--ease);
  }
  .card:hover {
    border-color: var(--accent);
    box-shadow: var(--shadow-md);
    transform: translateY(-1px);
  }
  .ct {
    font-size: 1rem;
    font-weight: 650;
    letter-spacing: -0.01em;
    line-height: 1.3;
  }
  .cx {
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
    color: var(--text-soft);
    font-size: 0.84rem;
    line-height: 1.5;
  }
  .cm {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: auto;
    padding-top: 0.3rem;
    color: var(--text-faint);
    font-size: 0.74rem;
  }
  .tag {
    padding: 0 0.5rem;
    border-radius: 999px;
    background: var(--tint-bg);
    color: var(--tint-ink);
    line-height: 1.55;
  }
</style>
