<script setup lang="ts">
// A frame's layers: the components and elements written in the app that its screen shows, nested
// as drawn. Hovering one outlines it on the screen; clicking picks it, as clicking the screen does.
// Picking on the screen opens the tree down to that layer.
import { computed, nextTick, ref, watch } from 'vue';
import type { ElementRef } from '../../model/index.ts';
import type { LayerNode } from '../../frame/protocol.ts';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';

const props = defineProps<{ frame: string }>();
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
    <li v-if="!rows.length" class="ac-caption ac-tree-empty">{{ state.attached[frame] ? 'Nothing written in the app on this screen' : 'Loading the screen' }}</li>
    <li v-for="r in rows" :key="r.key">
      <div
        class="ac-tree-row"
        :data-layer="r.key"
        :aria-current="r.key === pickedKey || undefined"
        :style="{ paddingLeft: `${r.depth * 12 + 4}px` }"
        @pointerenter="hover(r.node)"
        @pointerleave="hover(null)"
      >
        <button
          v-if="r.node.children.length"
          class="ac-tree-toggle"
          type="button"
          :aria-label="r.open ? 'Close' : 'Open'"
          :aria-expanded="r.open"
          @click="state.layersOpen[r.key] = !r.open"
        >
          <Icon name="chevronRight" :size="14" />
        </button>
        <span v-else class="ac-tree-toggle" />
        <button class="ac-tree-name" type="button" @click="store.pick(frame, r.node.ref)">
          <Icon :name="r.node.component ? 'component' : r.node.text && !r.node.children.length ? 'text' : 'box'" :size="14" />
          <span class="ac-truncate" :class="{ component: r.node.component }">{{ r.node.label }}</span>
          <span v-if="r.node.text" class="ac-tree-text ac-truncate">{{ r.node.text }}</span>
        </button>
      </div>
    </li>
  </ul>
</template>

<style>
.ac-tree { list-style: none; margin: 0; padding: 0 0 4px 20px; }
.ac-tree-empty { padding: 4px 12px 8px; }
.ac-tree-row {
  display: flex;
  align-items: center;
  height: 28px;
  min-width: 0;
  border-radius: 6px;
  color: var(--ac-ink-weak);
  transition: background-color 160ms var(--ac-ease);
}
.ac-tree-row[aria-current] { background: var(--ac-selected); color: var(--ac-violet-text); }
.ac-tree-toggle {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 20px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: none;
  color: var(--ac-icon-weak);
  cursor: pointer;
}
.ac-tree-toggle > svg { transition: rotate 160ms var(--ac-ease); }
.ac-tree-toggle[aria-expanded='true'] > svg { rotate: 90deg; }
.ac-tree-name {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  height: 100%;
  padding: 0 6px 0 2px;
  border: 0;
  background: none;
  text-align: left;
  font-size: 12px;
  line-height: 16px;
  cursor: pointer;
}
.ac-tree-name > svg { flex-shrink: 0; color: var(--ac-icon-weak); }
.ac-tree-row[aria-current] .ac-tree-name > svg { color: var(--ac-violet-icon); }
.ac-tree-name .component { color: var(--ac-ink); font-weight: 500; }
.ac-tree-row[aria-current] .component { color: var(--ac-violet-text); }
.ac-tree-text { flex: 0 100 auto; color: var(--ac-ink-muted); }
@media (hover: hover) and (pointer: fine) {
  .ac-tree-row:hover { background: var(--ac-hover); }
  .ac-tree-row[aria-current]:hover { background: var(--ac-selected); }
  .ac-tree-toggle:hover { background: var(--ac-active); }
}
</style>
