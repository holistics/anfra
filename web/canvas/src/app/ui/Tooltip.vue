<script setup lang="ts">
// The one tooltip on screen (see tooltip.ts): a dark label with the shortcut beside it, above or
// below what it names and kept inside the window.
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { hideTip, tip } from './tooltip.ts';

const el = ref<HTMLElement>();
const left = ref(0);
// Drawn hidden first, to measure its width, then put in place.
const placed = ref(false);
watch(() => [tip.open, tip.x, tip.y, tip.text, tip.keys], async () => {
  placed.value = false;
  if (!tip.open) return;
  await nextTick();
  const w = el.value?.offsetWidth ?? 0;
  left.value = Math.min(Math.max(8, tip.x - w / 2), window.innerWidth - w - 8);
  placed.value = true;
}, { immediate: true });

// Gone as soon as the board moves or a key is pressed.
const hide = () => hideTip();
onMounted(() => {
  window.addEventListener('wheel', hide, { passive: true, capture: true });
  window.addEventListener('keydown', hide, true);
});
onBeforeUnmount(() => {
  window.removeEventListener('wheel', hide, true);
  window.removeEventListener('keydown', hide, true);
});
</script>

<template>
  <div
    v-if="tip.open"
    ref="el"
    class="ac-tooltip"
    :class="{ above: tip.above }"
    :style="{ left: `${left}px`, top: `${tip.y}px`, visibility: placed ? undefined : 'hidden' }"
    role="tooltip"
  >
    <span>{{ tip.text }}</span>
    <span v-if="tip.keys" class="ac-tooltip-keys">{{ tip.keys }}</span>
  </div>
</template>
