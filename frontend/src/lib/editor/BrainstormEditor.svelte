<script lang="ts">
  /**
   * Mode Brainstorm : la note devient un outline (comme Workflowy).
   * Chaque ligne est une puce Markdown « - texte » indentée de deux
   * espaces par niveau — le format reste du Markdown standard, donc
   * recherche FTS, historique de versions, aperçu et export fonctionnent
   * sans rien savoir du mode.
   *
   * Entrée = puce suivante · Tab/Shift+Tab = indenter/désindenter ·
   * flèches = naviguer · case [ ]/[x] cliquable · chevron = replier.
   */
  import Icon from '../components/Icon.svelte';
  import { checkboxState, parseOutline, serializeOutline, taskText, type OutlineNode as Node } from './outline';

  let {
    content,
    onChange,
    spellcheck = true
  }: { content: string; onChange: (text: string) => void; spellcheck?: boolean } = $props();

  let nextId = 1;
  const nid = () => nextId++;

  // Le composant est remonté par note ({#key}) : le contenu initial est
  // capturé une fois, les frappes suivantes passent par onChange.
  function initialNodes(): Node[] {
    return parseOutline(content, nid);
  }
  let nodes = $state(initialNodes());
  /** Ids des nœuds dont les enfants sont repliés. */
  let collapsed = $state<Set<number>>(new Set());
  /** Nœud racine du zoom (null = plan entier). */
  let zoomId = $state<number | null>(null);
  let inputs = $state<HTMLInputElement[]>([]);

  function emit(): void {
    onChange(serializeOutline(nodes));
  }

  function hasChildren(index: number): boolean {
    return index + 1 < nodes.length && nodes[index + 1].depth > nodes[index].depth;
  }

  /** Index du nœud zoomé et borne de sa sous-arborescence. */
  const zoomRange = $derived.by(() => {
    if (zoomId === null) return null;
    const start = nodes.findIndex((n) => n.id === zoomId);
    if (start < 0) return null;
    const depth = nodes[start].depth;
    let end = start + 1;
    while (end < nodes.length && nodes[end].depth > depth) end++;
    return { start, end, depth };
  });

  /** Chemin d'ancêtres du nœud zoomé, pour le fil d'Ariane. */
  const zoomTrail = $derived.by(() => {
    if (!zoomRange) return [];
    const trail: Node[] = [];
    let depth = zoomRange.depth;
    for (let i = zoomRange.start - 1; i >= 0 && trail.length < depth; i--) {
      if (nodes[i].depth < depth) {
        trail.unshift(nodes[i]);
        depth = nodes[i].depth;
      }
    }
    return trail;
  });

  /** Index des nœuds visibles (zoom + branches repliées masqués). */
  function visible(): number[] {
    const out: number[] = [];
    const range = zoomRange;
    const from = range ? range.start : 0;
    const to = range ? range.end : nodes.length;
    let hiddenDepth = -1;
    for (let i = from; i < to; i++) {
      if (hiddenDepth >= 0) {
        if (nodes[i].depth > hiddenDepth) continue;
        hiddenDepth = -1;
      }
      out.push(i);
      if (collapsed.has(nodes[i].id) && hasChildren(i)) hiddenDepth = nodes[i].depth;
    }
    return out;
  }

  const vis = $derived(visible());

  function focusNode(index: number, caret: 'start' | 'end' = 'end'): void {
    const pos = vis.indexOf(index);
    if (pos < 0) return;
    const el = inputs[pos];
    el?.focus();
    if (el) {
      const at = caret === 'start' ? 0 : el.value.length;
      el.setSelectionRange(at, at);
    }
  }

  function insertAfter(index: number, depth?: number): void {
    const node: Node = { id: nextId++, depth: depth ?? nodes[index].depth, text: '' };
    nodes.splice(index + 1, 0, node);
    emit();
    focusNode(index + 1);
  }

  function removeNode(index: number): void {
    nodes.splice(index, 1);
    if (nodes.length === 0) nodes.push({ id: nextId++, depth: 0, text: '' });
    emit();
    focusNode(Math.max(0, index - 1));
  }

  function indent(index: number): void {
    const max = index === 0 ? 0 : nodes[index - 1].depth + 1;
    if (nodes[index].depth < max) {
      nodes[index].depth++;
      emit();
    }
  }

  function outdent(index: number): void {
    if (nodes[index].depth > 0) {
      nodes[index].depth--;
      emit();
    }
  }

  function toggleCollapse(index: number): void {
    const id = nodes[index].id;
    const next = new Set(collapsed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    collapsed = next;
  }

  /** Zoom : isole la sous-arborescence du nœud (focus Workflowy). */
  function zoomIn(index: number): void {
    if (hasChildren(index)) {
      zoomId = nodes[index].id;
      focusNode(index);
    }
  }

  /** Dézoome d'un niveau vers l'ancêtre, ou du plan entier. */
  function zoomOut(): void {
    if (!zoomRange) return;
    const parent = zoomTrail[zoomTrail.length - 1];
    if (parent) {
      zoomId = parent.id;
      focusNode(nodes.indexOf(parent));
    } else {
      zoomId = null;
    }
  }

  /** Déplace le nœud et sa sous-arborescence avant/après son voisin. */
  function moveNode(index: number, dir: -1 | 1): void {
    const depth = nodes[index].depth;
    // Étendue du bloc : le nœud + ses descendants.
    let end = index + 1;
    while (end < nodes.length && nodes[end].depth > depth) end++;
    const block = nodes.splice(index, end - index);

    if (dir === -1) {
      // Précédent frère : dernier nœud de même profondeur avant le bloc.
      let target = index - 1;
      while (target >= 0 && nodes[target].depth > depth) target--;
      if (target < 0 || nodes[target].depth !== depth) {
        nodes.splice(index, 0, ...block);
        return;
      }
      // Le bloc s'insère juste avant le frère précédent — sa
      // sous-arborescence (déjà passée) reste derrière lui.
      nodes.splice(target, 0, ...block);
      focusNode(target);
    } else {
      // Suivant frère : premier nœud de même profondeur après le bloc.
      let target = index; // index décalé après splice : le bloc est retiré
      while (target < nodes.length && nodes[target].depth > depth) target++;
      if (target >= nodes.length || nodes[target].depth !== depth) {
        nodes.splice(index, 0, ...block);
        return;
      }
      let insertAt = target + 1;
      while (insertAt < nodes.length && nodes[insertAt].depth > depth) insertAt++;
      nodes.splice(insertAt, 0, ...block);
      focusNode(insertAt - block.length);
    }
    emit();
  }

  function toggleTask(index: number): void {
    const n = nodes[index];
    const state = checkboxState(n.text);
    if (state === 'todo') n.text = `[x] ${taskText(n.text)}`;
    else if (state === 'done') n.text = `[ ] ${taskText(n.text)}`;
    else n.text = `[ ] ${n.text}`;
    emit();
  }

  function onKeydown(e: KeyboardEvent, index: number): void {
    const el = e.currentTarget as HTMLInputElement;
    const caretStart = el.selectionStart ?? 0;
    const visIdx = vis.indexOf(index);

    switch (e.key) {
      case 'Escape':
        // Échap dans l'outline = dézoomer d'un niveau ; on ne le laisse
        // pas remonter à la fenêtre (qui fermerait le volet split).
        if (zoomRange) {
          e.preventDefault();
          e.stopPropagation();
          zoomOut();
        }
        break;
      case 'Enter':
        e.preventDefault();
        insertAfter(index);
        break;
      case 'Tab':
        e.preventDefault();
        if (e.shiftKey) outdent(index);
        else indent(index);
        break;
      case 'ArrowUp':
        e.preventDefault();
        if (e.altKey) moveNode(index, -1);
        else if (visIdx > 0) focusNode(vis[visIdx - 1], 'end');
        break;
      case 'ArrowDown':
        e.preventDefault();
        if (e.altKey) moveNode(index, 1);
        else if (visIdx >= 0 && visIdx < vis.length - 1) focusNode(vis[visIdx + 1], 'start');
        break;
      case 'ArrowRight':
        if ((e.ctrlKey || e.metaKey) && e.shiftKey) {
          e.preventDefault();
          zoomIn(index);
        }
        break;
      case 'ArrowLeft':
        if ((e.ctrlKey || e.metaKey) && e.shiftKey && zoomRange) {
          e.preventDefault();
          e.stopPropagation();
          zoomOut();
        }
        break;
      case 'Backspace':
        if (caretStart === 0 && nodes[index].text === '') {
          e.preventDefault();
          removeNode(index);
        } else if (caretStart === 0 && nodes[index].text !== '') {
          // Fusion avec la puce précédente : la fin du texte rejoint la fin
          // du nœud du dessus (comportement outline classique).
          e.preventDefault();
          const prev = vis[visIdx - 1];
          if (prev !== undefined) {
            // On fusionne les libellés visibles : le marqueur de case
            // éventuel du nœud absorbé ne doit pas rester en clair.
            nodes[prev].text = `${nodes[prev].text} ${taskText(nodes[index].text)}`.trim();
            nodes.splice(index, 1);
            emit();
            focusNode(prev, 'end');
          }
        }
        break;
      case 'Delete': {
        const el = e.currentTarget as HTMLInputElement;
        const atEnd = el.selectionStart === el.value.length && el.selectionStart === el.selectionEnd;
        if (nodes[index].text === '') {
          e.preventDefault();
          removeNode(index);
        } else if (atEnd && visIdx >= 0 && visIdx < vis.length - 1) {
          // Fin de puce : absorber la puce suivante dans celle-ci.
          e.preventDefault();
          const next = vis[visIdx + 1];
          nodes[index].text = `${nodes[index].text} ${taskText(nodes[next].text)}`.trim();
          nodes.splice(next, 1);
          emit();
          focusNode(index, 'end');
        }
        break;
      }
    }
  }
</script>

<div class="outline" role="tree" aria-label="Plan de brainstorm">
  {#if zoomRange}
    <nav class="crumbs" aria-label="Position dans le plan">
      <button class="crumb" onclick={() => { zoomId = null; }}>Plan</button>
      {#each zoomTrail as ancestor (ancestor.id)}
        <Icon name="chevron-right" size={11} />
        <button class="crumb" onclick={() => { zoomId = ancestor.id; }}>{ancestor.text || '…'}</button>
      {/each}
      <Icon name="chevron-right" size={11} />
      <span class="crumb here">{nodes[zoomRange.start].text || '…'}</span>
    </nav>
  {/if}
  {#each vis as index (nodes[index].id)}
    {@const n = nodes[index]}
    {@const children = hasChildren(index)}
    {@const state = checkboxState(n.text)}
    <div
      class="row"
      class:branch={children}
      style:margin-left="{n.depth * 1.15}rem"
      role="treeitem"
      aria-selected="false"
      aria-expanded={children ? !collapsed.has(n.id) : undefined}
    >
      {#if children}
        <button
          class="twisty"
          title={collapsed.has(n.id) ? 'Déplier' : 'Replier'}
          aria-label={collapsed.has(n.id) ? 'Déplier la branche' : 'Replier la branche'}
          onclick={() => toggleCollapse(index)}
        >
          <Icon name={collapsed.has(n.id) ? 'chevron-right' : 'chevron-down'} size={12} stroke={2.4} />
        </button>
      {:else}
        <span class="twisty-space"></span>
      {/if}
      {#if state !== 'none'}
        <button
          class="task"
          class:done={state === 'done'}
          role="checkbox"
          aria-checked={state === 'done'}
          aria-label={state === 'done' ? 'Marquer à faire' : 'Marquer fait'}
          onclick={() => toggleTask(index)}
        ></button>
      {:else}
        <span class="bullet" aria-hidden="true"></span>
      {/if}
      <input
        bind:this={inputs[vis.indexOf(index)]}
        class="item"
        class:task-text={state !== 'none'}
        class:done={state === 'done'}
        value={taskText(n.text)}
        placeholder={index === 0 && n.text === '' ? 'Idée principale…' : ''}
        spellcheck={spellcheck}
        autocomplete="off"
        aria-label={`Élément ${index + 1}`}
        oninput={(e) => {
          const v = (e.target as HTMLInputElement).value;
          if (state === 'todo') n.text = `[ ] ${v}`;
          else if (state === 'done') n.text = `[x] ${v}`;
          else n.text = v;
          emit();
        }}
        onkeydown={(e) => onKeydown(e, index)}
      />
    </div>
  {/each}
  <p class="hint">
    Entrée : nouvelle puce · Tab : indenter · Maj+Tab : désindenter · Alt+↑/↓ : déplacer ·
    Ctrl+Maj+→ : zoom sur la branche · Échap : dézoomer · flèches : naviguer
  </p>
</div>

<style>
  .outline {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 1.4rem 1.6rem 2rem;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    min-height: 30px;
  }
  .row.branch .item {
    font-weight: 600;
  }
  .twisty,
  .twisty-space {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    display: grid;
    place-items: center;
  }
  .twisty {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text-faint);
    cursor: pointer;
    border-radius: 4px;
    padding: 0;
  }
  .twisty:hover {
    background: var(--hover);
    color: var(--text);
  }
  .bullet {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-faint);
    flex-shrink: 0;
    margin: 0 6px;
  }
  .task {
    width: 15px;
    height: 15px;
    flex-shrink: 0;
    appearance: none;
    border: 1.5px solid var(--border-strong, var(--border));
    border-radius: 4px;
    background: var(--surface);
    cursor: pointer;
    position: relative;
  }
  .task.done {
    background: var(--success);
    border-color: var(--success);
  }
  .task.done::after {
    content: '';
    position: absolute;
    left: 4px;
    top: 1px;
    width: 4px;
    height: 8px;
    border: solid #fff;
    border-width: 0 2px 2px 0;
    transform: rotate(45deg);
  }
  .item {
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 0.95rem;
    line-height: 1.5;
    padding: 0.15rem 0;
  }
  .item:focus {
    outline: none;
  }
  .item::placeholder {
    color: var(--text-faint);
  }
  .task-text.done {
    color: var(--text-faint);
    text-decoration: line-through;
  }
  .hint {
    margin-top: 1.6rem;
    font-size: 0.76rem;
    color: var(--text-faint);
    text-align: center;
  }
  .crumbs {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    flex-wrap: wrap;
    margin-bottom: 0.9rem;
    padding-bottom: 0.6rem;
    border-bottom: 1px solid var(--border);
    font-size: 0.8rem;
  }
  .crumbs :global(svg) {
    color: var(--text-faint);
    flex-shrink: 0;
  }
  .crumb {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--accent-text);
    font: inherit;
    cursor: pointer;
    padding: 0.1rem 0.3rem;
    border-radius: var(--radius-sm);
    max-width: 14rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .crumb:hover {
    background: var(--hover);
  }
  .crumb.here {
    color: var(--text);
    font-weight: 600;
    cursor: default;
  }
  .crumb.here:hover {
    background: transparent;
  }
</style>
