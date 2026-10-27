<script setup lang="ts">
// A frame's layers: the components and elements written in the app that its screen shows, nested
// as drawn. Hovering one outlines it on the screen; clicking picks it, as clicking the screen does.
// Picking on the screen opens the tree down to that layer.
import { computed, nextTick, ref, watch } from 'vue';
import type { ElementRef } from '../../model/index.ts';
import type { LayerNode } from '../../frame/protocol.ts';
import { useStore } from '../context.ts';
import TreeRow from '../ui/TreeRow.vue';

/** `depth`: how deep its frame's row is in the tree around it. */
const props = withDefaults(defineProps<{ frame: string, depth?: number }>(), { depth: 0 });
const store = useStore();
const { state } = store;

interface Row { node: LayerNode, depth: number, key: string, open: boolean }

const keyOf = (r: ElementRef) => `${props.frame}|${r.source ?? r.selector}|${r.index ?? 0}`;
const nodes = ref<LayerNode[]>([]);

// The screen changes as it loads and as files are edited: read the tree again, at most every 300ms.
let timer: ReturnType<typeof setTimeout> | undefined;
const refresh = () => {
  clearTimeout(timer);
  timer = setTimeout(() => (nodes.value = store.tree(props.frame)), 300);
};
watch(() => [state.layouts[props.frame], state.attached[props.frame]], refresh, { immediate: true });

const rows = computed<Row[]>(() => {
  const out: Row[] = [];
  const add = (list: LayerNode[], depth: number) => {
    for (const node of list) {
      const key = keyOf(node.ref);
      const open = !!state.layersOpen[key];
      out.push({ node, depth, key, open });
      if (open) add(node.children, depth + 1);
    }
  };
  add(nodes.value, 0);
  return out;
});

const pickedKey = computed(() => (state.picked?.frame === props.frame ? keyOf(state.picked.info.ref) : ''));

/** Open the tree down to the picked layer, and bring it into view. */
watch([pickedKey, nodes], async ([key]) => {
  if (!key) return;
  const path: string[] = [];
  const find = (list: LayerNode[]): boolean => list.some((n) => {
    const k = keyOf(n.ref);
    if (k === key) return true;
    path.push(k);
    if (find(n.children)) return true;
    path.pop();
    return false;
  });
  if (!find(nodes.value)) return;
  for (const k of path) state.layersOpen[k] = true;
  await nextTick();
  document.querySelector(`[data-layer="${CSS.escape(key)}"]`)?.scrollIntoView({ block: 'nearest' });
});

function hover (n: LayerNode | null) {
  const info = n && store.locate(n.ref);
  state.hover = info ? { frame: props.frame, info } : null;
}
</script>

<template>
  <ul class="ac-tree" role="group">
    <li v-if="!rows.length" class="ac-caption ac-tree-empty" :style="{ paddingLeft: `${(depth + 1) * 12 + 28}px` }">{{ state.attached[frame] ? 'Nothing written in the app on this screen' : 'Loading the screen' }}</li>
    <li v-for="r in rows" :key="r.key">
      <TreeRow
        :label="r.node.label"
        :icon="r.node.component ? 'component' : r.node.text && !r.node.children.length ? 'text' : 'box'"
        :depth="depth + 1 + r.depth"
        :meta="r.node.text"
        :strong="!!r.node.component"
        :current="r.key === pickedKey"
        :expandable="r.node.children.length > 0"
        :open="r.open"
        :data-layer="r.key"
        @toggle="state.layersOpen[r.key] = !r.open"
        @pick="store.pick(frame, r.node.ref)"
        @pointerenter="hover(r.node)"
        @pointerleave="hover(null)"
      />
    </li>
  </ul>
</template>
