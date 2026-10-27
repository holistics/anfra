<script setup lang="ts">
// Writing a new comment, by the element or spot it is about.
import { computed, onMounted, ref } from 'vue';
import { elementMarkdown } from '../../model/index.ts';
import { useStore } from '../context.ts';
import Button from '../ui/Button.vue';
import Field from '../ui/Field.vue';

const store = useStore();
const { state, board } = store;
const area = ref<InstanceType<typeof Field>>();
const c = computed(() => state.composer!);

const what = computed(() => {
  const a = c.value.anchor;
  if (a.type !== 'element') return 'On the canvas';
  return `${a.component ? `<${a.component}>` : `<${a.tag}>`}${a.text ? ` "${a.text.slice(0, 40)}"` : ''}`;
});

const style = computed(() => ({
  left: `${Math.min(c.value.at.x + 16, state.viewport.width - 336)}px`,
  top: `${Math.min(Math.max(c.value.at.y - 20, 8), state.viewport.height - 220)}px`,
}));

onMounted(() => {
  area.value?.focus();
  area.value?.el?.setSelectionRange(c.value.body.length, c.value.body.length);
});

function send () {
  const made = store.comment(c.value.anchor, c.value.body);
  if (made) state.focus = made.id;
  state.composer = null;
}

function copy () {
  const a = c.value.anchor;
  if (a.type !== 'element' || !board.value) return;
  void store.copy(`${elementMarkdown(board.value, a)}${c.value.body.trim() ? `\n\n> ${c.value.body.trim()}` : ''}`, 'Copied as Markdown');
}
</script>

<template>
  <form class="ac-composer ac-popover ac-pop-in" :style="style" data-ui @submit.prevent="send" @pointerdown.stop>
    <p class="ac-caption ac-composer-what">{{ what }}</p>
    <Field
      ref="area"
      v-model="c.body"
      :rows="3"
      placeholder="What should change? Claude reads this"
      @keydown.meta.enter.prevent="send"
      @keydown.ctrl.enter.prevent="send"
      @keydown.esc="state.composer = null"
    />
    <div class="ac-row">
      <Button v-if="c.anchor.type === 'element'" variant="ghost" icon="copy" label="Copy the element and this comment as Markdown" tip="Copy as Markdown" @click="copy" />
      <span class="ac-spacer" />
      <Button variant="ghost" @click="state.composer = null">Cancel</Button>
      <Button variant="primary" type="submit" :disabled="!c.body.trim()">Comment</Button>
    </div>
  </form>
</template>
