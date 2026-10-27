<script setup lang="ts">
// A live screen: the app at the frame's route, in an iframe, under its scenario. Above it, its
// label (drag it to move the frame); over it, the outlines of the hovered and picked elements and
// the pins of comments on its elements.
//
// Only screens near the view run. A frame's app starts when the frame first comes near the view
// and stops drawing while it is far; when many are running, one far for a while is let go, and a
// still of it stays in its place until it comes near again.
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { update, type Frame } from '../../model/index.ts';
import { frameName } from '../../frame/protocol.ts';
import { pinNumber, useStore } from '../context.ts';
import { intersects } from '../geometry.ts';
import Icon from './Icon.vue';

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

const editing = ref(false);
const input = ref<HTMLInputElement>();
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
      <input v-if="editing" ref="input" class="ac-frame-rename" :value="frame.name" data-ui @keydown.enter="done" @keydown.esc="editing = false" @blur="done" />
      <span v-else class="ac-frame-name">{{ frame.name }}</span>
      <span class="ac-chip" :class="{ live: frame.scenario === 'live' }" :title="state.settings.scenarios[frame.scenario ?? 'default']?.description">{{ state.settings.scenarios[frame.scenario ?? 'default']?.label ?? frame.scenario ?? 'default' }}</span>
      <span v-if="now && now !== frame.route" class="ac-frame-route ac-mono" :title="`Now at ${now}`">{{ now }}</span>
      <span class="ac-spacer" />
      <button class="ac-frame-reload" type="button" title="Reload" aria-label="Reload the frame" data-ui @click="store.reload(frame.id)">
        <Icon name="reload" :size="12" />
      </button>
    </div>
    <div class="ac-frame-screen" :class="{ far: !near }">
      <iframe
        v-if="alive"
        :key="key"
        :src="frame.route"
        :name="frameName(frame.id, frame.scenario)"
        :title="frame.name"
      />
      <img v-if="showStill" class="ac-frame-still" :src="still" alt="" />
    </div>
    <div class="ac-frame-overlay">
      <div v-if="hoverBox && state.tool !== 'play'" class="ac-outline hover" :style="box(hoverBox)" />
      <div v-if="pickedBox" class="ac-outline picked" :style="box(pickedBox)">
        <span class="ac-outline-name">{{ pickedName }}</span>
      </div>
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
    <div v-if="selected && state.selection.length === 1" class="ac-handle" data-hit="resize" :data-id="frame.id" />
  </div>
</template>

<style>
.ac-frame {
  position: absolute;
  background: var(--ac-surface);
  box-shadow: 0 0 0 calc(1px * var(--ac-inv)) var(--ac-line), 0 calc(1px * var(--ac-inv)) calc(3px * var(--ac-inv)) rgb(24 20 31 / 0.06);
  transition: opacity 220ms var(--ac-ease);
}
.ac-frame.selected, .ac-frame.current { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); }
.is-playing .ac-frame:not(.current) { opacity: 0.4; }
.is-playing .ac-frame:not(.current) iframe { pointer-events: none; }
.ac-frame-label {
  position: absolute;
  left: 0;
  bottom: 100%;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 0 8px;
  transform: scale(var(--ac-inv));
  transform-origin: 0 100%;
  color: var(--ac-ink-weak);
  font-size: 12px;
  line-height: 16px;
  cursor: grab;
}
.ac-frame.selected .ac-frame-label, .ac-frame.current .ac-frame-label { color: var(--ac-violet-text); }
.ac-frame-name { flex: 0 1 auto; min-width: 48px; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-frame-label .ac-chip { display: block; flex: 0 100 auto; min-width: 0; height: 18px; line-height: 18px; overflow: hidden; text-overflow: ellipsis; font-weight: 400; }
.ac-frame-label .ac-chip.live::before { content: ''; width: 6px; height: 6px; border-radius: 999px; background: var(--ac-success-icon); }
.ac-frame-route { color: var(--ac-ink-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.ac-frame-rename { height: 24px; padding: 0 6px; border: 1px solid var(--ac-violet-line); border-radius: 6px; background: var(--ac-surface); font-size: 12px; outline: none; }
.ac-frame-reload {
  position: relative;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--ac-icon-weak);
  cursor: pointer;
  transition: background-color 160ms var(--ac-ease), color 160ms var(--ac-ease);
}
@media (hover: hover) and (pointer: fine) {
  .ac-frame-reload:hover { background: var(--ac-hover); color: var(--ac-ink); }
}
.ac-frame-screen { position: relative; width: 100%; height: 100%; background: var(--ac-surface); }
.ac-frame-screen iframe { display: block; width: 100%; height: 100%; border: 0; }
/* Far from the view: kept, but not drawn */
.ac-frame-screen.far { content-visibility: hidden; }
.ac-frame-still { position: absolute; inset: 0; width: 100%; height: 100%; pointer-events: none; }
.ac-frame-overlay { position: absolute; inset: 0; pointer-events: none; overflow: hidden; }
.ac-outline { position: absolute; pointer-events: none; }
.ac-outline.hover { box-shadow: 0 0 0 calc(1px * var(--ac-inv)) var(--ac-violet); background: color-mix(in oklch, var(--ac-violet) 6%, transparent); }
.ac-outline.picked { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); }
.ac-outline-name {
  position: absolute;
  left: calc(-2px * var(--ac-inv));
  bottom: 100%;
  margin-bottom: calc(2px * var(--ac-inv));
  padding: 0 6px;
  border-radius: 4px;
  background: var(--ac-violet);
  color: var(--ac-on-violet);
  font-size: 12px;
  line-height: 18px;
  font-weight: 500;
  white-space: nowrap;
  transform: scale(var(--ac-inv));
  transform-origin: bottom left;
}
.ac-outline.drop.line { background: var(--ac-violet); border-radius: 2px; box-shadow: none; }
.ac-outline.drop.inside { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); background: color-mix(in oklch, var(--ac-violet) 10%, transparent); }
.ac-outline.focus { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); background: color-mix(in oklch, var(--ac-violet) 10%, transparent); }
.ac-pin {
  position: absolute;
  display: grid;
  place-items: center;
  min-width: 24px;
  height: 24px;
  padding: 0 6px;
  border: 2px solid var(--ac-surface);
  border-radius: 999px 999px 999px 4px;
  background: var(--ac-violet);
  color: var(--ac-on-violet);
  font-size: 12px;
  font-weight: 600;
  transform: translate(-30%, -70%) scale(var(--ac-inv));
  transform-origin: 30% 70%;
  pointer-events: auto;
  cursor: pointer;
  box-shadow: var(--ac-e2);
}
.ac-pin.resolved { background: var(--ac-ink-muted); }
.ac-pin.active { outline: 2px solid var(--ac-violet); outline-offset: 2px; }
.ac-handle {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 12px;
  height: 12px;
  transform: translate(50%, 50%) scale(var(--ac-inv));
  border: 2px solid var(--ac-violet);
  border-radius: 4px;
  background: var(--ac-surface);
  cursor: nwse-resize;
}
</style>
