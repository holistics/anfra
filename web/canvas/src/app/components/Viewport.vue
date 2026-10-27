<script setup lang="ts">
// The canvas itself: the world of sections, frames, notes and arrows, panned and zoomed, and every
// pointer gesture on it. Nodes mark where a gesture starts (`data-hit`); this decides what it does.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
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
import ContextMenu from './ContextMenu.vue';
import { menuFor, type CommandGroup } from '../commands.ts';
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

// --- The right-click menu ------------------------------------------------------------------------

const menu = ref<{ at: Point, groups: CommandGroup[] } | null>(null);
watch(() => [state.view.x, state.view.y, state.view.zoom], () => (menu.value = null));

/** On the canvas: for the shape under the pointer (selected first), or for the empty canvas. */
function onContextMenu (e: MouseEvent) {
  e.preventDefault();
  if (!board.value || state.flow) return;
  const at = local(e);
  const p = store.toWorld(at);
  const hit = (e.target as HTMLElement).closest<HTMLElement>('[data-hit]')?.dataset.id ?? shapeAt(p)?.id;
  state.picked = null;
  state.composer = null;
  adding.value = null;
  if (hit) {
    if (!state.selection.includes(hit)) state.selection = [hit];
    menu.value = { at, groups: menuFor(store, 'selection') };
  } else {
    state.selection = [];
    menu.value = { at, groups: menuFor(store, 'canvas', p) };
  }
}

// On an element of a screen: the frame tells where, and the element is picked first.
store.handlers.menu = (frameId, point) => {
  const f = board.value && find(board.value, frameId);
  if (f?.kind !== 'frame') return;
  menu.value = { at: store.toScreen({ x: f.x + point.x, y: f.y + point.y }), groups: menuFor(store, 'element') };
};

store.handlers.addScreen = (at) => {
  adding.value = { at, screen: store.toScreen(at) };
};

// An element picked with the comment tool: a comment on it, written where it is.
store.handlers.pickedForComment = (frameId, info, point) => {
  const f = board.value && find(board.value, frameId);
  if (f?.kind !== 'frame') return;
  const at = store.toScreen({ x: f.x + point.x, y: f.y + point.y });
  state.composer = { anchor: { type: 'element', ...info.ref }, at, body: '' };
};

/** Where a palette item dragged to a world point would land: the element under it, and beside or in it. */
function dropAt (p: Point) {
  const f = board.value?.frames.find((s) => contains(s, p));
  const info = f && store.pickAt(f.id, p.x - f.x, p.y - f.y);
  return f && info ? { frame: f.id, info, where: store.whereIn(info, p.y - f.y) } : null;
}

function onDragOver (e: DragEvent) {
  if (!e.dataTransfer?.types.includes('application/x-anfra-component')) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'copy';
  state.drop = dropAt(world(e));
}

function onDrop (e: DragEvent) {
  const id = e.dataTransfer?.getData('application/x-anfra-component');
  state.dragging = false;
  const target = state.drop ?? (id ? dropAt(world(e)) : null);
  state.drop = null;
  if (!id || !board.value) return;
  e.preventDefault();
  const item = state.settings.palette.find((i) => i.id === id);
  if (!item) return;
  if (!target) return store.flash('Drop it onto a screen, on the element it goes beside or in');
  void store.insert(target.frame, target.info, target.where, item);
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

// While the view moves, the world is one layer, moved and scaled as it was drawn, and drawn sharp
// again once it stops: a pan or a wheel turn doesn't redraw every screen at each step.
const moving = ref(false);
let settle: ReturnType<typeof setTimeout> | undefined;
watch(() => [state.view.x, state.view.y, state.view.zoom], () => {
  moving.value = true;
  clearTimeout(settle);
  settle = setTimeout(() => (moving.value = false), 180);
});
onBeforeUnmount(() => clearTimeout(settle));
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
    :class="{ 'is-playing': !!state.flow, 'is-dragging': state.dragging, 'is-moving': moving, 'is-idle': state.tool !== 'select' && state.tool !== 'comment' && state.tool !== 'play' || state.panning }"
    :style="{ cursor }"
    @pointerdown="onPointerDown"
    @wheel="onWheel"
    @dragover="onDragOver"
    @dragleave="(e) => !el?.contains(e.relatedTarget as Node) && (state.drop = null)"
    @drop="onDrop"
    @contextmenu="onContextMenu"
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
    <ContextMenu v-if="menu" :groups="menu.groups" :at="menu.at" @close="menu = null" />
    <AddFrame v-if="adding" :at="adding.at" :screen="adding.screen" @add="addFrameAt" @cancel="adding = null" />
    <FlowBar v-if="state.flow" />
    <Toolbar v-else @zoom="(f) => zoomAt({ x: state.viewport.width / 2, y: state.viewport.height / 2 }, f)" />
    <div v-if="state.toast" :key="state.toast" class="ac-toast" data-ui role="status">{{ state.toast }}</div>
  </div>
</template>
