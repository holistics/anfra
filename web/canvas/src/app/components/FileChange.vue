<script setup lang="ts">
// One changed file in the Changes tab: its name and folder, how many lines came and went, and,
// opened, its changed lines with a few around them. A screen's file can be found on the board.
import { computed, ref } from 'vue';
import { bounds, type Frame } from '../../model/index.ts';
import type { FileDiff } from '../../vite/changes.ts';
import { useStore } from '../context.ts';
import Button from '../ui/Button.vue';
import Icon from '../ui/Icon.vue';

const props = defineProps<{ file: FileDiff }>();
const store = useStore();
const { state, board } = store;
const open = ref(false);
const all = ref(false);

/** Lines shown before "Show all": a long diff is a click away, not a scroll. */
const LIMIT = 200;
const STATUS: Partial<Record<FileDiff['status'], string>> = { added: 'New', deleted: 'Deleted', renamed: 'Renamed' };

const name = computed(() => props.file.path.split('/').pop() ?? props.file.path);
const dir = computed(() => props.file.path.split('/').slice(0, -1).join('/'));
const total = computed(() => props.file.hunks.reduce((n, h) => n + h.lines.length, 0));

/** Each part of the file that changed, its lines moved left by the indent they all share. */
const hunks = computed(() => {
  let budget = all.value ? Infinity : LIMIT;
  return props.file.hunks.flatMap((h) => {
    if (budget <= 0) return [];
    const lines = h.lines.slice(0, budget);
    budget -= lines.length;
    const indent = Math.min(...lines.filter((l) => l.text.trim()).map((l) => /^\s*/.exec(l.text)![0].length));
    const cut = Number.isFinite(indent) ? indent : 0;
    const start = h.lines.find((l) => l.kind !== ' ')?.new ?? h.lines[0]?.new ?? h.lines[0]?.old ?? 1;
    return [{ start, lines: lines.map((l) => ({ ...l, text: l.text.slice(cut) })) }];
  });
});

/** The first line the change touched, to open the file at. */
const first = computed(() => props.file.hunks.flatMap((h) => h.lines).find((l) => l.kind === '+')?.new ?? hunks.value[0]?.start ?? 1);

/** Show the screens of this board that draw this file. */
function find () {
  const ids = store.framesShowing(props.file.path);
  const frames = board.value?.frames.filter((f): f is Frame => ids.includes(f.id)) ?? [];
  if (!frames.length) return store.flash('No screen on this board shows this file');
  state.selection = frames.map((f) => f.id);
  store.zoomTo(bounds(frames), 1, true);
}
</script>

<template>
  <div class="ac-file">
    <button class="ac-file-head" type="button" :aria-expanded="open" @click="open = !open">
      <Icon name="chevronRight" :size="14" />
      <span class="ac-file-text">
        <span class="ac-file-name">
          <span class="ac-truncate">{{ name }}</span>
          <span v-if="STATUS[file.status]" class="ac-chip">{{ STATUS[file.status] }}</span>
        </span>
        <span v-if="dir" class="ac-file-dir ac-caption ac-truncate">{{ file.from ? `from ${file.from}` : dir }}</span>
      </span>
      <span class="ac-diffstat ac-num">
        <span v-if="file.added" class="added">+{{ file.added }}</span>
        <span v-if="file.removed" class="removed">−{{ file.removed }}</span>
      </span>
    </button>
    <div v-if="open" class="ac-diff">
      <div class="ac-row tight">
        <Button v-if="file.status !== 'deleted'" variant="ghost" small icon="external" @click="store.openInEditor(`${file.path}:${first}`)">Open in editor</Button>
        <Button v-if="file.path.endsWith('.vue') && file.status !== 'deleted'" variant="ghost" small icon="frame" @click="find">Find on the board</Button>
      </div>
      <p v-if="file.binary" class="ac-caption">A binary or very large file: no lines to show</p>
      <template v-for="(h, i) in hunks" :key="i">
        <p class="ac-diff-at ac-caption">Line {{ h.start }}</p>
        <div class="ac-diff-lines ac-mono">
          <div v-for="(l, j) in h.lines" :key="j" class="ac-diff-line" :class="{ added: l.kind === '+', removed: l.kind === '-' }">
            <span class="ac-diff-num">{{ l.new ?? l.old }}</span>
            <span class="ac-diff-sign" aria-hidden="true">{{ l.kind === '+' ? '+' : l.kind === '-' ? '−' : '' }}</span>
            <span class="ac-diff-text">{{ l.text || ' ' }}</span>
          </div>
        </div>
      </template>
      <Button v-if="!all && total > LIMIT" variant="ghost" small @click="all = true">Show all {{ total }} lines</Button>
    </div>
  </div>
</template>
