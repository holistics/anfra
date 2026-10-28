<script setup lang="ts">
// A board drawn small, for its card until it has a cover: its sections and its screens.
import { computed } from 'vue';
import type { BoardSummary } from '../../../vite/boards.ts';

const props = defineProps<{ map: BoardSummary['map'] }>();

const box = computed(() => {
  const all = [...props.map.sections, ...props.map.frames];
  if (!all.length) return undefined;
  const x = Math.min(...all.map((r) => r[0]));
  const y = Math.min(...all.map((r) => r[1]));
  const w = Math.max(...all.map((r) => r[0] + r[2])) - x;
  const h = Math.max(...all.map((r) => r[1] + r[3])) - y;
  const pad = Math.max(w, h) * 0.06;
  return `${x - pad} ${y - pad} ${w + pad * 2} ${h + pad * 2}`;
});
</script>

<template>
  <svg v-if="box" class="ac-board-map" :viewBox="box" preserveAspectRatio="xMidYMid meet" aria-hidden="true">
    <rect v-for="(r, i) in map.sections" :key="`s${i}`" class="section" :x="r[0]" :y="r[1]" :width="r[2]" :height="r[3]" :rx="Math.min(r[2], r[3]) * 0.02" />
    <rect v-for="(r, i) in map.frames" :key="`f${i}`" class="frame" :x="r[0]" :y="r[1]" :width="r[2]" :height="r[3]" :rx="Math.min(r[2], r[3]) * 0.02" />
  </svg>
  <span v-else class="ac-board-map empty ac-caption">No screens yet</span>
</template>
