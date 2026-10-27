<script setup lang="ts">
// The arrows between shapes: curved connectors that leave and enter square to a side, spread out
// where several share one, with their label at the middle. An arrow runs (its dashes flow from
// start to end) when it is the way on: selected, leaving the selected frame, or a next step while
// a flow is played. The arrows already walked in a flow stay violet; the rest fade back.
import { computed } from 'vue';
import { find, isShape, type Arrow, type Point, type Rect, type Shape } from '../../model/index.ts';
import { useStore } from '../context.ts';
import { curve, edgePoint, center, port, sides, type Side } from '../geometry.ts';

// Drawn twice: the lines under the frames, so a connector passing behind a screen doesn't cross
// it, and the labels over everything, so they can always be read and clicked.
const props = withDefaults(defineProps<{ part: 'lines' | 'labels', draft?: { from: Shape, to: Point } | null }>(), { draft: null });
const store = useStore();
const { state, board } = store;

/** A frame's box includes its label above it, so an arrow meets the label rather than crossing it. */
const box = (s: Shape): Rect => (s.kind === 'frame' ? { ...s, y: s.y - 32 / state.view.zoom, height: s.height + 32 / state.view.zoom } : s);

type Role = 'walked' | 'next' | 'hover' | 'run' | 'rest' | 'faded';

const arrows = computed(() => {
  const b = board.value;
  if (!b) return [];
  const ends = b.arrows.flatMap((a) => {
    const from = find(b, a.from);
    const to = find(b, a.to);
    if (!isShape(from) || !isShape(to)) return [];
    const r1 = box(from);
    const r2 = box(to);
    const [s1, s2] = sides(r1, r2);
    return [{ a, r1, r2, s1, s2 }];
  });

  // Arrows sharing a side of a shape spread along it, ordered by where their other end is, so they
  // don't leave from one point and cross each other.
  const groups = new Map<string, { i: number, key: number }[]>();
  const add = (shape: string, side: Side, i: number, other: Rect) => {
    const k = `${shape}|${side}`;
    const c = center(other);
    const list = groups.get(k) ?? [];
    list.push({ i, key: side === 'left' || side === 'right' ? c.y : c.x });
    groups.set(k, list);
  };
  ends.forEach((e, i) => {
    add(e.a.from, e.s1, i * 2, e.r2);
    add(e.a.to, e.s2, i * 2 + 1, e.r1);
  });
  const along = new Map<number, number>();
  for (const list of groups.values()) {
    list.sort((p, q) => p.key - q.key);
    list.forEach((x, n) => along.set(x.i, list.length === 1 ? 0.5 : 0.3 + (0.4 * n) / (list.length - 1)));
  }

  const flow = state.flow;
  const here = flow?.path.at(-1);
  const selectedFrame = state.selection.length === 1 ? state.selection[0] : '';
  const role = (a: Arrow): Role => {
    if (flow) {
      if (flow.hover === a.id) return 'hover';
      if (flow.via.includes(a.id)) return 'walked';
      if (a.from === here) return 'next';
      return 'faded';
    }
    if (state.selection.includes(a.id) || a.from === selectedFrame) return 'run';
    return 'rest';
  };

  return ends.map((e, i) => {
    const p1 = port(e.r1, e.s1, along.get(i * 2));
    const p2 = port(e.r2, e.s2, along.get(i * 2 + 1));
    return { ...e.a, ...curve(p1, e.s1, p2, e.s2), role: role(e.a), selected: state.selection.includes(e.a.id) };
  });
});

const draftLine = computed(() => {
  if (!props.draft) return null;
  return { p1: edgePoint(box(props.draft.from), props.draft.to), p2: props.draft.to };
});
</script>

<template>
  <svg v-if="part === 'lines'" class="ac-arrows" width="1" height="1" overflow="visible" aria-hidden="true">
    <defs>
      <marker id="ac-head" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="9" markerHeight="9" markerUnits="userSpaceOnUse" orient="auto-start-reverse">
        <path d="M1 1.5 L8.5 5 L1 8.5" fill="none" stroke="context-stroke" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
      </marker>
    </defs>
    <g v-for="a in arrows" :key="a.id" class="ac-arrow" :class="[a.role, { selected: a.selected }]">
      <path :d="a.d" class="hit" data-hit="select" :data-id="a.id" />
      <path :d="a.d" class="stroke" marker-end="url(#ac-head)" />
      <path v-if="a.role === 'run' || a.role === 'next' || a.role === 'hover'" :d="a.d" class="flowing" />
    </g>
    <line v-if="draftLine" :x1="draftLine.p1.x" :y1="draftLine.p1.y" :x2="draftLine.p2.x" :y2="draftLine.p2.y" class="ac-arrow-draft" marker-end="url(#ac-head)" />
  </svg>
  <div v-else class="ac-arrow-labels">
    <template v-for="a in arrows" :key="a.id">
      <div
        v-if="a.label"
        class="ac-arrow ac-arrow-label"
        :class="[a.role, { selected: a.selected }]"
        :style="{ left: `${a.mid.x}px`, top: `${a.mid.y}px` }"
        data-hit="select"
        :data-id="a.id"
        @click="a.role === 'next' || a.role === 'hover' ? store.playNext(a.id) : undefined"
      >
        {{ a.label }}
      </div>
    </template>
  </div>
</template>

<style>
.ac-arrows { position: absolute; left: 0; top: 0; overflow: visible; pointer-events: none; }
.ac-arrow { transition: opacity 220ms var(--ac-ease); }
.ac-arrow .stroke, .ac-arrow-draft {
  fill: none;
  stroke: var(--ac-icon-weak);
  stroke-width: calc(1.5px * var(--ac-inv));
  stroke-linecap: round;
  transition: stroke 160ms var(--ac-ease);
}
.ac-arrow .hit { fill: none; stroke: transparent; stroke-width: calc(16px * var(--ac-inv)); pointer-events: stroke; cursor: pointer; }
.ac-arrow-draft { stroke: var(--ac-violet); stroke-dasharray: calc(6px * var(--ac-inv)) calc(5px * var(--ac-inv)); }

/* The way on: violet, with dashes running from start to end over a quiet base line */
.ac-arrow.run .stroke, .ac-arrow.next .stroke, .ac-arrow.hover .stroke, .ac-arrow.walked .stroke, .ac-arrow.selected .stroke {
  stroke: var(--ac-violet);
}
.ac-arrow.run .stroke, .ac-arrow.next .stroke, .ac-arrow.hover .stroke {
  stroke: color-mix(in oklch, var(--ac-violet) 35%, transparent);
  stroke-width: calc(2px * var(--ac-inv));
}
.ac-arrow.walked .stroke { stroke-width: calc(2px * var(--ac-inv)); }
.ac-arrow .flowing {
  fill: none;
  stroke: var(--ac-violet);
  stroke-width: calc(2px * var(--ac-inv));
  stroke-linecap: round;
  stroke-dasharray: calc(8px * var(--ac-inv)) calc(10px * var(--ac-inv));
  animation: ac-flow 700ms linear infinite;
}
.ac-arrow.hover .flowing { stroke-width: calc(2.5px * var(--ac-inv)); animation-duration: 450ms; }
@keyframes ac-flow {
  to { stroke-dashoffset: calc(-18px * var(--ac-inv)); }
}
.ac-arrow.faded { opacity: 0.25; }

.ac-arrow-labels { position: absolute; left: 0; top: 0; pointer-events: none; }
.ac-arrow-label {
  position: absolute;
  transform: translate(-50%, -50%) scale(var(--ac-inv));
  width: max-content;
  max-width: 240px;
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--ac-surface);
  box-shadow: 0 0 0 1px var(--ac-line);
  color: var(--ac-ink-weak);
  font-size: 12px;
  line-height: 16px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: auto;
  cursor: pointer;
  transition: color 160ms var(--ac-ease), box-shadow 160ms var(--ac-ease), background-color 160ms var(--ac-ease);
}
.ac-arrow-label.run, .ac-arrow-label.next, .ac-arrow-label.walked, .ac-arrow-label.selected {
  color: var(--ac-violet-text);
  box-shadow: 0 0 0 1px var(--ac-violet-line);
}
.ac-arrow-label.hover { background: var(--ac-violet); color: var(--ac-on-violet); box-shadow: none; }

@media (prefers-reduced-motion: reduce) {
  .ac-arrow .flowing { animation: none; }
}
</style>
