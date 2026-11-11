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

  let {
    content,
    onChange,
    spellcheck = true
  }: { content: string; onChange: (text: string) => void; spellcheck?: boolean } = $props();

  interface Node {
    id: number;
    depth: number;
    text: string;
  }

  let nextId = 1;

  function parse(text: string): Node[] {
    const out: Node[] = [];
    for (const line of text.split('\n')) {
      if (!line.trim()) continue;
      const m = /^(\s*)(?:[-*+]\s+)?(.*)$/.exec(line);
      if (!m) continue;
      const indent = m[1].replace(/\t/g, '  ').length;
      out.push({ id: nextId++, depth: Math.floor(indent / 2), text: m[2].trimEnd() });
    }
    if (out.length === 0) out.push({ id: nextId++, depth: 0, text: '' });
    return out;
  }

  function serialize(nodes: Node[]): string {
    return nodes.map((n) => `${'  '.repeat(n.depth)}- ${n.text}`).join('\n');
  }

  // Le composant est remonté par note ({#key}) : le contenu initial est
  // capturé une fois, les frappes suivantes passent par onChange.
  function initialNodes(): Node[] {
    return parse(content);
  }
  let nodes = $state(initialNodes());
  /** Ids des nœuds dont les enfants sont repliés. */
  let collapsed = $state<Set<number>>(new Set());
  let inputs = $state<HTMLInputElement[]>([]);

  function emit(): void {
    onChange(serialize(nodes));
  }

  function hasChildren(index: number): boolean {
    return index + 1 < nodes.length && nodes[index + 1].depth > nodes[index].depth;
  }

  /** Index des nœuds visibles (enfants des branches repliées masqués). */
  function visible(): number[] {
    const out: number[] = [];
    let hiddenDepth = -1;
    for (let i = 0; i < nodes.length; i++) {
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

  /** Case à cocher en tête de puce : « [ ] » / « [x] ». */
  function checkboxState(text: string): 'none' | 'todo' | 'done' {
    if (/^\[ \]\s*/.test(text)) return 'todo';
    if (/^\[[xX]\]\s*/.test(text)) return 'done';
    return 'none';
  }

  function toggleTask(index: number): void {
    const n = nodes[index];
    const state = checkboxState(n.text);
    if (state === 'todo') n.text = n.text.replace(/^\[ \]\s*/, '[x] ');
    else if (state === 'done') n.text = n.text.replace(/^\[[xX]\]\s*/, '[ ] ');
    else n.text = `[ ] ${n.text}`;
    emit();
  }

  function onKeydown(e: KeyboardEvent, index: number): void {
    const el = e.currentTarget as HTMLInputElement;
    const caretStart = el.selectionStart ?? 0;
    const visIdx = vis.indexOf(index);

    switch (e.key) {
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
        if (visIdx > 0) focusNode(vis[visIdx - 1], 'end');
        break;
      case 'ArrowDown':
        e.preventDefault();
        if (visIdx >= 0 && visIdx < vis.length - 1) focusNode(vis[visIdx + 1], 'start');
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
            nodes[prev].text = `${nodes[prev].text} ${nodes[index].text}`.trim();
            nodes.splice(index, 1);
            emit();
            focusNode(prev, 'end');
          }
        }
        break;
      case 'Delete':
        if (nodes[index].text === '') {
          e.preventDefault();
          removeNode(index);
        }
        break;
    }
  }
</script>

<div class="outline" role="tree" aria-label="Plan de brainstorm">
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
        value={n.text.replace(state === 'todo' ? /^\[ \]\s*/ : state === 'done' ? /^\[[xX]\]\s*/ : /^/, '')}
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
    Entrée : nouvelle puce · Tab : indenter · Maj+Tab : désindenter · flèches : naviguer · Backspace sur une puce vide : supprimer
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
</style>
