<script setup lang="ts">
// Beside the canvas: what is on the board, the comments on it, and the components to place.
import { computed, ref, watch } from 'vue';
import {
  commentMarkdown, contents, find, isShape, remove, reply, setStatus,
  type Comment, type Shape,
} from '../../model/index.ts';
import { pinNumber, useStore } from '../context.ts';
import Icon from './Icon.vue';
import LayerTree from './LayerTree.vue';
import Palette from './Palette.vue';

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

// Picking an element on a screen opens that frame's layers, down to it.
watch(() => state.picked?.frame, (frame) => {
  if (frame) state.layersOpen[frame] = true;
});

const scenarioLabel = (id = 'default') => state.settings.scenarios[id]?.label ?? (id === 'live' ? 'Live backend' : id);

/** "2 min ago", "Yesterday", or a date: when a comment was written, as people say it. */
function when (iso: string): string {
  const t = new Date(iso).getTime();
  const min = Math.round((Date.now() - t) / 60000);
  if (min < 1) return 'Just now';
  if (min < 60) return `${min} min ago`;
  const h = Math.round(min / 60);
  if (h < 24) return `${h} h ago`;
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

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

</script>

<template>
  <aside class="ac-panel ac-left" aria-label="Board">
    <div class="ac-left-head">
      <div class="ac-seg fill" role="tablist" aria-label="Board panel">
        <button role="tab" type="button" :aria-selected="tab === 'comments'" @click="tab = 'comments'">
          Comments <span v-if="openCount" class="ac-count">{{ openCount }}</span>
        </button>
        <button role="tab" type="button" :aria-selected="tab === 'layers'" @click="tab = 'layers'">Layers</button>
        <button role="tab" type="button" :aria-selected="tab === 'components'" @click="tab = 'components'">Components</button>
      </div>
    </div>

    <div v-if="tab === 'layers'" class="ac-list" role="tabpanel">
      <p v-if="!layers.length" class="ac-caption ac-list-empty">Nothing on this board yet</p>
      <ul class="ac-layers">
        <li v-for="g in layers" :key="g.shape.id">
          <div class="ac-layer" :class="{ group: g.shape.kind === 'section' }" :aria-current="state.selection.includes(g.shape.id) || undefined">
            <button v-if="g.shape.kind === 'frame'" class="ac-tree-toggle" type="button" :aria-expanded="!!state.layersOpen[g.shape.id]" aria-label="Show its layers" @click="state.layersOpen[g.shape.id] = !state.layersOpen[g.shape.id]">
              <Icon name="chevronRight" :size="14" />
            </button>
            <button class="ac-layer-name" type="button" @click="focusShape(g.shape)">
              <Icon :name="g.shape.kind" :size="16" />
              <span class="ac-truncate">{{ label(g.shape) }}</span>
              <span v-if="g.shape.kind === 'frame'" class="ac-layer-meta">{{ scenarioLabel(g.shape.scenario) }}</span>
            </button>
          </div>
          <LayerTree v-if="g.shape.kind === 'frame' && state.layersOpen[g.shape.id]" :frame="g.shape.id" />
          <ul v-if="g.items.length">
            <li v-for="i in g.items" :key="i.id">
              <div class="ac-layer nested" :aria-current="state.selection.includes(i.id) || undefined">
                <button v-if="i.kind === 'frame'" class="ac-tree-toggle" type="button" :aria-expanded="!!state.layersOpen[i.id]" aria-label="Show its layers" @click="state.layersOpen[i.id] = !state.layersOpen[i.id]">
                  <Icon name="chevronRight" :size="14" />
                </button>
                <span v-else class="ac-tree-toggle" />
                <button class="ac-layer-name" type="button" @click="focusShape(i)">
                  <Icon :name="i.kind" :size="16" />
                  <span class="ac-truncate">{{ label(i) }}</span>
                  <span v-if="i.kind === 'frame'" class="ac-layer-meta">{{ scenarioLabel(i.scenario) }}</span>
                </button>
              </div>
              <LayerTree v-if="i.kind === 'frame' && state.layersOpen[i.id]" :frame="i.id" />
            </li>
          </ul>
        </li>
      </ul>
    </div>

    <div v-else-if="tab === 'comments'" class="ac-list" role="tabpanel">
      <div class="ac-row ac-list-tools">
        <div class="ac-seg" role="group" aria-label="Show">
          <button type="button" :aria-pressed="filter === 'open'" @click="filter = 'open'">Open</button>
          <button type="button" :aria-pressed="filter === 'resolved'" @click="filter = 'resolved'">Resolved</button>
        </div>
        <span class="ac-spacer" />
        <button class="ac-btn ghost icon" type="button" :disabled="!comments.length" title="Copy these comments as one Markdown task" aria-label="Copy these comments as Markdown" @click="store.copyFeedback(comments)">
          <Icon name="copy" />
        </button>
      </div>
      <div v-if="!comments.length" class="ac-list-empty">
        <p class="ac-heading">{{ filter === 'open' ? 'No open comments' : 'Nothing resolved yet' }}</p>
        <p class="ac-caption">{{ filter === 'open' ? 'Press C and click an element of a screen to leave one for Claude' : 'Comments Claude or you resolve land here, with the note on what changed' }}</p>
      </div>
      <article
        v-for="c in comments"
        :key="c.id"
        class="ac-comment"
        :class="{ active: state.focus === c.id }"
        @click="focusComment(c)"
      >
        <div class="ac-row">
          <span class="ac-pin static" :class="{ resolved: c.status === 'resolved' }">{{ pinNumber(store, c.id) }}</span>
          <span class="ac-caption ac-truncate">{{ where(c) }}</span>
        </div>
        <p class="ac-comment-body">{{ c.body }}</p>
        <p class="ac-caption">{{ c.author }} · {{ when(c.createdAt) }}</p>
        <div v-for="r in c.replies" :key="r.id" class="ac-reply">
          <p class="ac-caption"><span class="ac-reply-author">{{ r.author }}</span> · {{ when(r.createdAt) }}</p>
          <p class="ac-comment-body">{{ r.body }}</p>
        </div>
        <div v-if="state.focus === c.id" class="ac-stack ac-comment-actions" @click.stop>
          <input v-model="drafts[c.id]" class="ac-field" placeholder="Reply" aria-label="Reply" @keydown.enter="sendReply(c)" />
          <div class="ac-row">
            <button class="ac-btn ghost icon" type="button" title="Copy as Markdown" aria-label="Copy this comment as Markdown" @click="store.copy(commentMarkdown(board!, c), 'Comment copied as Markdown')"><Icon name="copy" /></button>
            <button class="ac-btn ghost icon" type="button" title="Delete" aria-label="Delete this comment" @click="board && store.commit(remove(board, [c.id]))"><Icon name="trash" /></button>
            <span class="ac-spacer" />
            <button v-if="c.status === 'open'" class="ac-btn" type="button" @click="setCommentStatus(c, 'resolved')"><Icon name="resolve" /> Resolve</button>
            <button v-else class="ac-btn" type="button" @click="setCommentStatus(c, 'open')"><Icon name="reopen" /> Reopen</button>
          </div>
        </div>
      </article>
    </div>

    <div v-else class="ac-list" role="tabpanel">
      <Palette />
    </div>
  </aside>
</template>

<style>
.ac-left-head .ac-seg > button { flex: 1 1 auto; }
.ac-left-head { position: sticky; top: 0; z-index: 1; padding: 8px 8px 8px 12px; background: var(--ac-paper); }
.ac-list { display: grid; align-content: start; gap: 2px; padding: 0 8px 16px 12px; }
.ac-list > * { min-width: 0; }
.ac-list-tools { padding: 4px 0 8px; }
.ac-list-empty { display: grid; gap: 4px; padding: 24px 8px; text-align: center; }
.ac-list-empty.ac-caption { padding: 16px 8px; }
.ac-list-intro { padding: 4px 4px 12px; }
.ac-layers, .ac-layers ul { list-style: none; margin: 0; padding: 0; }
.ac-layer {
  display: flex;
  align-items: center;
  height: var(--ac-control);
  padding: 0 4px;
  border-radius: 6px;
  color: var(--ac-ink-weak);
  transition: background-color 160ms var(--ac-ease), color 160ms var(--ac-ease);
}
.ac-layer-name {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  height: 100%;
  padding: 0 4px;
  border: 0;
  background: none;
  text-align: left;
  cursor: pointer;
}
.ac-layer-name > svg { flex-shrink: 0; color: var(--ac-icon-weak); }
.ac-layer-name > .ac-truncate { flex: 1; }
.ac-layer.group .ac-layer-name { padding-left: 24px; color: var(--ac-ink); font-weight: 500; }
.ac-layer.nested { padding-left: 16px; }
.ac-layers ul .ac-tree { padding-left: 36px; }
.ac-layer-meta { flex-shrink: 0; max-width: 40%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ac-ink-muted); font-size: 12px; line-height: 16px; }
.ac-layer[aria-current] { background: var(--ac-selected); color: var(--ac-violet-text); }
.ac-layer[aria-current] .ac-layer-name > svg { color: var(--ac-violet-icon); }
@media (hover: hover) and (pointer: fine) {
  .ac-layer:not([aria-current]):hover { background: var(--ac-hover); color: var(--ac-ink); }
}

.ac-comment {
  display: grid;
  gap: 6px;
  padding: 12px;
  border-radius: 10px;
  cursor: pointer;
  transition: background-color 160ms var(--ac-ease), box-shadow 160ms var(--ac-ease);
}
.ac-comment + .ac-comment { margin-top: 2px; }
.ac-comment.active { background: var(--ac-surface); box-shadow: 0 0 0 1px var(--ac-line), var(--ac-e1); cursor: default; }
@media (hover: hover) and (pointer: fine) {
  .ac-comment:not(.active):hover { background: var(--ac-hover); }
}
.ac-comment-body { white-space: pre-wrap; overflow-wrap: anywhere; text-wrap: pretty; }
.ac-reply { display: grid; gap: 2px; margin-top: 4px; padding-left: 12px; border-left: 2px solid var(--ac-line); }
.ac-reply-author { color: var(--ac-ink-weak); font-weight: 500; }
.ac-comment-actions { margin-top: 6px; }
.ac-comment-actions .ac-row { gap: 4px; }
.ac-pin.static { position: static; flex-shrink: 0; transform: none; box-shadow: none; border-width: 0; min-width: 20px; height: 20px; padding: 0 5px; }

.ac-components { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
.ac-component {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  height: var(--ac-control);
  padding: 0 10px;
  border: 0;
  border-radius: 10px;
  background: var(--ac-surface);
  box-shadow: 0 0 0 1px var(--ac-line);
  color: var(--ac-ink);
  text-align: left;
  cursor: grab;
  transition: box-shadow 160ms var(--ac-ease), background-color 160ms var(--ac-ease);
}
.ac-component > svg { flex-shrink: 0; color: var(--ac-icon-weak); }
.ac-component[aria-pressed='true'] { background: var(--ac-selected); box-shadow: 0 0 0 1px var(--ac-violet-line); color: var(--ac-violet-text); }
.ac-component[aria-pressed='true'] > svg { color: var(--ac-violet-icon); }
@media (hover: hover) and (pointer: fine) {
  .ac-component:hover { box-shadow: 0 0 0 1px var(--ac-line-hover); }
}
</style>
