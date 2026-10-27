<script setup lang="ts">
// The arrows between shapes: right-angled connectors with rounded corners, routed around the
// screens (route.ts), spread out where several share a side, each label on a straight run where
// it collides with no screen and no other label. An arrow runs (its dashes flow from start to end)
// when it is the way on: selected, leaving the selected frame, or a next step while a flow is
// played. The arrows already walked in a flow stay violet; the rest fade back.
import { computed } from 'vue';
import type { Arrow, Point, Shape } from '../../model/index.ts';
import { useConnectors } from '../connectors.ts';
import { useStore } from '../context.ts';
import { edgePoint } from '../geometry.ts';

// Drawn twice: the lines under the frames, so a connector passing near a screen never covers it,
// and the labels over everything, so they can always be read and clicked. Both draw the same routes.
const props = withDefaults(defineProps<{ part: 'lines' | 'labels', draft?: { from: Shape, to: Point } | null }>(), { draft: null });
const store = useStore();
const { state } = store;
const { list: routes, box } = useConnectors(store);

type Role = 'walked' | 'next' | 'hover' | 'run' | 'rest' | 'faded';

const arrows = computed(() => {
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
  return routes.value.map((r) => ({ ...r.arrow, d: r.d, mid: r.mid, role: role(r.arrow), selected: state.selection.includes(r.arrow.id) }));
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
