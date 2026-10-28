<script setup lang="ts">
// A live screen: the app at the frame's route, in an iframe, under its scenario. Above it, its
// label (drag it to move the frame); over it, the outlines of the hovered and picked elements and
// of the elements changed since the last commit, and the pins of comments on its elements. It can
// show its screen before the changes beside it now, split by a line to drag.
//
// Only screens near the view run. A frame's app starts when the frame first comes near the view
// and stops drawing while it is far; when many are running, one far for a while is let go, and a
// still of it stays in its place until it comes near again.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { update, type Frame } from '../../model/index.ts';
import { frameName } from '../../frame/protocol.ts';
import { hasScenarios, pinNumber, useStore } from '../context.ts';
import { intersects } from '../geometry.ts';
import Button from '../ui/Button.vue';
import Field from '../ui/Field.vue';
import Icon from '../ui/Icon.vue';
import { vTip } from '../ui/tooltip.ts';

/** Running apps kept, at most, beyond those near the view. */
const KEEP = 12;
/** How long a frame stays far before its app may be let go. */
const LET_GO_MS = 20_000;

const props = defineProps<{ frame: Frame }>();
const store = useStore();
const { state, board } = store;

const selected = computed(() => state.selection.includes(props.frame.id));
const current = computed(() => state.flow?.path.at(-1) === props.frame.id);
const key = computed(() => `${props.frame.route}|${props.frame.scenario ?? ''}|${state.reloads[props.frame.id] ?? 0}`);
const now = computed(() => state.routes[props.frame.id]);

/** Within the view, or half a view from it. */
const near = computed(() => {
  const { x, y, zoom } = state.view;
  const w = state.viewport.width / zoom;
  const h = state.viewport.height / zoom;
  return !!w && intersects(props.frame, { x: -x / zoom - w / 2, y: -y / zoom - h / 2, width: w * 2, height: h * 2 });
});
const alive = computed(() => !!state.alive[props.frame.id]);
const still = ref('');
let timer: ReturnType<typeof setTimeout> | undefined;
const letGo = () => {
  const running = Object.values(state.alive).filter(Boolean).length;
  if (near.value) return;
  if (running <= KEEP) {
    timer = setTimeout(letGo, LET_GO_MS);
    return;
  }
  const shot = store.snapshot(props.frame.id);
  if (shot) {
    URL.revokeObjectURL(still.value);
    still.value = URL.createObjectURL(new Blob([shot], { type: 'image/svg+xml' }));
  }
  state.alive[props.frame.id] = false;
};
watch(near, (n) => {
  clearTimeout(timer);
  if (n) state.alive[props.frame.id] = true;
  else if (alive.value) timer = setTimeout(letGo, LET_GO_MS);
}, { immediate: true });
// A still of the screen while it starts again, so coming back to it doesn't flash empty.
const showStill = computed(() => !!still.value && !(alive.value && state.attached[props.frame.id]));
onBeforeUnmount(() => {
  clearTimeout(timer);
  URL.revokeObjectURL(still.value);
  delete state.alive[props.frame.id];
});

const box = (r: { x: number, y: number, width: number, height: number }) => ({
  left: `${r.x}px`, top: `${r.y}px`, width: `${r.width}px`, height: `${r.height}px`,
});

const hoverBox = computed(() => (state.hover?.frame === props.frame.id ? state.hover.info.rect : null));
const pickedBox = computed(() => {
  void state.layouts[props.frame.id];
  const p = state.picked;
  if (p?.frame !== props.frame.id) return null;
  return store.locate(p.info.ref)?.rect ?? null;
});

const pins = computed(() => {
  void state.layouts[props.frame.id];
  if (!state.attached[props.frame.id]) return [];
  return (board.value?.comments ?? [])
    .filter((c) => c.anchor.type === 'element' && c.anchor.frame === props.frame.id && (c.status === 'open' || state.focus === c.id))
    .flatMap((c) => {
      const at = c.anchor.type === 'element' ? store.locate(c.anchor)?.rect : undefined;
      return at ? [{ id: c.id, n: pinNumber(store, c.id), resolved: c.status === 'resolved', x: at.x + at.width, y: at.y, rect: at }] : [];
    });
});

/** A palette item about to be dropped: a line before or after the element, or a box inside it. */
const drop = computed(() => {
  const d = state.drop;
  if (d?.frame !== props.frame.id) return null;
  const r = d.info.rect;
  if (d.where === 'inside') return { class: 'inside', style: box(r) };
  return { class: 'line', style: box({ x: r.x, y: d.where === 'before' ? r.y - 1 : r.y + r.height - 1, width: r.width, height: 2 }) };
});

/** The picked element's name, on its outline. */
const pickedName = computed(() => state.picked?.frame === props.frame.id ? state.picked.info.ref.component ?? state.picked.info.ref.tag : '');

const focused = computed(() => pins.value.find((p) => p.id === state.focus)?.rect ?? null);

/** The elements changed since the last commit (or on the branch), while the Changes tab is open. */
const changed = computed(() => {
  void state.layouts[props.frame.id];
  if (state.panel !== 'changes' || !state.outlineChanges || !state.attached[props.frame.id]) return [];
  return store.changedRects(props.frame.id);
});

// --- Before and now -------------------------------------------------------------------------------

const comparing = computed(() => state.compare?.frame === props.frame.id);
function compare () {
  state.compare = comparing.value ? null : { frame: props.frame.id, at: 0.5 };
}
// The still goes with the changes: once they are committed or undone, so is the comparing.
watch(() => store.hasStill(props.frame.id), (has) => {
  if (!has && comparing.value) state.compare = null;
});
const screen = ref<HTMLElement>();
function slide (e: PointerEvent) {
  const el = e.currentTarget as HTMLElement;
  el.setPointerCapture(e.pointerId);
  const move = (m: PointerEvent) => {
    const r = screen.value?.getBoundingClientRect();
    if (r && state.compare) state.compare.at = Math.min(1, Math.max(0, (m.clientX - r.left) / r.width));
  };
  move(e);
  el.addEventListener('pointermove', move);
  el.addEventListener('pointerup', () => el.removeEventListener('pointermove', move), { once: true });
}

const editing = ref(false);
const input = ref<InstanceType<typeof Field>>();
async function rename () {
  editing.value = true;
  await nextTick();
  input.value?.select();
}
watch(() => state.renaming === props.frame.id, (now) => {
  if (!now) return;
  state.renaming = '';
  void rename();
});
function done (e: Event) {
  const name = (e.target as HTMLInputElement).value.trim();
  editing.value = false;
  if (board.value && name && name !== props.frame.name) store.commit(update<Frame>(board.value, props.frame.id, { name }));
}
</script>

<template>
  <div
    class="ac-frame"
    :class="{ selected, current }"
    :style="{ left: `${frame.x}px`, top: `${frame.y}px`, width: `${frame.width}px`, height: `${frame.height}px` }"
  >
    <div class="ac-frame-label" :style="{ width: `calc(${frame.width}px / var(--ac-inv))` }" data-hit="move" :data-id="frame.id" @dblclick="rename">
      <Field v-if="editing" ref="input" class="ac-frame-rename" small :value="frame.name" aria-label="Name" data-ui @keydown.enter="done" @keydown.esc="editing = false" @blur="done" />
      <span v-else class="ac-frame-name">{{ frame.name }}</span>
      <span v-if="hasScenarios(store)" class="ac-chip" :class="{ live: frame.scenario === 'live' }" v-tip="state.settings.scenarios[frame.scenario ?? 'default']?.description">{{ state.settings.scenarios[frame.scenario ?? 'default']?.label ?? frame.scenario ?? 'default' }}</span>
      <span v-if="now && now !== frame.route" class="ac-frame-route ac-mono" v-tip="'Where the screen is now'">{{ now }}</span>
      <span class="ac-spacer" />
      <Button
        v-if="store.hasStill(frame.id)"
        variant="ghost"
        small
        icon="compare"
        label="Compare with before the changes"
        :aria-pressed="comparing"
        data-ui
        @click="compare"
      />
      <Button variant="ghost" small icon="reload" label="Reload the frame" data-ui @click="store.reload(frame.id)" />
    </div>
    <div ref="screen" class="ac-frame-screen" :class="{ far: !near }">
      <iframe
        v-if="alive"
        :key="key"
        :src="frame.route"
        :name="frameName(frame.id, frame.scenario)"
        :title="frame.name"
      />
      <img v-if="showStill" class="ac-frame-still" :src="still" alt="" />
      <!-- The screen before the changes, over the left of the live one: a document, so it loads the app's fonts -->
      <div v-if="comparing && state.compare" class="ac-compare" :style="{ '--at': `${state.compare.at * 100}%` }">
        <iframe class="ac-compare-before" :src="store.stillUrl(frame.id)" :title="`${frame.name}, before the changes`" tabindex="-1" />
        <span class="ac-compare-tag before">Before</span>
        <span class="ac-compare-tag now">Now</span>
        <div class="ac-compare-line" data-ui role="slider" aria-label="Before and now" :aria-valuenow="Math.round(state.compare.at * 100)" aria-valuemin="0" aria-valuemax="100" @pointerdown.stop="slide">
          <span class="ac-compare-knob"><Icon name="code" :size="14" /></span>
        </div>
      </div>
    </div>
    <div class="ac-frame-overlay">
      <div v-for="(r, i) in changed" :key="`c${i}`" class="ac-outline changed" :style="box(r)" />
      <div v-if="hoverBox && state.tool !== 'play'" class="ac-outline hover" :style="box(hoverBox)" />
      <div v-if="pickedBox" class="ac-outline picked" :style="box(pickedBox)" />
      <div v-if="drop" class="ac-outline drop" :class="drop.class" :style="drop.style" />
      <div v-if="focused" class="ac-outline focus" :style="box(focused)" />
      <button
        v-for="p in pins"
        :key="p.id"
        class="ac-pin"
        :class="{ resolved: p.resolved, active: state.focus === p.id }"
        :style="{ left: `${p.x}px`, top: `${p.y}px` }"
        type="button"
        data-ui
        :aria-label="`Comment ${p.n}`"
        @click.stop="state.focus = state.focus === p.id ? '' : p.id"
      >
        {{ p.n }}
      </button>
    </div>
    <!-- The picked element's name, above it: outside the frame's clip, so it shows over the frame's edge -->
    <div v-if="pickedBox" class="ac-outline-tag" :style="{ left: `${Math.min(Math.max(pickedBox.x, 0), frame.width)}px`, top: `${Math.min(Math.max(pickedBox.y, 0), frame.height)}px` }">
      <span class="ac-outline-name">{{ pickedName }}</span>
    </div>
    <div v-if="selected && state.selection.length === 1" class="ac-handle" data-hit="resize" :data-id="frame.id" />
  </div>
</template>
