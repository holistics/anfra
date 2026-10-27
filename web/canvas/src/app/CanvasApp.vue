<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { bounds, duplicate, shapes } from '../model/index.ts';
import { provideStore } from './context.ts';
import { TOOLS, type Store } from './store.ts';
import Viewport from './components/Viewport.vue';
import LeftPanel from './components/LeftPanel.vue';
import Inspector from './components/Inspector.vue';
import Icon from './components/Icon.vue';

const props = defineProps<{ store: Store }>();
const store = props.store;
provideStore(store);
const { state, board } = store;
const viewport = ref<InstanceType<typeof Viewport>>();

const creating = ref(false);
const newName = ref('');
const slugOf = (name: string) => name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 64);

async function createBoard () {
  const slug = slugOf(newName.value);
  if (!slug) return;
  await store.create(slug, newName.value.trim());
  creating.value = false;
  newName.value = '';
}

const pick = (slug: string) => {
  location.hash = slug;
};

const openCount = computed(() => board.value?.comments.filter((c) => c.status === 'open').length ?? 0);

function fitAll () {
  if (board.value) store.zoomTo(bounds(shapes(board.value)));
}

// --- Keyboard --------------------------------------------------------------------------------------

function shortcut (key: string, meta: boolean, shift: boolean): boolean {
  const k = key.toLowerCase();
  if (meta && k === 'z') {
    if (shift) store.redo();
    else store.undo();
    return true;
  }
  if (meta && k === 'd') {
    if (board.value && state.selection.length) {
      const r = duplicate(board.value, state.selection);
      store.commit(r.board);
      state.selection = r.ids;
    }
    return true;
  }
  if (meta && k === 'a') {
    if (board.value) state.selection = shapes(board.value).map((s) => s.id);
    return true;
  }
  if (meta && (k === '=' || k === '+' || k === '-')) {
    viewport.value?.zoomAt({ x: state.viewport.width / 2, y: state.viewport.height / 2 }, k === '-' ? 1 / 1.25 : 1.25);
    return true;
  }
  if (meta) return false;
  if (shift && (k === '!' || k === '1')) {
    fitAll();
    return true;
  }
  if (shift && (k === '@' || k === '2')) {
    store.zoomTo(bounds(store.selected().filter((o) => 'width' in o) as never));
    return true;
  }
  if (k === 'escape') {
    if (state.composer) state.composer = null;
    else if (state.placing) state.placing = '';
    else if (state.picked) state.picked = null;
    else state.selection = [];
    state.focus = '';
    return true;
  }
  if (k === 'backspace' || k === 'delete') {
    store.removeSelection();
    return true;
  }
  const tool = TOOLS.find((t) => t.key === k);
  if (tool) {
    state.tool = tool.tool;
    return true;
  }
  return false;
}

const typing = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));

function onKeyDown (e: KeyboardEvent) {
  if (typing(e.target)) return;
  if (e.key === ' ' && !e.repeat) {
    state.panning = true;
    e.preventDefault();
    return;
  }
  if (shortcut(e.key, e.metaKey || e.ctrlKey, e.shiftKey)) e.preventDefault();
}
function onKeyUp (e: KeyboardEvent) {
  if (e.key === ' ') state.panning = false;
}
store.handlers.key = (e) => {
  if (e.key === ' ') return;
  shortcut(e.key, e.meta, e.shift);
};

async function openFromHash () {
  const slug = decodeURIComponent(location.hash.slice(1));
  const target = slug || state.boards[0]?.slug;
  if (target && target !== state.slug) {
    await store.open(target);
    requestAnimationFrame(fitAll);
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKeyDown);
  window.addEventListener('keyup', onKeyUp);
  window.addEventListener('hashchange', openFromHash);
  await store.loadList();
  await openFromHash();
});
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown);
  window.removeEventListener('keyup', onKeyUp);
  window.removeEventListener('hashchange', openFromHash);
});
</script>

<template>
  <div class="ac">
    <header class="ac-top">
      <span class="ac-heading">Canvas</span>
      <select class="ac-field ac-board-pick" :value="state.slug" aria-label="Board" @change="pick(($event.target as HTMLSelectElement).value)">
        <option v-if="!state.boards.length" value="">No boards</option>
        <option v-for="b in state.boards" :key="b.slug" :value="b.slug">{{ b.name }}{{ b.open ? ` (${b.open})` : '' }}</option>
      </select>
      <form v-if="creating" class="ac-row" @submit.prevent="createBoard">
        <input v-model="newName" class="ac-field" placeholder="New board's name" autofocus @keydown.esc="creating = false" />
        <button class="ac-btn small" type="submit" :disabled="!slugOf(newName)">Create</button>
      </form>
      <button v-else class="ac-btn ghost small" type="button" @click="creating = true"><Icon name="plus" :size="14" /> New board</button>
      <span class="ac-spacer" />
      <span class="ac-caption">{{ state.saving === 'saving' ? 'Saving…' : state.saving === 'error' ? 'Not saved' : 'Saved' }}</span>
      <button class="ac-btn ghost icon small" type="button" title="Undo (⌘Z)" aria-label="Undo" :disabled="!state.history.past" @click="store.undo()"><Icon name="undo" :size="14" /></button>
      <button class="ac-btn ghost icon small" type="button" title="Redo (⇧⌘Z)" aria-label="Redo" :disabled="!state.history.future" @click="store.redo()"><Icon name="redo" :size="14" /></button>
      <button class="ac-btn small" type="button" :disabled="!openCount" title="Copy every open comment as one Markdown task" @click="store.copyFeedback()">
        <Icon name="copy" :size="14" /> Copy feedback
      </button>
    </header>
    <LeftPanel />
    <main class="ac-main">
      <p v-if="state.error" class="ac-error" role="alert">{{ state.error }}</p>
      <div v-else-if="!state.slug && !state.boards.length" class="ac-start">
        <p class="ac-heading">No boards yet</p>
        <p class="ac-caption">A board is a canvas of screens. Create one to start.</p>
        <button class="ac-btn primary" type="button" @click="creating = true">New board</button>
      </div>
      <Viewport v-else ref="viewport" />
    </main>
    <Inspector />
  </div>
</template>

<style>
.ac-top { grid-column: 1 / -1; display: flex; align-items: center; gap: 8px; padding: 0 16px; }
.ac-board-pick { width: auto; min-width: 200px; max-width: 320px; }
.ac-main { position: relative; display: grid; min-height: 0; overflow: clip; }
.ac-main > .ac-viewport { min-height: 0; }
.ac-error, .ac-start { align-self: center; justify-self: center; display: grid; gap: 8px; text-align: center; max-width: 420px; }
.ac-error { color: var(--ac-danger); }
</style>
