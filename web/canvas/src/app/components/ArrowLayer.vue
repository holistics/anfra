<script setup lang="ts">
import { computed } from 'vue';
import { find, isShape, type Point, type Shape } from '../../model/index.ts';
import { useStore } from '../context.ts';
import { center, edgePoint } from '../geometry.ts';

const props = defineProps<{ draft: { from: Shape, to: Point } | null }>();
const store = useStore();
const { state, board } = store;

/** A frame's box includes its label above it, so an arrow meets the label rather than crossing it. */
const box = (s: Shape) => (s.kind === 'frame' ? { ...s, y: s.y - 28 / state.view.zoom, height: s.height + 28 / state.view.zoom } : s);

const arrows = computed(() => (board.value?.arrows ?? []).flatMap((a) => {
  const from = find(board.value!, a.from);
  const to = find(board.value!, a.to);
  if (!isShape(from) || !isShape(to)) return [];
  const p1 = edgePoint(box(from), center(box(to)));
  const p2 = edgePoint(box(to), center(box(from)));
  return [{ ...a, p1, p2, mid: { x: (p1.x + p2.x) / 2, y: (p1.y + p2.y) / 2 }, selected: state.selection.includes(a.id) }];
}));

const draftLine = computed(() => {
  if (!props.draft) return null;
  return { p1: edgePoint(box(props.draft.from), props.draft.to), p2: props.draft.to };
});
</script>

<template>
  <svg class="ac-arrows" width="1" height="1" overflow="visible">
    <defs>
      <marker id="ac-head" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto-start-reverse">
        <path d="M0 0 L10 5 L0 10 z" fill="context-stroke" />
      </marker>
    </defs>
    <g v-for="a in arrows" :key="a.id" class="ac-arrow" :class="{ selected: a.selected }">
      <line :x1="a.p1.x" :y1="a.p1.y" :x2="a.p2.x" :y2="a.p2.y" class="hit" data-hit="select" :data-id="a.id" />
      <line :x1="a.p1.x" :y1="a.p1.y" :x2="a.p2.x" :y2="a.p2.y" class="stroke" marker-end="url(#ac-head)" />
      <foreignObject v-if="a.label" :x="a.mid.x" :y="a.mid.y" width="1" height="1" overflow="visible">
        <div class="ac-arrow-label" data-hit="select" :data-id="a.id">{{ a.label }}</div>
      </foreignObject>
    </g>
    <line v-if="draftLine" :x1="draftLine.p1.x" :y1="draftLine.p1.y" :x2="draftLine.p2.x" :y2="draftLine.p2.y" class="ac-arrow-draft" marker-end="url(#ac-head)" />
  </svg>
</template>

<style>
.ac-arrows { position: absolute; left: 0; top: 0; overflow: visible; pointer-events: none; }
.ac-arrow .stroke, .ac-arrow-draft {
  stroke: var(--ac-ink-weak);
  stroke-width: calc(1.5px * var(--ac-inv));
  fill: none;
}
.ac-arrow-draft { stroke: var(--ac-violet); stroke-dasharray: calc(6px * var(--ac-inv)); }
.ac-arrow .hit { stroke: transparent; stroke-width: calc(14px * var(--ac-inv)); pointer-events: stroke; cursor: pointer; }
.ac-arrow.selected .stroke { stroke: var(--ac-violet); stroke-width: calc(2px * var(--ac-inv)); }
.ac-arrow-label {
  position: absolute;
  transform: translate(-50%, -50%) scale(var(--ac-inv));
  width: max-content;
  max-width: 240px;
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--ac-surface);
  box-shadow: 0 0 0 1px var(--ac-border-strong);
  color: var(--ac-ink-weak);
  font-size: 12px;
  line-height: 16px;
  pointer-events: auto;
  cursor: pointer;
}
.ac-arrow.selected .ac-arrow-label { color: var(--ac-violet-text); box-shadow: 0 0 0 1px var(--ac-violet); }
</style>
