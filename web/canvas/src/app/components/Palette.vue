<script setup lang="ts">
// The app's components, drawn as they look. Drag one onto a screen, or click it and then the
// element to put it before, after or in: it is written into that screen's template.
import { computed, ref } from 'vue';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';
import PreviewBox from './PreviewBox.vue';

const store = useStore();
const { state } = store;
const query = ref('');

const items = computed(() => {
  const q = query.value.trim().toLowerCase();
  return state.settings.palette.filter((i) => !q || i.name.toLowerCase().includes(q) || i.id.includes(q));
});

const place = (id: string) => store.place(id);

function onDragStart (e: DragEvent, id: string) {
  e.dataTransfer?.setData('application/x-anfra-component', id);
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'copy';
  state.dragging = true;
}
</script>

<template>
  <div class="ac-palette">
    <label class="ac-affix ac-palette-search">
      <Icon name="search" />
      <input v-model="query" class="ac-field" type="search" placeholder="Search components" aria-label="Search components" />
    </label>
    <p v-if="!state.settings.palette.length" class="ac-caption ac-list-intro">No component folder is configured</p>
    <p v-else-if="!items.length" class="ac-caption ac-list-intro">No component matches</p>
    <div class="ac-palette-grid">
      <button
        v-for="i in items"
        :key="i.id"
        class="ac-palette-item"
        :aria-pressed="state.placing === i.id"
        type="button"
        draggable="true"
        :title="`Drag onto a screen, or click and then click where it goes`"
        @dragstart="onDragStart($event, i.id)"
        @dragend="state.dragging = false; state.drop = null"
        @click="place(i.id)"
      >
        <PreviewBox :id="i.id" />
        <span class="ac-palette-name ac-truncate">{{ i.name }}</span>
      </button>
    </div>
  </div>
</template>

<style>
.ac-palette { display: grid; gap: 12px; }
.ac-palette-search > svg { color: var(--ac-icon-weak); }
.ac-palette-grid { display: grid; gap: 8px; }
.ac-palette-item {
  display: grid;
  min-width: 0;
  padding: 0;
  border: 0;
  border-radius: 10px;
  background: var(--ac-surface);
  box-shadow: 0 0 0 1px var(--ac-line-weak);
  text-align: left;
  cursor: grab;
  transition: box-shadow 160ms var(--ac-ease), scale 90ms var(--ac-ease);
}
.ac-palette-item:active { scale: 0.97; }
.ac-palette-name { padding: 6px 10px 8px; font-size: 12px; line-height: 16px; font-weight: 500; }
.ac-palette-item[aria-pressed='true'] { box-shadow: 0 0 0 2px var(--ac-violet); }
@media (hover: hover) and (pointer: fine) {
  .ac-palette-item:hover { box-shadow: 0 0 0 1px var(--ac-line-hover), var(--ac-e1); }
  .ac-palette-item[aria-pressed='true']:hover { box-shadow: 0 0 0 2px var(--ac-violet); }
}
</style>
