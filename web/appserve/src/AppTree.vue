<script setup lang="ts">
import type { CatalogEntry } from './catalog';
import Icon from './Icon.vue';

const props = defineProps<{
  entries: CatalogEntry[];
  selected?: string;
  /** Folders the reader collapsed; ignored while `forceOpen` (a search is active). */
  collapsed: Set<string>;
  forceOpen: boolean;
  /** How deep this list is: the top is the sidebar's menu, below it each folder's sub-menu. */
  depth?: number;
}>();

const emit = defineEmits<{ select: [path: string], toggle: [path: string] }>();

const open = (path: string) => props.forceOpen || !props.collapsed.has(path);
// As in anfra Cloud's sidebar: a menu button at the top, a sub-menu button in a folder.
const row = () => (props.depth ? 'a-tree-row' : 'a-nav-item');
</script>

<template>
  <ul class="tree" :class="{ 'a-tree': depth }" role="group">
    <li v-for="entry in entries" :key="entry.path">
      <template v-if="entry.kind === 'folder'">
        <button
          type="button"
          class="folder"
          :class="row()"
          :aria-expanded="open(entry.path)"
          :data-folder="entry.path"
          @click="emit('toggle', entry.path)"
        >
          <Icon :name="open(entry.path) ? 'folder-open' : 'folder'" />
          <span class="name">{{ entry.name }}</span>
          <Icon name="chevron" class="caret" :class="{ open: open(entry.path) }" />
        </button>
        <AppTree
          v-if="open(entry.path) && entry.children.length"
          :entries="entry.children"
          :selected="selected"
          :collapsed="collapsed"
          :force-open="forceOpen"
          :depth="(depth ?? 0) + 1"
          @select="emit('select', $event)"
          @toggle="emit('toggle', $event)"
        />
      </template>
      <button
        v-else
        type="button"
        class="app"
        :class="[row(), { selected: entry.path === selected }]"
        :aria-current="entry.path === selected ? 'page' : undefined"
        :data-app="entry.path"
        :title="entry.path"
        @click="emit('select', entry.path)"
      >
        <Icon name="app" /><span class="name">{{ entry.label }}</span>
      </button>
    </li>
  </ul>
</template>
