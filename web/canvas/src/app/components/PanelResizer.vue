<script setup lang="ts">
// The edge between a panel and the board. Drag it to make the panel wider or narrower, and well past
// its narrowest to hide it. Double-click it for the panel's first width. With focus, ← and → move it.
import { computed, ref } from 'vue';
import { useStore } from '../context.ts';
import { PANEL_WIDTH, clampWidth, type Side } from '../panels.ts';

const props = defineProps<{ side: Side }>();
const { state } = useStore();

const limits = computed(() => PANEL_WIDTH[props.side]);
/** Dragging right widens the left panel and narrows the right one. */
const sign = computed(() => (props.side === 'left' ? 1 : -1));
const start = ref<{ x: number, width: number } | null>(null);

function down (e: PointerEvent) {
  if (e.button !== 0) return;
  (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  start.value = { x: e.clientX, width: state.panels[props.side] };
  // The frames stop taking the pointer, so the drag isn't lost over one.
  state.dragging = true;
}

function move (e: PointerEvent) {
  if (!start.value) return;
  const width = start.value.width + sign.value * (e.clientX - start.value.x);
  if (width < limits.value.min - 96) {
    // Hidden at the width it had, for when it shows again.
    state.panels[props.side] = start.value.width;
    up();
    state.panels[`${props.side}Open`] = false;
    return;
  }
  state.panels[props.side] = clampWidth(props.side, width);
}

function up () {
  if (!start.value) return;
  start.value = null;
  state.dragging = false;
}

function key (e: KeyboardEvent) {
  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
  e.preventDefault();
  e.stopPropagation();
  const by = (e.key === 'ArrowRight' ? 16 : -16) * sign.value;
  state.panels[props.side] = clampWidth(props.side, state.panels[props.side] + by);
}
</script>

<template>
  <div
    class="ac-resizer"
    :class="[side, { active: start }]"
    role="separator"
    aria-orientation="vertical"
    :aria-label="`Resize the ${side} panel`"
    :aria-valuenow="state.panels[side]"
    :aria-valuemin="limits.min"
    :aria-valuemax="limits.max"
    tabindex="0"
    data-ui
    @pointerdown="down"
    @pointermove="move"
    @pointerup="up"
    @pointercancel="up"
    @lostpointercapture="up"
    @dblclick="state.panels[side] = limits.initial"
    @keydown="key"
  />
</template>
