<script setup lang="ts">
// One level of the structure: a row per node, its children indented below. Hovering a row asks the
// page to highlight the node; clicking selects it.
import type { StructureNode } from './inspect';

defineProps<{ nodes: StructureNode[], selected?: string, depth?: number }>();
const emit = defineEmits<{ hover: [key: string | null], select: [key: string] }>();

const title = (node: StructureNode) => (node.problems.length ? node.problems.join('\n') : undefined);
</script>

<template>
  <ul class="tree" role="tree">
    <li v-for="node in nodes" :key="node.key" role="treeitem" :aria-selected="node.key === selected">
      <div
        class="node"
        :class="{ selected: node.key === selected, problem: node.problems.length > 0 }"
        :style="{ paddingLeft: `${8 + (depth ?? 0) * 14}px` }"
        :title="title(node)"
        :data-testid="`node-${node.key}`"
        @mouseenter="emit('hover', node.key)"
        @click.stop="emit('select', node.key)"
      >
        <span class="kind" :class="node.kind">{{ node.kind }}</span>
        <span class="name">{{ node.kind === 'container' || node.kind === 'block' ? node.id : node.name }}</span>
        <span v-if="node.app" class="app">app {{ node.app }}</span>
        <span v-if="node.label" class="label">{{ node.label }}</span>
        <span v-if="node.problems.length" class="pill error" aria-label="Problems">!</span>
      </div>
      <StructureTree
        v-if="node.children.length"
        :nodes="node.children"
        :selected="selected"
        :depth="(depth ?? 0) + 1"
        @hover="emit('hover', $event)"
        @select="emit('select', $event)"
      />
    </li>
  </ul>
</template>
