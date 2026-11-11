<script lang="ts">
  /**
   * Vue mind map : rend le même outline Markdown (listes indentées)
   * en arbre SVG. Lecture seule — l'édition reste dans le mode
   * Brainstorm ou l'éditeur classique.
   */
  import { buildTree, checkboxState, parseOutline, taskText, type OutlineNode } from './outline';
  import Icon from '../components/Icon.svelte';

  let { content }: { content: string } = $props();

  interface Placed {
    node: OutlineNode;
    x: number;
    y: number;
    w: number;
    h: number;
    children: Placed[];
    state: 'none' | 'todo' | 'done';
  }

  const GAP_X = 56;
  const GAP_Y = 10;
  const NODE_H = 30;
  const CHAR_W = 7.4;
  const PAD_X = 16;

  /** Largeur estimée du libellé (pas de mesure DOM : stable et rapide). */
  function width(text: string): number {
    return Math.min(260, Math.max(84, text.length * CHAR_W + PAD_X * 2));
  }

  function place(tree: ReturnType<typeof buildTree>, depth: number, y0: { v: number }): Placed[] {
    return tree.map((t) => {
      const children = place(t.children, depth + 1, y0);
      const leafH = t.children.length === 0 ? NODE_H : Math.max(NODE_H, childrenSpan(children));
      const y = y0.v + leafH / 2 - NODE_H / 2;
      if (children.length === 0) y0.v += NODE_H + GAP_Y;
      else y0.v += leafH;
      return {
        node: t.node,
        x: depth * GAP_X,
        y,
        w: width(taskText(t.node.text)),
        h: NODE_H,
        children,
        state: checkboxState(t.node.text)
      };
    });
  }

  function childrenSpan(children: Placed[]): number {
    if (children.length === 0) return 0;
    const first = children[0];
    const last = children[children.length - 1];
    return last.y + last.h - first.y + GAP_Y;
  }

  let nextId = 1;
  const nid = () => nextId++;

  /** Arêtes parent → enfant (cubiques). */
  interface Edge { x1: number; y1: number; x2: number; y2: number }

  const roots = $derived.by(() => {
    const flat = parseOutline(content, nid);
    const y0 = { v: 0 };
    const placed = place(buildTree(flat), 0, y0);
    return { list: placed, height: Math.max(y0.v, NODE_H) };
  });

  /** Liste plate des nœuds + arêtes, dans l'ordre de rendu. */
  const flat = $derived.by(() => {
    const nodes: Placed[] = [];
    const edges: Edge[] = [];
    const walk = (list: Placed[]) => {
      for (const p of list) {
        nodes.push(p);
        for (const c of p.children) {
          edges.push({ x1: p.x + p.w, y1: p.y + p.h / 2, x2: c.x, y2: c.y + c.h / 2 });
          walk(c.children);
        }
      }
    };
    walk(roots.list);
    return { nodes, edges };
  });

  const bounds = $derived.by(() => {
    let maxX = 0;
    let maxY = roots.height;
    for (const p of flat.nodes) {
      maxX = Math.max(maxX, p.x + p.w);
      maxY = Math.max(maxY, p.y + p.h);
    }
    return { w: maxX + 40, h: maxY + 40 };
  });

  // Pan + zoom : viewBox piloté à la souris, boutons pour zoomer.
  let view = $state({ x: 0, y: 0, w: 800, h: 600 });
  let dragging = false;
  let last = { x: 0, y: 0 };
  let svgEl = $state<SVGSVGElement | null>(null);

  function fit(): void {
    if (bounds.w <= 0 || bounds.h <= 0) return;
    const pad = 30;
    view = { x: -pad, y: -pad, w: bounds.w + pad * 2, h: bounds.h + pad * 2 };
  }

  $effect(() => {
    // Ajuste la vue au premier rendu et quand le contenu change de taille.
    fit();
  });

  function onWheel(e: WheelEvent): void {
    e.preventDefault();
    const rect = svgEl?.getBoundingClientRect();
    if (!rect) return;
    const mx = view.x + ((e.clientX - rect.left) / rect.width) * view.w;
    const my = view.y + ((e.clientY - rect.top) / rect.height) * view.h;
    const factor = e.deltaY > 0 ? 1.12 : 0.89;
    const w = Math.min(6000, Math.max(120, view.w * factor));
    const h = w * (rect.height / rect.width);
    view = { x: mx - ((mx - view.x) / view.w) * w, y: my - ((my - view.y) / (view.h || 1)) * h, w, h };
  }

  function onPointerDown(e: PointerEvent): void {
    dragging = true;
    last = { x: e.clientX, y: e.clientY };
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
  }
  function onPointerMove(e: PointerEvent): void {
    if (!dragging || !svgEl) return;
    const rect = svgEl.getBoundingClientRect();
    view = {
      ...view,
      x: view.x - ((e.clientX - last.x) / rect.width) * view.w,
      y: view.y - ((e.clientY - last.y) / rect.height) * view.h
    };
    last = { x: e.clientX, y: e.clientY };
  }
  function onPointerUp(): void { dragging = false; }

  function zoomBy(factor: number): void {
    const w = Math.min(6000, Math.max(120, view.w * factor));
    const cx = view.x + view.w / 2;
    const cy = view.y + view.h / 2;
    view = { x: cx - w / 2, y: cy - (w * view.h) / view.w / 2, w, h: (w * view.h) / view.w };
  }

  function label(p: Placed): string {
    const t = taskText(p.node.text);
    if (p.state === 'done') return `☑ ${t}`;
    if (p.state === 'todo') return `☐ ${t}`;
    return t;
  }
</script>

<div class="mindmap">
  {#if roots.list.length === 0 || (roots.list.length === 1 && !roots.list[0].node.text)}
    <div class="empty">
      <Icon name="sparkles" size={30} />
      <p>La carte se dessine à partir des puces de la note.</p>
      <p class="sub">Passe en mode Brainstorm pour structurer tes idées.</p>
    </div>
  {:else}
    <svg
      bind:this={svgEl}
      class="canvas"
      viewBox="{view.x} {view.y} {view.w} {view.h}"
      role="img"
      aria-label="Carte mentale de la note"
      onwheel={onWheel}
      onpointerdown={onPointerDown}
      onpointermove={onPointerMove}
      onpointerup={onPointerUp}
      onpointercancel={onPointerUp}
    >
      {#each flat.edges as edge, i (i)}
        <path
          class="edge"
          d="M {edge.x1} {edge.y1} C {edge.x1 + GAP_X * 0.55} {edge.y1}, {edge.x2 - GAP_X * 0.55} {edge.y2}, {edge.x2} {edge.y2}"
        />
      {/each}
      {#each flat.nodes as p (p.node.id)}
        <g class="node" class:done={p.state === 'done'} class:todo={p.state === 'todo'}>
          <rect x={p.x} y={p.y} width={p.w} height={p.h} rx="8" />
          <text x={p.x + PAD_X} y={p.y + p.h / 2 + 4}>{label(p)}</text>
        </g>
      {/each}
    </svg>
    <div class="tools">
      <button onclick={() => zoomBy(0.8)} aria-label="Zoom arrière" title="Zoom arrière"><Icon name="zoom-out" size={14} /></button>
      <button onclick={fit} aria-label="Ajuster" title="Ajuster la carte"><Icon name="focus" size={14} /></button>
      <button onclick={() => zoomBy(1.25)} aria-label="Zoom avant" title="Zoom avant"><Icon name="zoom-in" size={14} /></button>
    </div>
    <p class="hint">Molette : zoom · glisser : déplacer</p>
  {/if}
</div>

<style>
  .mindmap {
    position: relative;
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .canvas {
    flex: 1;
    width: 100%;
    height: 100%;
    cursor: grab;
    touch-action: none;
    background:
      radial-gradient(circle, var(--border) 1px, transparent 1px);
    background-size: 24px 24px;
  }
  .canvas:active { cursor: grabbing; }
  .edge {
    fill: none;
    stroke: var(--border-strong, var(--border));
    stroke-width: 1.5;
  }
  .node rect {
    fill: var(--surface);
    stroke: var(--border);
    stroke-width: 1;
  }
  .node text {
    font-size: 12px;
    fill: var(--text);
    font-family: inherit;
  }
  .node.done rect { stroke: var(--accent); }
  .node.done text { fill: var(--text-faint); text-decoration: line-through; }
  .node.todo rect { stroke-dasharray: 4 3; }
  .tools {
    position: absolute;
    right: 1rem;
    bottom: 2.2rem;
    display: flex;
    gap: 0.3rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.25rem;
    box-shadow: var(--shadow, 0 2px 8px rgb(0 0 0 / 0.12));
  }
  .tools button {
    appearance: none;
    border: none;
    background: transparent;
    color: var(--text);
    width: 28px;
    height: 28px;
    border-radius: var(--radius-sm);
    cursor: pointer;
    display: grid;
    place-items: center;
  }
  .tools button:hover { background: var(--hover); }
  .hint {
    position: absolute;
    left: 50%;
    bottom: 0.6rem;
    transform: translateX(-50%);
    margin: 0;
    font-size: 0.72rem;
    color: var(--text-faint);
    pointer-events: none;
  }
  .empty {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    color: var(--text-faint);
    text-align: center;
    padding: 2rem;
  }
  .empty .sub { font-size: 0.8rem; opacity: 0.8; }
</style>
