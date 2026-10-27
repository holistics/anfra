<script setup lang="ts">
// Playing a board as a prototype. The frame in view is live; the arrows leaving it are the ways on,
// named by their labels. Taking one runs its arrow and glides to the next frame; Back retraces.
// The path so far sits above, so a happy path and each edge case can be walked one after another.
import { computed } from 'vue';
import type { Frame } from '../../model/index.ts';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';

const store = useStore();
const { state, board } = store;

const frame = (id: string): Frame | undefined => board.value?.frames.find((f) => f.id === id);
const flow = computed(() => state.flow!);
const here = computed(() => frame(flow.value.path.at(-1)!));

const next = computed(() => (board.value?.arrows ?? [])
  .filter((a) => a.from === here.value?.id && frame(a.to))
  .map((a, i) => ({ id: a.id, n: i + 1, label: a.label || frame(a.to)!.name, to: frame(a.to)!.name })));

/** The path, with the middle folded away once it is long. */
const crumbs = computed(() => {
  const all = flow.value.path.map((id, i) => ({ i, name: frame(id)?.name ?? 'A removed frame' }));
  return all.length > 4 ? [all[0], { i: -1, name: '…' }, ...all.slice(-2)] : all;
});

const hover = (id: string) => {
  if (state.flow) state.flow.hover = id;
};
</script>

<template>
  <div class="ac-flowbar ac-popover ac-pop-in" data-ui role="region" aria-label="Playing the flow">
    <div class="ac-flowbar-path">
      <Icon name="play" :size="14" class="ac-flowbar-icon" />
      <ol class="ac-crumbs">
        <li v-for="c in crumbs" :key="c.i" :aria-current="c.i === flow.path.length - 1 ? 'step' : undefined">
          <span class="ac-truncate">{{ c.name }}</span>
        </li>
      </ol>
      <span class="ac-spacer" />
      <span class="ac-caption ac-num">Step {{ flow.path.length }}</span>
    </div>
    <div class="ac-flowbar-actions">
      <button class="ac-btn ghost icon" type="button" title="Back (←)" aria-label="Back one step" :disabled="flow.path.length < 2" @click="store.playBack()">
        <Icon name="back" />
      </button>
      <div class="ac-flowbar-next">
        <template v-if="next.length">
          <button
            v-for="o in next"
            :key="o.id"
            class="ac-btn"
            type="button"
            :title="`${o.label}: to ${o.to} (${o.n})`"
            @click="store.playNext(o.id)"
            @pointerenter="hover(o.id)"
            @pointerleave="hover('')"
            @focus="hover(o.id)"
            @blur="hover('')"
          >
            <span class="ac-kbd">{{ o.n }}</span>
            <span class="ac-truncate">{{ o.label }}</span>
            <Icon name="arrowRight" />
          </button>
        </template>
        <template v-else>
          <span class="ac-caption">End of this path</span>
          <button class="ac-btn" type="button" @click="store.play(flow.path[0])"><Icon name="reload" /> Play again</button>
        </template>
      </div>
      <button class="ac-btn ghost" type="button" title="Stop (Esc)" @click="store.stopPlay()">Stop</button>
    </div>
  </div>
</template>

<style>
.ac-flowbar {
  position: absolute;
  left: 50%;
  bottom: 16px;
  translate: -50% 0;
  display: grid;
  gap: 4px;
  width: min(720px, calc(100% - 32px));
  padding: 8px;
  transform-origin: bottom;
}
.ac-flowbar-path { display: flex; align-items: center; gap: 8px; min-width: 0; padding: 0 4px; height: 24px; }
.ac-flowbar-icon { flex-shrink: 0; color: var(--ac-violet-icon); }
.ac-crumbs { display: flex; align-items: center; min-width: 0; margin: 0; padding: 0; list-style: none; color: var(--ac-ink-muted); font-size: 12px; line-height: 16px; }
.ac-crumbs li { display: flex; align-items: center; min-width: 0; max-width: 200px; }
.ac-crumbs li + li::before { content: '›'; padding: 0 6px; color: var(--ac-icon-weak); }
.ac-crumbs li[aria-current] { color: var(--ac-ink); font-weight: 500; }
.ac-flowbar-actions { display: flex; align-items: flex-start; gap: 8px; min-width: 0; }
.ac-flowbar-next { flex: 1; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
.ac-flowbar-next > .ac-btn { min-width: 0; max-width: 100%; padding-left: 6px; }
.ac-flowbar-next > .ac-caption { padding-left: 4px; }
</style>
