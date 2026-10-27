<script setup lang="ts">
// The app's components, drawn as they look. Drag one onto a screen, or click it and then the
// element to put it before, after or in: it is written into that screen's template.
import { computed, ref } from 'vue';
import { useStore } from '../context.ts';
import Field from '../ui/Field.vue';
import { vTip } from '../ui/tooltip.ts';
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
    <Field v-model="query" icon="search" type="search" placeholder="Search components" aria-label="Search components" />
    <p v-if="!state.settings.palette.length" class="ac-caption ac-list-intro">No component folder is configured</p>
    <p v-else-if="!items.length" class="ac-caption ac-list-intro">No component matches</p>
    <div class="ac-palette-grid">
      <button
        v-for="i in items"
        :key="i.id"
        class="ac-tile"
        :aria-pressed="state.placing === i.id"
        type="button"
        draggable="true"
        v-tip="'Drag onto a screen, or click it and then where it goes'"
        @dragstart="onDragStart($event, i.id)"
        @dragend="state.dragging = false; state.drop = null"
        @click="place(i.id)"
      >
        <PreviewBox :id="i.id" />
        <span class="ac-tile-name ac-truncate">{{ i.name }}</span>
      </button>
    </div>
  </div>
</template>
