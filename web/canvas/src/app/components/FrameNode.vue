<script setup lang="ts">
// A live screen: the app at the frame's route, in an iframe, under its scenario. Above it, its
// label (drag it to move the frame); over it, the outlines of the hovered and picked elements and
// the pins of comments on its elements.
import { computed, nextTick, ref } from 'vue';
import { update, type Frame } from '../../model/index.ts';
import { frameName } from '../../frame/protocol.ts';
import { pinNumber, useStore } from '../context.ts';
import Icon from './Icon.vue';

const props = defineProps<{ frame: Frame }>();
const store = useStore();
const { state, board } = store;

const selected = computed(() => state.selection.includes(props.frame.id));
const reloads = ref(0);
const key = computed(() => `${props.frame.route}|${props.frame.scenario ?? ''}|${reloads.value}`);
const current = computed(() => state.routes[props.frame.id]);

const box = (r: { x: number, y: number, width: number, height: number }) => ({
  left: `${r.x}px`, top: `${r.y}px`, width: `${r.width}px`, height: `${r.height}px`,
});

const hoverBox = computed(() => (state.hover?.frame === props.frame.id ? state.hover.info.rect : null));
const pickedBox = computed(() => {
  void state.layout;
  const p = state.picked;
  if (p?.frame !== props.frame.id) return null;
  return store.locate(p.info.ref)?.rect ?? null;
});

const pins = computed(() => {
  void state.layout;
  if (!state.attached[props.frame.id]) return [];
  return (board.value?.comments ?? [])
    .filter((c) => c.anchor.type === 'element' && c.anchor.frame === props.frame.id && (c.status === 'open' || state.focus === c.id))
    .flatMap((c) => {
      const at = c.anchor.type === 'element' ? store.locate(c.anchor)?.rect : undefined;
      return at ? [{ id: c.id, n: pinNumber(store, c.id), resolved: c.status === 'resolved', x: at.x + at.width, y: at.y, rect: at }] : [];
    });
});

const focused = computed(() => pins.value.find((p) => p.id === state.focus)?.rect ?? null);

const editing = ref(false);
const input = ref<HTMLInputElement>();
async function rename () {
  editing.value = true;
  await nextTick();
  input.value?.select();
}
function done (e: Event) {
  const name = (e.target as HTMLInputElement).value.trim();
  editing.value = false;
  if (board.value && name && name !== props.frame.name) store.commit(update<Frame>(board.value, props.frame.id, { name }));
}
</script>

<template>
  <div
    class="ac-frame"
    :class="{ selected }"
    :style="{ left: `${frame.x}px`, top: `${frame.y}px`, width: `${frame.width}px`, height: `${frame.height}px` }"
  >
    <div class="ac-frame-label" :style="{ width: `calc(${frame.width}px / var(--ac-inv))` }" data-hit="move" :data-id="frame.id" @dblclick="rename">
      <input v-if="editing" ref="input" class="ac-frame-rename" :value="frame.name" data-ui @keydown.enter="done" @keydown.esc="editing = false" @blur="done" />
      <span v-else class="ac-frame-name">{{ frame.name }}</span>
      <span class="ac-chip" :class="{ violet: frame.scenario && frame.scenario !== 'live' }">{{ frame.scenario ?? 'default' }}</span>
      <span v-if="current && current !== frame.route" class="ac-frame-route ac-mono">{{ current }}</span>
      <span class="ac-spacer" />
      <button class="ac-frame-reload" type="button" title="Reload" aria-label="Reload the frame" data-ui @click="reloads++">
        <Icon name="reload" :size="12" />
      </button>
    </div>
    <iframe
      :key="key"
      class="ac-frame-screen"
      :src="frame.route"
      :name="frameName(frame.id, frame.scenario)"
      :title="frame.name"
    />
    <div class="ac-frame-overlay">
      <div v-if="hoverBox && state.tool !== 'play'" class="ac-outline hover" :style="box(hoverBox)" />
      <div v-if="pickedBox" class="ac-outline picked" :style="box(pickedBox)" />
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
  box-shadow: 0 0 0 calc(1px * var(--ac-inv)) var(--ac-border-strong);
}
.ac-frame.selected { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); }
.ac-frame-label {
  position: absolute;
  left: 0;
  bottom: 100%;
  display: flex;
  align-items: center;
  gap: 6px;
  height: 28px;
  padding: 0 2px 4px;
  transform: scale(var(--ac-inv));
  transform-origin: 0 100%;
  color: var(--ac-ink-weak);
  font-size: 12px;
  line-height: 16px;
  cursor: grab;
}
.ac-frame.selected .ac-frame-label { color: var(--ac-violet-text); }
.ac-frame-name { font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-frame-route { color: var(--ac-ink-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.ac-frame-rename { height: 22px; padding: 0 4px; border: 1px solid var(--ac-violet); border-radius: 6px; background: var(--ac-surface); font-size: 12px; }
.ac-frame-reload {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--ac-ink-muted);
  cursor: pointer;
}
.ac-frame-reload:hover { background: var(--ac-hover); color: var(--ac-ink); }
.ac-frame-screen { display: block; width: 100%; height: 100%; border: 0; background: var(--ac-surface); }
.ac-frame-overlay { position: absolute; inset: 0; pointer-events: none; overflow: hidden; }
.ac-outline { position: absolute; pointer-events: none; }
.ac-outline.hover { box-shadow: 0 0 0 calc(1px * var(--ac-inv)) var(--ac-violet); background: color-mix(in oklch, var(--ac-violet) 6%, transparent); }
.ac-outline.picked { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); }
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
  box-shadow: var(--ac-shadow);
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
  border-radius: 3px;
  background: var(--ac-surface);
  cursor: nwse-resize;
}
</style>
