<script setup lang="ts">
// Beside the canvas: what is on the board, the comments on it, and the components to place.
import { computed, ref } from 'vue';
import {
  commentMarkdown, contents, find, isShape, remove, reply, setStatus,
  type Comment, type Shape,
} from '../../model/index.ts';
import { pinNumber, useStore } from '../context.ts';
import Icon from './Icon.vue';

const store = useStore();
const { state, board } = store;
const tab = ref<'layers' | 'comments' | 'components'>('comments');
const filter = ref<'open' | 'resolved'>('open');
const drafts = ref<Record<string, string>>({});

const layers = computed(() => {
  const b = board.value;
  if (!b) return [];
  const inSection = new Set<string>();
  const groups = b.sections.map((s) => {
    const items = contents(b, s);
    for (const i of items) inSection.add(i.id);
    return { shape: s as Shape, items };
  });
  const loose = [...b.frames, ...b.notes].filter((s) => !inSection.has(s.id)).map((s) => ({ shape: s as Shape, items: [] as Shape[] }));
  return [...groups, ...loose];
});

const label = (s: Shape) => (s.kind === 'note' ? s.text.split('\n')[0] || 'Note' : s.name);

function focusShape (s: Shape) {
  state.selection = [s.id];
  store.zoomTo(s, state.view.zoom > 1 ? state.view.zoom : 1);
}

const comments = computed(() => (board.value?.comments ?? []).filter((c) => c.status === filter.value));
const openCount = computed(() => (board.value?.comments ?? []).filter((c) => c.status === 'open').length);

function where (c: Comment): string {
  if (c.anchor.type === 'canvas') return 'On the canvas';
  const f = board.value && find(board.value, c.anchor.frame);
  const el = c.anchor.component ? `<${c.anchor.component}>` : `<${c.anchor.tag}>`;
  return `${el} in ${f?.kind === 'frame' ? f.name : 'a removed frame'}`;
}

function focusComment (c: Comment) {
  state.focus = c.id;
  const target = c.anchor.type === 'element' ? find(board.value!, c.anchor.frame) : undefined;
  if (isShape(target)) store.zoomTo(target, Math.max(state.view.zoom, 0.6));
  else if (c.anchor.type === 'canvas') store.zoomTo({ x: c.anchor.x - 400, y: c.anchor.y - 300, width: 800, height: 600 }, state.view.zoom);
}

function setCommentStatus (c: Comment, status: Comment['status']) {
  if (board.value) store.commit(setStatus(board.value, c.id, status));
}

function sendReply (c: Comment) {
  const body = drafts.value[c.id]?.trim();
  if (!body || !board.value) return;
  store.commit(reply(board.value, c.id, state.settings.author, body));
  drafts.value[c.id] = '';
}

function startPlacing (name: string) {
  state.placing = name;
  state.tool = 'comment';
  store.flash(`Click an element in a frame to say where the ${name} goes`);
}

function onDragStart (e: DragEvent, name: string) {
  e.dataTransfer?.setData('application/x-anfra-component', name);
  state.dragging = true;
}
</script>

<template>
  <aside class="ac-panel ac-left" aria-label="Board">
    <div class="ac-tabs" role="tablist">
      <button role="tab" class="ac-btn ghost small" :aria-pressed="tab === 'comments'" :aria-selected="tab === 'comments'" @click="tab = 'comments'">
        Comments <span v-if="openCount" class="ac-chip violet">{{ openCount }}</span>
      </button>
      <button role="tab" class="ac-btn ghost small" :aria-pressed="tab === 'layers'" :aria-selected="tab === 'layers'" @click="tab = 'layers'">Layers</button>
      <button role="tab" class="ac-btn ghost small" :aria-pressed="tab === 'components'" :aria-selected="tab === 'components'" @click="tab = 'components'">Components</button>
    </div>

    <div v-if="tab === 'layers'" class="ac-section">
      <p v-if="!layers.length" class="ac-caption">Nothing on this board yet</p>
      <ul class="ac-layers">
        <li v-for="g in layers" :key="g.shape.id">
          <button class="ac-layer" :class="{ selected: state.selection.includes(g.shape.id) }" type="button" @click="focusShape(g.shape)">
            <Icon :name="g.shape.kind" :size="14" />
            <span class="ac-layer-name">{{ label(g.shape) }}</span>
          </button>
          <ul v-if="g.items.length">
            <li v-for="i in g.items" :key="i.id">
              <button class="ac-layer nested" :class="{ selected: state.selection.includes(i.id) }" type="button" @click="focusShape(i)">
                <Icon :name="i.kind" :size="14" />
                <span class="ac-layer-name">{{ label(i) }}</span>
                <span v-if="i.kind === 'frame'" class="ac-caption">{{ i.scenario ?? 'default' }}</span>
              </button>
            </li>
          </ul>
        </li>
      </ul>
    </div>

    <template v-else-if="tab === 'comments'">
      <div class="ac-section ac-row">
        <button class="ac-btn small" :aria-pressed="filter === 'open'" @click="filter = 'open'">Open</button>
        <button class="ac-btn small" :aria-pressed="filter === 'resolved'" @click="filter = 'resolved'">Resolved</button>
        <span class="ac-spacer" />
        <button class="ac-btn small" :disabled="!comments.length" title="Copy these comments as one Markdown task" @click="store.copyFeedback(comments)">
          <Icon name="copy" :size="14" /> Markdown
        </button>
      </div>
      <p v-if="!comments.length" class="ac-section ac-caption">
        {{ filter === 'open' ? 'No open comments. Press C and click an element of a screen to leave one.' : 'No resolved comments yet' }}
      </p>
      <article
        v-for="c in comments"
        :key="c.id"
        class="ac-section ac-comment"
        :class="{ active: state.focus === c.id }"
        @click="focusComment(c)"
      >
        <div class="ac-row">
          <span class="ac-pin static" :class="{ resolved: c.status === 'resolved' }">{{ pinNumber(store, c.id) }}</span>
          <span class="ac-caption ac-comment-where">{{ where(c) }}</span>
        </div>
        <p class="ac-comment-body">{{ c.body }}</p>
        <p class="ac-caption">{{ c.author }} · {{ new Date(c.createdAt).toLocaleString() }}</p>
        <div v-for="r in c.replies" :key="r.id" class="ac-reply">
          <p class="ac-caption"><strong>{{ r.author }}</strong></p>
          <p class="ac-comment-body">{{ r.body }}</p>
        </div>
        <div v-if="state.focus === c.id" class="ac-stack" @click.stop>
          <input v-model="drafts[c.id]" class="ac-field" placeholder="Reply" @keydown.enter="sendReply(c)" />
          <div class="ac-row">
            <button class="ac-btn small" title="Copy as Markdown" @click="store.copy(commentMarkdown(board!, c), 'Comment copied as Markdown')"><Icon name="copy" :size="14" /></button>
            <button class="ac-btn small" title="Delete" @click="board && store.commit(remove(board, [c.id]))"><Icon name="trash" :size="14" /></button>
            <span class="ac-spacer" />
            <button v-if="c.status === 'open'" class="ac-btn small" @click="setCommentStatus(c, 'resolved')"><Icon name="resolve" :size="14" /> Resolve</button>
            <button v-else class="ac-btn small" @click="setCommentStatus(c, 'open')"><Icon name="reopen" :size="14" /> Reopen</button>
          </div>
        </div>
      </article>
    </template>

    <div v-else class="ac-section ac-stack">
      <p class="ac-caption">Drag a component onto an element of a screen, or click one and then the element. It becomes a comment telling Claude where to add it.</p>
      <p v-if="!state.settings.components.length" class="ac-caption">No component folder is configured</p>
      <div class="ac-components">
        <button
          v-for="c in state.settings.components"
          :key="c"
          class="ac-component"
          :class="{ selected: state.placing === c }"
          type="button"
          draggable="true"
          @dragstart="onDragStart($event, c)"
          @dragend="state.dragging = false"
          @click="startPlacing(c)"
        >
          <Icon name="component" :size="14" /> {{ c }}
        </button>
      </div>
    </div>
  </aside>
</template>

<style>
.ac-left { border-right: 0; }
.ac-tabs { position: sticky; top: 0; z-index: 1; display: flex; gap: 4px; padding: 8px 12px; background: var(--ac-paper); border-bottom: 1px solid var(--ac-border); }
.ac-layers, .ac-layers ul { list-style: none; margin: 0; padding: 0; }
.ac-layer {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 30px;
  padding: 0 8px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--ac-ink-weak);
  text-align: left;
  cursor: pointer;
}
.ac-layer.nested { padding-left: 28px; }
.ac-layer:hover { background: var(--ac-hover); color: var(--ac-ink); }
.ac-layer.selected { background: var(--ac-wash); color: var(--ac-violet-text); }
.ac-layer-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-comment { display: grid; gap: 4px; cursor: pointer; }
.ac-comment:hover { background: var(--ac-hover); }
.ac-comment.active { background: var(--ac-surface); }
.ac-comment-where { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ac-comment-body { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.ac-reply { margin-left: 8px; padding-left: 10px; border-left: 2px solid var(--ac-border); }
.ac-pin.static { position: static; transform: none; box-shadow: none; border-width: 0; min-width: 20px; height: 20px; font-size: 11px; }
.ac-components { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.ac-component {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 8px;
  border: 1px solid var(--ac-border);
  border-radius: 8px;
  background: var(--ac-surface);
  font-size: 12px;
  cursor: grab;
  overflow: hidden;
  white-space: nowrap;
}
.ac-component:hover { border-color: var(--ac-border-strong); }
.ac-component.selected { border-color: var(--ac-violet); color: var(--ac-violet-text); }
</style>
