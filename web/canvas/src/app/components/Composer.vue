<script setup lang="ts">
// Writing a new comment, by the element or spot it is about.
import { computed, onMounted, ref } from 'vue';
import { elementMarkdown } from '../../model/index.ts';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';

const store = useStore();
const { state, board } = store;
const area = ref<HTMLTextAreaElement>();
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
  area.value?.setSelectionRange(c.value.body.length, c.value.body.length);
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
    <textarea
      ref="area"
      v-model="c.body"
      class="ac-field"
      rows="3"
      placeholder="What should change? Claude reads this"
      @keydown.meta.enter.prevent="send"
      @keydown.ctrl.enter.prevent="send"
      @keydown.esc="state.composer = null"
    />
    <div class="ac-row">
      <button v-if="c.anchor.type === 'element'" class="ac-btn ghost icon" type="button" title="Copy as Markdown" aria-label="Copy the element and this comment as Markdown" @click="copy"><Icon name="copy" /></button>
      <span class="ac-spacer" />
      <button class="ac-btn ghost" type="button" @click="state.composer = null">Cancel</button>
      <button class="ac-btn primary" type="submit" :disabled="!c.body.trim()">Comment</button>
    </div>
  </form>
</template>

<style>
.ac-composer { position: absolute; width: 320px; padding: 12px; display: grid; gap: 8px; z-index: 5; transform-origin: top left; }
.ac-composer-what { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-composer textarea.ac-field { min-height: 72px; }
</style>
