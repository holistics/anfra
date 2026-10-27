<script setup lang="ts">
import { computed, nextTick, ref } from 'vue';
import { update, type Section } from '../../model/index.ts';
import { useStore } from '../context.ts';

const props = defineProps<{ section: Section }>();
const store = useStore();
const { state, board } = store;
const selected = computed(() => state.selection.includes(props.section.id));

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
  if (board.value && name && name !== props.section.name) store.commit(update<Section>(board.value, props.section.id, { name }));
}
</script>

<template>
  <div
    class="ac-section-node"
    :class="{ selected }"
    :style="{ left: `${section.x}px`, top: `${section.y}px`, width: `${section.width}px`, height: `${section.height}px` }"
    data-hit="move"
    :data-id="section.id"
  >
    <div class="ac-section-title" @dblclick.stop="rename">
      <input v-if="editing" ref="input" class="ac-frame-rename" :value="section.name" data-ui @keydown.enter="done" @keydown.esc="editing = false" @blur="done" />
      <span v-else>{{ section.name }}</span>
    </div>
    <div v-if="selected && state.selection.length === 1" class="ac-handle" data-hit="resize" :data-id="section.id" />
  </div>
</template>

<style>
.ac-section-node {
  position: absolute;
  border-radius: calc(20px * var(--ac-inv));
  background: color-mix(in oklch, var(--ac-surface) 55%, transparent);
  box-shadow: 0 0 0 calc(1px * var(--ac-inv)) var(--ac-line-weak);
}
.ac-section-node.selected { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); }
.is-playing .ac-section-node { opacity: 0.6; }
.ac-section-title {
  position: absolute;
  left: 0;
  bottom: 100%;
  display: inline-flex;
  align-items: center;
  height: 32px;
  padding: 0 4px 8px;
  color: var(--ac-ink-weak);
  font-weight: 600;
  white-space: nowrap;
  transform: scale(var(--ac-inv));
  transform-origin: 0 100%;
  cursor: grab;
}
.ac-section-node.selected .ac-section-title { color: var(--ac-violet-text); }
</style>
