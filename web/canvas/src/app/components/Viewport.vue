<script setup lang="ts">
// The canvas itself: the world of sections, frames, notes and arrows, panned and zoomed, and every
// pointer gesture on it. Nodes mark where a gesture starts (`data-hit`); this decides what it does.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import {
  addArrow, addNote, addSection, find, isShape, move, resize, shapes,
  type Frame, type Point, type Rect, type Shape,
} from '../../model/index.ts';
import { useStore } from '../context.ts';
import { between, contains, intersects } from '../geometry.ts';
import FrameNode from './FrameNode.vue';
import SectionNode from './SectionNode.vue';
import NoteNode from './NoteNode.vue';
import ArrowLayer from './ArrowLayer.vue';
import FlowBar from './FlowBar.vue';
import CanvasPins from './CanvasPins.vue';
import Composer from './Composer.vue';
import AddFrame from './AddFrame.vue';
import Toolbar from './Toolbar.vue';

const store = useStore();
const { state, board } = store;
const el = ref<HTMLElement>();

type Drag =
  | { kind: 'pan', start: Point, view: Point }
  | { kind: 'move', ids: string[], start: Point, from: NonNullable<typeof board.value> }
  | { kind: 'resize', id: string, start: Point, rect: Rect, from: NonNullable<typeof board.value> }
  | { kind: 'marquee', start: Point, at: Point, add: boolean }
  | { kind: 'section', start: Point, at: Point }
  | { kind: 'arrow', from: string, at: Point };

const drag = ref<Drag | null>(null);
const adding = ref<{ at: Point, screen: Point } | null>(null);

const local = (e: { clientX: number, clientY: number }): Point => {
  const r = el.value!.getBoundingClientRect();
  return { x: e.clientX - r.left, y: e.clientY - r.top };
};
const world = (e: { clientX: number, clientY: number }) => store.toWorld(local(e));

/** The topmost shape at a world point: notes, then frames, then sections. */
function shapeAt (p: Point): Shape | undefined {
  const b = board.value;
  if (!b) return undefined;
  return [...b.notes].reverse().find((s) => contains(s, p))
    ?? [...b.frames].reverse().find((s) => contains({ ...s, y: s.y - 40 / state.view.zoom, height: s.height + 40 / state.view.zoom }, p))
    ?? [...b.sections].reverse().find((s) => contains(s, p));
}

function select (id: string, add: boolean) {
  if (add) state.selection = state.selection.includes(id) ? state.selection.filter((x) => x !== id) : [...state.selection, id];
  else if (!state.selection.includes(id)) state.selection = [id];
  if (state.picked && !state.selection.includes(state.picked.frame)) state.picked = null;
}

function onPointerDown (e: PointerEvent) {
  if (!board.value || (e.target as HTMLElement).closest('[data-ui]')) return;
  const p = world(e);
  const hit = (e.target as HTMLElement).closest<HTMLElement>('[data-hit]');
  const id = hit?.dataset.id;
  if (e.button === 1 || state.panning || state.tool === 'hand') {
    return start(e, { kind: 'pan', start: local(e), view: { x: state.view.x, y: state.view.y } });
  }
  if (e.button !== 0) return;
  state.composer = null;
  adding.value = null;

  switch (state.tool) {
    case 'select':
    case 'play':
      if (hit?.dataset.hit === 'resize' && id) {
        const s = find(board.value, id);
        if (isShape(s)) start(e, { kind: 'resize', id, start: p, rect: { x: s.x, y: s.y, width: s.width, height: s.height }, from: board.value });
        return;
      }
      if (hit?.dataset.hit === 'move' && id) {
        select(id, e.shiftKey);
        return start(e, { kind: 'move', ids: [...state.selection], start: p, from: board.value });
      }
      if (hit?.dataset.hit === 'select' && id) return select(id, e.shiftKey);
      if (!e.shiftKey) {
        state.selection = [];
        state.picked = null;
      }
      return start(e, { kind: 'marquee', start: p, at: p, add: e.shiftKey });
    case 'comment':
      state.composer = { anchor: { type: 'canvas', ...p }, at: local(e), body: '' };
      return;
    case 'frame':
      adding.value = { at: p, screen: local(e) };
      return;
    case 'note': {
      const r = addNote(board.value, { x: p.x, y: p.y });
      store.commit(r.board);
      state.selection = [r.note.id];
      state.focus = r.note.id;
      state.tool = 'select';
      return;
    }
    case 'section':
      return start(e, { kind: 'section', start: p, at: p });
    case 'arrow': {
      const from = shapeAt(p);
      if (from) start(e, { kind: 'arrow', from: from.id, at: p });
      return;
    }
  }
}

function start (e: PointerEvent, d: Drag) {
  e.preventDefault();
  drag.value = d;
  state.dragging = true;
  window.addEventListener('pointermove', onMove);
  window.addEventListener('pointerup', onUp, { once: true });
}

function onMove (e: PointerEvent) {
  const d = drag.value;
  if (!d || !board.value) return;
  const p = world(e);
  switch (d.kind) {
    case 'pan': {
      const at = local(e);
      state.view.x = d.view.x + at.x - d.start.x;
      state.view.y = d.view.y + at.y - d.start.y;
      break;
    }
    case 'move':
      store.preview(move(d.from, d.ids, { x: Math.round(p.x - d.start.x), y: Math.round(p.y - d.start.y) }));
      break;
    case 'resize':
      store.preview(resize(d.from, d.id, { ...d.rect, width: d.rect.width + p.x - d.start.x, height: d.rect.height + p.y - d.start.y }));
      break;
    case 'marquee':
    case 'section':
    case 'arrow':
      d.at = p;
      break;
  }
}

function onUp () {
  window.removeEventListener('pointermove', onMove);
  const d = drag.value;
  drag.value = null;
  state.dragging = false;
  if (!d || !board.value) return;
  switch (d.kind) {
    case 'move':
    case 'resize':
      store.endDrag();
      break;
    case 'marquee': {
      const box = between(d.start, d.at);
      if (box.width < 4 && box.height < 4) break;
      const inside = shapes(board.value).filter((s) => intersects(s, box)
        && !(s.kind === 'section' && !(box.x <= s.x && box.y <= s.y && box.x + box.width >= s.x + s.width && box.y + box.height >= s.y + s.height)));
      state.selection = [...new Set([...(d.add ? state.selection : []), ...inside.map((s) => s.id)])];
      break;
    }
    case 'section': {
      const box = between(d.start, d.at);
      const r = box.width > 40 && box.height > 40 ? addSection(board.value, { ...box }) : addSection(board.value, { x: d.start.x, y: d.start.y });
      store.commit(r.board);
      state.selection = [r.section.id];
      state.tool = 'select';
      break;
    }
    case 'arrow': {
      const to = shapeAt(d.at);
      if (to && to.id !== d.from) {
        const r = addArrow(board.value, d.from, to.id);
        store.commit(r.board);
        if (r.arrow) state.selection = [r.arrow.id];
      }
      break;
    }
  }
}

// --- Zoom and pan ---------------------------------------------------------------------------------

function zoomAt (screen: Point, factor: number) {
  const before = store.toWorld(screen);
  state.view.zoom = Math.min(4, Math.max(0.05, state.view.zoom * factor));
  state.view.x = screen.x - before.x * state.view.zoom;
  state.view.y = screen.y - before.y * state.view.zoom;
}

function wheel (screen: Point, dx: number, dy: number, zoom: boolean) {
  if (zoom) zoomAt(screen, Math.exp(-dy * 0.01));
  else {
    state.view.x -= dx;
    state.view.y -= dy;
  }
}

const onWheel = (e: WheelEvent) => {
  e.preventDefault();
  wheel(local(e), e.deltaX, e.deltaY, e.ctrlKey || e.metaKey);
};

// A frame forwards the wheel from inside it, at a point of its own viewport.
store.handlers.wheel = (frameId, e) => {
  const f = board.value && find(board.value, frameId);
  if (f?.kind !== 'frame') return;
  wheel(store.toScreen({ x: f.x + e.x, y: f.y + e.y }), e.deltaX, e.deltaY, e.zoom);
};

// A component dropped onto a frame: a comment on the element it landed on, for the agent to place.
store.handlers.pickedForComment = (frameId, info, point) => {
  const f = board.value && find(board.value, frameId);
  if (f?.kind !== 'frame') return;
  const at = store.toScreen({ x: f.x + point.x, y: f.y + point.y });
  const body = state.placing ? `Add a \`<${componentName(state.placing)}>\` here` : '';
  state.composer = { anchor: { type: 'element', ...info.ref }, at, body };
  state.placing = '';
};

const componentName = (dir: string) => dir.split('-').map((w) => w[0].toUpperCase() + w.slice(1)).join('');

function onDragOver (e: DragEvent) {
  if (!e.dataTransfer?.types.includes('application/x-anfra-component')) return;
  e.preventDefault();
  const p = world(e);
  const f = board.value?.frames.find((s) => contains(s, p));
  state.hover = f ? (() => {
    const info = store.pickAt(f.id, p.x - f.x, p.y - f.y);
    return info ? { frame: f.id, info } : null;
  })() : null;
}

function onDrop (e: DragEvent) {
  const name = e.dataTransfer?.getData('application/x-anfra-component');
  state.dragging = false;
  if (!name || !board.value) return;
  e.preventDefault();
  const p = world(e);
  const f = board.value.frames.find((s) => contains(s, p));
  const info = f && store.pickAt(f.id, p.x - f.x, p.y - f.y);
  state.hover = null;
  if (!f || !info) return store.flash('Drop a component onto an element of a frame');
  state.placing = name;
  store.handlers.pickedForComment(f.id, info, { x: p.x - f.x, y: p.y - f.y });
}

function addFrameAt (frame: Frame) {
  if (!board.value) return;
  store.commit({ ...board.value, frames: [...board.value.frames, frame] });
  state.selection = [frame.id];
  adding.value = null;
  state.tool = 'select';
}

// --- Size ----------------------------------------------------------------------------------------

let observer: ResizeObserver | undefined;
onMounted(() => {
  observer = new ResizeObserver(([entry]) => {
    state.viewport.width = entry.contentRect.width;
    state.viewport.height = entry.contentRect.height;
  });
  observer.observe(el.value!);
});
onBeforeUnmount(() => observer?.disconnect());

const transform = computed(() => `translate(${state.view.x}px, ${state.view.y}px) scale(${state.view.zoom})`);
const marquee = computed(() => (drag.value?.kind === 'marquee' || drag.value?.kind === 'section') ? between(drag.value.start, drag.value.at) : null);
const draftArrow = computed(() => {
  const d = drag.value;
  if (d?.kind !== 'arrow' || !board.value) return null;
  const from = find(board.value, d.from);
  return isShape(from) ? { from, to: d.at } : null;
});
const cursor = computed(() => {
  if (drag.value?.kind === 'pan') return 'grabbing';
  if (state.panning || state.tool === 'hand') return 'grab';
  if (['frame', 'section', 'note', 'arrow', 'comment'].includes(state.tool)) return 'crosshair';
  return 'default';
});

defineExpose({ zoomAt });
</script>

<template>
  <div
    ref="el"
    class="ac-viewport"
    :class="{ 'is-playing': !!state.flow, 'is-dragging': state.dragging, 'is-idle': state.tool !== 'select' && state.tool !== 'comment' && state.tool !== 'play' || state.panning }"
    :style="{ cursor }"
    @pointerdown="onPointerDown"
    @wheel="onWheel"
    @dragover="onDragOver"
    @drop="onDrop"
  >
    <div v-if="board" class="ac-world" :style="{ transform, '--ac-inv': 1 / state.view.zoom }">
      <SectionNode v-for="s in board.sections" :key="s.id" :section="s" />
      <ArrowLayer part="lines" :draft="draftArrow" />
      <FrameNode v-for="f in board.frames" :key="f.id" :frame="f" />
      <NoteNode v-for="n in board.notes" :key="n.id" :note="n" />
      <ArrowLayer part="labels" />
      <CanvasPins />
      <div
        v-if="marquee"
        class="ac-marquee"
        :class="{ section: drag?.kind === 'section' }"
        :style="{ left: `${marquee.x}px`, top: `${marquee.y}px`, width: `${marquee.width}px`, height: `${marquee.height}px` }"
      />
    </div>

    <div v-if="board && !board.frames.length && !board.sections.length && !board.notes.length" class="ac-empty" data-ui>
      <p class="ac-title">An empty board</p>
      <p class="ac-caption">Press F and click to put a screen of the app here, or ask Claude to lay out a flow</p>
    </div>

    <Composer v-if="state.composer" />
    <AddFrame v-if="adding" :at="adding.at" :screen="adding.screen" @add="addFrameAt" @cancel="adding = null" />
    <FlowBar v-if="state.flow" />
    <Toolbar v-else @zoom="(f) => zoomAt({ x: state.viewport.width / 2, y: state.viewport.height / 2 }, f)" />
    <div v-if="state.toast" :key="state.toast" class="ac-toast" data-ui role="status">{{ state.toast }}</div>
  </div>
</template>

<style>
.ac-viewport {
  position: relative;
  /* clip, not hidden: a hidden box still scrolls when something in a frame takes focus */
  overflow: clip;
  background-color: var(--ac-sunken);
  background-image: radial-gradient(circle, var(--ac-line) 1px, transparent 1.5px);
  background-size: 24px 24px;
  box-shadow: 0 0 0 1px var(--ac-line-weak);
  border-radius: 14px;
  margin: 0 8px 8px;
  touch-action: none;
  user-select: none;
}
.ac-world { position: absolute; left: 0; top: 0; transform-origin: 0 0; }
.ac-viewport.is-dragging iframe, .ac-viewport.is-idle iframe { pointer-events: none; }
.ac-marquee {
  position: absolute;
  border: calc(1px * var(--ac-inv)) solid var(--ac-violet);
  background: color-mix(in oklch, var(--ac-violet) 8%, transparent);
  pointer-events: none;
}
.ac-marquee.section { border-style: dashed; }
.ac-empty {
  position: absolute;
  left: 50%;
  top: 40%;
  transform: translate(-50%, -50%);
  max-width: 360px;
  text-align: center;
  display: grid;
  gap: 8px;
}
.ac-toast {
  position: absolute;
  top: 12px;
  left: 50%;
  translate: -50% 0;
  max-width: min(560px, calc(100% - 32px));
  padding: 6px 12px;
  border-radius: 10px;
  background: var(--ac-inverted);
  color: var(--ac-on-inverted);
  font-size: 12px;
  line-height: 16px;
  box-shadow: var(--ac-e2);
  transform-origin: top;
  animation: ac-pop-in 160ms var(--ac-ease);
}
</style>
