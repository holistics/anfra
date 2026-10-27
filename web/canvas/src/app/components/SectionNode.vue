<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { update, type Section } from '../../model/index.ts';
import { useStore } from '../context.ts';
import Field from '../ui/Field.vue';

const props = defineProps<{ section: Section }>();
const store = useStore();
const { state, board } = store;
const selected = computed(() => state.selection.includes(props.section.id));

const editing = ref(false);
const input = ref<InstanceType<typeof Field>>();
async function rename () {
  editing.value = true;
  await nextTick();
  input.value?.select();
}
watch(() => state.renaming === props.section.id, (now) => {
  if (!now) return;
  state.renaming = '';
  void rename();
});
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
      <Field v-if="editing" ref="input" class="ac-frame-rename" small :value="section.name" aria-label="Name" data-ui @keydown.enter="done" @keydown.esc="editing = false" @blur="done" />
      <span v-else>{{ section.name }}</span>
    </div>
    <div v-if="selected && state.selection.length === 1" class="ac-handle" data-hit="resize" :data-id="section.id" />
  </div>
</template>
