<script setup lang="ts">
// Beside the canvas: what is on the board, the comments on it, and the components to place.
import { computed, ref, watch } from 'vue';
import {
  commentMarkdown, contents, find, isShape, remove, reply, setStatus,
  type Comment, type Shape,
} from '../../model/index.ts';
import { pinNumber, useStore } from '../context.ts';
import Button from '../ui/Button.vue';
import Field from '../ui/Field.vue';
import Segmented, { type SegmentedOption } from '../ui/Segmented.vue';
import TreeRow from '../ui/TreeRow.vue';
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
const TABS = computed<SegmentedOption<typeof tab.value>[]>(() => [
  { value: 'comments', label: 'Comments', count: openCount.value },
  { value: 'layers', label: 'Layers' },
  { value: 'components', label: 'Components' },
]);
const FILTERS: SegmentedOption<typeof filter.value>[] = [{ value: 'open', label: 'Open' }, { value: 'resolved', label: 'Resolved' }];
const toggle = (id: string) => (state.layersOpen[id] = !state.layersOpen[id]);

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
      <Segmented v-model="tab" :options="TABS" label="Board panel" tabs fill />
    </div>

    <div v-if="tab === 'layers'" class="ac-list" role="tabpanel">
      <p v-if="!layers.length" class="ac-caption ac-list-empty">Nothing on this board yet</p>
      <ul class="ac-tree">
        <li v-for="g in layers" :key="g.shape.id">
          <TreeRow
            :label="label(g.shape)"
            :icon="g.shape.kind"
            :meta="g.shape.kind === 'frame' ? scenarioLabel(g.shape.scenario) : undefined"
            strong
            :current="state.selection.includes(g.shape.id)"
            :expandable="g.shape.kind === 'frame'"
            :open="!!state.layersOpen[g.shape.id]"
            @toggle="toggle(g.shape.id)"
            @pick="focusShape(g.shape)"
          />
          <LayerTree v-if="g.shape.kind === 'frame' && state.layersOpen[g.shape.id]" :frame="g.shape.id" />
          <ul v-if="g.items.length" class="ac-tree">
            <li v-for="i in g.items" :key="i.id">
              <TreeRow
                :label="label(i)"
                :icon="i.kind"
                :depth="1"
                :meta="i.kind === 'frame' ? scenarioLabel(i.scenario) : undefined"
                strong
                :current="state.selection.includes(i.id)"
                :expandable="i.kind === 'frame'"
                :open="!!state.layersOpen[i.id]"
                @toggle="toggle(i.id)"
                @pick="focusShape(i)"
              />
              <LayerTree v-if="i.kind === 'frame' && state.layersOpen[i.id]" :frame="i.id" :depth="1" />
            </li>
          </ul>
        </li>
      </ul>
    </div>

    <div v-else-if="tab === 'comments'" class="ac-list" role="tabpanel">
      <div class="ac-row ac-list-tools">
        <Segmented v-model="filter" :options="FILTERS" label="Show" />
        <span class="ac-spacer" />
        <Button variant="ghost" icon="copy" label="Copy these comments as Markdown" tip="Copy these comments as one Markdown task" :disabled="!comments.length" @click="store.copyFeedback(comments)" />
      </div>
      <div v-if="!comments.length" class="ac-list-empty">
        <p class="ac-heading">{{ filter === 'open' ? 'No open comments' : 'Nothing resolved yet' }}</p>
        <p class="ac-caption">{{ filter === 'open' ? 'Press C and click an element of a screen to leave one for Claude' : 'Comments Claude or you resolve land here, with the note on what changed' }}</p>
      </div>
      <article
        v-for="c in comments"
        :key="c.id"
        class="ac-card ac-comment"
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
          <Field v-model="drafts[c.id]" placeholder="Reply" aria-label="Reply" @keydown.enter="sendReply(c)" />
          <div class="ac-row tight">
            <Button variant="ghost" icon="copy" label="Copy this comment as Markdown" @click="store.copy(commentMarkdown(board!, c), 'Comment copied as Markdown')" />
            <Button variant="ghost" icon="trash" label="Delete this comment" @click="board && store.commit(remove(board, [c.id]))" />
            <span class="ac-spacer" />
            <Button v-if="c.status === 'open'" icon="resolve" @click="setCommentStatus(c, 'resolved')">Resolve</Button>
            <Button v-else icon="reopen" @click="setCommentStatus(c, 'open')">Reopen</Button>
          </div>
        </div>
      </article>
    </div>

    <div v-else class="ac-list" role="tabpanel">
      <Palette />
    </div>
  </aside>
</template>
