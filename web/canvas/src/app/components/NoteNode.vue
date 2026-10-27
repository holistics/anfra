<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { update, type Note } from '../../model/index.ts';
import { useStore } from '../context.ts';

const props = defineProps<{ note: Note }>();
const store = useStore();
const { state, board } = store;
const selected = computed(() => state.selection.includes(props.note.id));

const editing = ref(false);
const area = ref<HTMLTextAreaElement>();
async function edit () {
  editing.value = true;
  await nextTick();
  area.value?.focus();
}
function done () {
  const text = area.value?.value ?? '';
  editing.value = false;
  if (board.value && text !== props.note.text) store.commit(update<Note>(board.value, props.note.id, { text }));
}
watch(() => state.focus, (id) => {
  if (id === props.note.id) {
    state.focus = '';
    void edit();
  }
}, { immediate: true });
</script>

<template>
  <div
    class="ac-note"
    :class="{ selected }"
    :style="{ left: `${note.x}px`, top: `${note.y}px`, width: `${note.width}px`, height: `${note.height}px` }"
    data-hit="move"
    :data-id="note.id"
    @dblclick.stop="edit"
  >
    <div class="ac-note-body">
      <textarea v-if="editing" ref="area" class="ac-note-text" :value="note.text" data-ui placeholder="Write a note" @blur="done" @keydown.esc="done" />
      <p v-else class="ac-note-text" :class="{ empty: !note.text }">{{ note.text || 'Double-click to write' }}</p>
    </div>
    <div v-if="selected && state.selection.length === 1" class="ac-handle" data-hit="resize" :data-id="note.id" />
  </div>
</template>
