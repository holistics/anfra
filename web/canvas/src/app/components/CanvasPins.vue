<script setup lang="ts">
// Comments pinned to a spot on the canvas rather than an element: a remark on a whole flow.
import { computed } from 'vue';
import { pinNumber, useStore } from '../context.ts';

const store = useStore();
const { state, board } = store;

const pins = computed(() => (board.value?.comments ?? []).flatMap((c) =>
  c.anchor.type === 'canvas' && (c.status === 'open' || state.focus === c.id)
    ? [{ id: c.id, n: pinNumber(store, c.id), x: c.anchor.x, y: c.anchor.y, resolved: c.status === 'resolved' }]
    : []));
</script>

<template>
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
</template>
