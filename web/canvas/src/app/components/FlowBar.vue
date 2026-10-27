<script setup lang="ts">
// Playing a board as a prototype. The frame in view is live; the arrows leaving it are the ways on,
// named by their labels. Taking one runs its arrow and glides to the next frame; Back retraces.
// The path so far sits above, so a happy path and each edge case can be walked one after another.
import { computed } from 'vue';
import type { Frame } from '../../model/index.ts';
import { useStore } from '../context.ts';
import Button from '../ui/Button.vue';
import Icon from '../ui/Icon.vue';

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
      <Button variant="ghost" icon="back" label="Back one step" keys="←" :disabled="flow.path.length < 2" @click="store.playBack()" />
      <div class="ac-flowbar-next">
        <template v-if="next.length">
          <Button
            v-for="o in next"
            :key="o.id"
            class="ac-flowbar-way"
            :tip="`To ${o.to}`"
            :keys="String(o.n)"
            @click="store.playNext(o.id)"
            @pointerenter="hover(o.id)"
            @pointerleave="hover('')"
            @focus="hover(o.id)"
            @blur="hover('')"
          >
            <span class="ac-kbd">{{ o.n }}</span>
            <span class="ac-truncate">{{ o.label }}</span>
            <Icon name="arrowRight" />
          </Button>
        </template>
        <template v-else>
          <span class="ac-caption">End of this path</span>
          <Button icon="reload" @click="store.play(flow.path[0])">Play again</Button>
        </template>
      </div>
      <Button variant="ghost" tip="Stop playing" keys="Esc" @click="store.stopPlay()">Stop</Button>
    </div>
  </div>
</template>
