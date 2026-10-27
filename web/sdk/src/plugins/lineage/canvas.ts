/**
 * The Lineage canvas: cards as elements, edges as SVG under them, in a world that pans (drag the
 * background) and zooms (the wheel, or the controls). A click pins a card's path and dims the
 * rest. Cards are kept by id across draws, so a redraw moves nothing unless the layout changes:
 * a new shape is laid out and fitted to the view, and a card that changes size is laid out again
 * in place.
 */
import { h, icon, replace, type Children } from '../inspect/view';
import {
  crossFilterLoops, curve, layout, layoutKey, shapeOf, type Box,
} from './layout';
import { pathOf, type LineageGraph, type LineageGraphEdge, type LineageGraphNode } from './model';

const SVG = 'http://www.w3.org/2000/svg';
const FIT_PADDING = 0.12;
const MAX_FIT_ZOOM = 1;
const MIN_ZOOM = 0.1;
const MAX_ZOOM = 2;
const CENTER_ZOOM = 0.8;

/** How a card stands against a pinned path: on it, the one pinned, or off it. */
export type Tone = 'normal' | 'selected' | 'on-path' | 'dim';

export interface CanvasOptions {
  /** The smallest zoom a fit goes to: cards stay readable, and the large view shows the rest. */
  minFitZoom: number;
  /** A card's content. */
  card: (node: LineageGraphNode) => Children;
  /** A card clicked (its id), or the background (undefined). */
  onSelect: (id: string | undefined) => void;
}

export interface Canvas {
  readonly element: HTMLElement;
  /** Draw `graph`, with `selected`'s path pinned. */
  draw: (graph: LineageGraph, selected: string | undefined) => void;
  /** Bring a card to the middle, without zooming out from where the reader is. */
  center: (id: string) => void;
  dispose: () => void;
}

let canvases = 0;

/** Each edge's look: what it means, and whether a filter is in force. */
function edgeClass (e: LineageGraphEdge): string {
  if (!e.kind) return 'uses';
  return e.active ? 'filter-on' : 'filter-off';
}

export function createCanvas (doc: Document, options: CanvasOptions): Canvas {
  const prefix = `lineage-${++canvases}`;
  const svg = doc.createElementNS(SVG, 'svg');
  svg.setAttribute('class', 'edges');
  svg.setAttribute('width', '1');
  svg.setAttribute('height', '1');
  svg.setAttribute('aria-hidden', 'true');
  const cardsLayer = h(doc, 'div', { class: 'cards' });
  const world = h(doc, 'div', { class: 'world' });
  world.append(svg, cardsLayer);
  const view = { x: 0, y: 0, k: 1 };
  const controls = h(doc, 'div', { class: 'controls' },
    h(doc, 'button', { type: 'button', class: 'row-action', 'data-tip': 'Zoom in', 'aria-label': 'Zoom in', onclick: () => zoomBy(1.25) }, icon(doc, 'plus')),
    h(doc, 'button', { type: 'button', class: 'row-action', 'data-tip': 'Zoom out', 'aria-label': 'Zoom out', onclick: () => zoomBy(0.8) }, icon(doc, 'minus')),
    h(doc, 'button', { type: 'button', class: 'row-action', 'data-tip': 'Fit to view', 'aria-label': 'Fit to view', 'data-testid': 'lineage-fit', onclick: () => fit(true) }, icon(doc, 'fit')));
  const element = h(doc, 'div', { class: 'canvas', 'data-testid': 'lineage-canvas' }, world, controls);

  const cards = new Map<string, HTMLElement>();
  let graph: LineageGraph = { nodes: [], edges: [] };
  let selected: string | undefined;
  let boxes = new Map<string, Box>();
  let laidOutKey: string | undefined;
  let fittedShape: string | undefined;
  /** Whether a fit is owed, for when the canvas first has a size. */
  let owesFit = false;
  let hasFitted = false;
  let owesCenter: string | undefined;

  const apply = (duration = 0) => {
    world.style.transition = duration ? `transform ${duration}ms ease` : '';
    world.style.transform = `translate(${view.x}px, ${view.y}px) scale(${view.k})`;
    element.style.backgroundSize = `${18 * view.k}px ${18 * view.k}px`;
    element.style.backgroundPosition = `${view.x}px ${view.y}px`;
  };
  const clamp = (k: number) => Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k));
  const size = () => ({ width: element.clientWidth, height: element.clientHeight });
  const zoomAt = (px: number, py: number, k: number, duration = 0) => {
    const next = clamp(k);
    view.x = px - ((px - view.x) / view.k) * next;
    view.y = py - ((py - view.y) / view.k) * next;
    view.k = next;
    apply(duration);
  };
  const zoomBy = (factor: number) => {
    const { width, height } = size();
    zoomAt(width / 2, height / 2, view.k * factor, 150);
  };

  function fit (animate: boolean) {
    const { width, height } = size();
    if (!width || !height || boxes.size === 0) {
      owesFit = true;
      return;
    }
    owesFit = false;
    const all = [...boxes.values()];
    const [minX, minY] = [Math.min(...all.map((b) => b.x)), Math.min(...all.map((b) => b.y))];
    const [maxX, maxY] = [Math.max(...all.map((b) => b.x + b.width)), Math.max(...all.map((b) => b.y + b.height))];
    const [w, hgt] = [maxX - minX, maxY - minY];
    const k = Math.min(MAX_FIT_ZOOM, Math.max(options.minFitZoom, Math.min(width / (w * (1 + 2 * FIT_PADDING)), height / (hgt * (1 + 2 * FIT_PADDING)))));
    view.k = clamp(k);
    view.x = (width - w * view.k) / 2 - minX * view.k;
    view.y = Math.max((height - hgt * view.k) / 2, height * FIT_PADDING / 2) - minY * view.k;
    apply(animate ? 200 : 0);
    hasFitted = true;
  }

  function center (id: string) {
    const box = boxes.get(id);
    const { width, height } = size();
    if (!box || !width || !height) {
      owesCenter = id;
      return;
    }
    owesCenter = undefined;
    const k = clamp(Math.max(view.k, CENTER_ZOOM));
    view.k = k;
    view.x = width / 2 - (box.x + box.width / 2) * k;
    view.y = height / 2 - (box.y + box.height / 2) * k;
    apply(250);
  }

  const measure = (id: string) => {
    const card = cards.get(id);
    return card ? { width: card.offsetWidth, height: card.offsetHeight } : undefined;
  };

  function drawEdges (path: ReturnType<typeof pathOf> | undefined) {
    const loops = crossFilterLoops(graph, boxes);
    const defs = doc.createElementNS(SVG, 'defs');
    for (const name of ['uses', 'filter-on', 'filter-off']) {
      for (const on of [false, true]) {
        const marker = doc.createElementNS(SVG, 'marker');
        const attributes = {
          id: `${prefix}-${name}${on ? '-on' : ''}`, class: `m ${name}${on ? ' on' : ''}`, viewBox: '0 0 10 10', refX: '10', refY: '5',
          markerWidth: '14', markerHeight: '14', markerUnits: 'userSpaceOnUse', orient: 'auto-start-reverse',
        };
        for (const [key, value] of Object.entries(attributes)) marker.setAttribute(key, value);
        const arrow = doc.createElementNS(SVG, 'path');
        arrow.setAttribute('d', 'M0,1 L10,5 L0,9 z');
        marker.append(arrow);
        defs.append(marker);
      }
    }
    const lines = graph.edges.flatMap((e) => {
      const [from, to] = [boxes.get(e.source), boxes.get(e.target)];
      if (!from || !to) return [];
      const d = e.kind === 'crossFilter' ? loops.get(e.id) : curve(from, to).path;
      if (!d) return [];
      const on = path?.edges.has(e.id) ?? false;
      const look = edgeClass(e);
      const line = doc.createElementNS(SVG, 'path');
      line.setAttribute('d', d);
      line.setAttribute('class', `edge ${look}${on ? ' on' : path ? ' dim' : ''}`);
      line.setAttribute('marker-end', `url(#${prefix}-${look}${on ? '-on' : ''})`);
      line.setAttribute('data-edge', e.id);
      return [{ line, on }];
    });
    // The path's edges over the rest.
    lines.sort((a, b) => Number(a.on) - Number(b.on));
    svg.replaceChildren(defs, ...lines.map((l) => l.line));
  }

  function place () {
    const key = layoutKey(graph, measure);
    if (key !== laidOutKey) {
      laidOutKey = key;
      boxes = layout(graph, measure);
      for (const [id, box] of boxes) {
        const card = cards.get(id);
        if (card) card.style.transform = `translate(${box.x}px, ${box.y}px)`;
      }
    }
    const shape = shapeOf(graph);
    if (shape !== fittedShape) {
      fittedShape = shape;
      fit(hasFitted);
    }
  }

  function draw (next: LineageGraph, pinned: string | undefined) {
    graph = next;
    selected = pinned && next.nodes.some((n) => n.id === pinned) ? pinned : undefined;
    const path = selected ? pathOf(graph, selected) : undefined;
    const seen = new Set<string>();
    for (const node of graph.nodes) {
      seen.add(node.id);
      let card = cards.get(node.id);
      if (!card) {
        card = h(doc, 'div', { class: 'lcard', 'data-id': node.id, 'data-testid': `card-${node.id}` });
        const id = node.id;
        card.addEventListener('click', (event) => {
          if ((event.target as Element).closest?.('button')) return;
          options.onSelect(selected === id ? undefined : id);
        });
        cards.set(node.id, card);
        resizes?.observe(card);
      }
      const tone: Tone = !path ? 'normal' : node.id === selected ? 'selected' : path.nodes.has(node.id) ? 'on-path' : 'dim';
      card.className = `lcard kind-${node.kind} ${tone}`;
      replace(doc, card, options.card(node));
      cardsLayer.append(card);
    }
    for (const [id, card] of cards) {
      if (seen.has(id)) continue;
      resizes?.unobserve(card);
      card.remove();
      cards.delete(id);
    }
    place();
    drawEdges(path);
    if (owesCenter) center(owesCenter);
  }

  // A card that changes size (a value wraps, a font loads) is laid out again; a canvas that first
  // gets its size is fitted then.
  const View = doc.defaultView;
  let frame: number | undefined;
  const later = () => {
    if (frame !== undefined || !View) return;
    frame = View.requestAnimationFrame(() => {
      frame = undefined;
      const before = laidOutKey;
      place();
      if (laidOutKey !== before) drawEdges(selected ? pathOf(graph, selected) : undefined);
      if (owesFit) fit(false);
      if (owesCenter) center(owesCenter);
    });
  };
  const resizes = View && 'ResizeObserver' in View ? new View.ResizeObserver(later) : undefined;
  resizes?.observe(element);

  // Pan by dragging the background; a click on it, without a drag, lets go of the pin.
  let drag: { id: number, x: number, y: number, vx: number, vy: number, moved: boolean } | undefined;
  element.addEventListener('pointerdown', (event) => {
    if (event.button !== 0 || (event.target as Element).closest?.('.lcard, .controls')) return;
    drag = { id: event.pointerId, x: event.clientX, y: event.clientY, vx: view.x, vy: view.y, moved: false };
    element.setPointerCapture?.(event.pointerId);
  });
  element.addEventListener('pointermove', (event) => {
    if (!drag || event.pointerId !== drag.id) return;
    const [dx, dy] = [event.clientX - drag.x, event.clientY - drag.y];
    if (!drag.moved && Math.hypot(dx, dy) < 3) return;
    drag.moved = true;
    element.classList.add('panning');
    view.x = drag.vx + dx;
    view.y = drag.vy + dy;
    apply();
  });
  const release = (event: PointerEvent) => {
    if (!drag || event.pointerId !== drag.id) return;
    const { moved } = drag;
    drag = undefined;
    element.classList.remove('panning');
    if (!moved && event.type === 'pointerup') options.onSelect(undefined);
  };
  element.addEventListener('pointerup', release);
  element.addEventListener('pointercancel', release);
  element.addEventListener('wheel', (event) => {
    event.preventDefault();
    const at = element.getBoundingClientRect();
    const factor = Math.exp(-event.deltaY * (event.deltaMode ? 0.05 : 0.002));
    zoomAt(event.clientX - at.left, event.clientY - at.top, view.k * factor);
  }, { passive: false });
  apply();

  return {
    element,
    draw,
    center,
    dispose: () => {
      resizes?.disconnect();
      if (frame !== undefined) View?.cancelAnimationFrame(frame);
    },
  };
}
