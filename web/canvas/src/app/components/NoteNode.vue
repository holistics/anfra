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

<style>
.ac-note {
  position: absolute;
  background: var(--ac-note);
  box-shadow: 0 0 0 calc(1px * var(--ac-inv)) var(--ac-note-line), 0 calc(1px * var(--ac-inv)) calc(3px * var(--ac-inv)) rgb(24 20 31 / 0.06);
  border-radius: calc(10px * var(--ac-inv));
}
.ac-note.selected { box-shadow: 0 0 0 calc(2px * var(--ac-inv)) var(--ac-violet); }
.ac-note-body { height: 100%; padding: 16px 20px; font-size: 20px; line-height: 28px; color: var(--ac-ink); }
.ac-note-text {
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  resize: none;
  white-space: pre-wrap;
  overflow: hidden;
  outline: none;
}
.ac-note-text.empty { color: var(--ac-ink-muted); }
.ac-note-text::placeholder { color: var(--ac-ink-muted); }
</style>
