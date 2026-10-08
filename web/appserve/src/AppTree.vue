<script setup lang="ts">
import type { CatalogEntry } from './catalog';
import Icon from './Icon.vue';

defineProps<{
  entries: CatalogEntry[];
  selected?: string;
  /** Folders the reader collapsed; ignored while `forceOpen` (a search is active). */
  collapsed: Set<string>;
  forceOpen: boolean;
}>();

const emit = defineEmits<{ select: [path: string], toggle: [path: string] }>();
</script>

<template>
  <ul class="tree" role="group">
    <li v-for="entry in entries" :key="entry.path">
      <template v-if="entry.kind === 'folder'">
        <button
          type="button"
          class="folder"
          :aria-expanded="forceOpen || !collapsed.has(entry.path)"
          :data-folder="entry.path"
          @click="emit('toggle', entry.path)"
        >
          <Icon name="chevron" class="caret" :class="{ open: forceOpen || !collapsed.has(entry.path) }" />
          <Icon name="folder" class="kind" />
          <span class="name">{{ entry.name }}</span>
        </button>
        <AppTree
          v-if="forceOpen || !collapsed.has(entry.path)"
          :entries="entry.children"
          :selected="selected"
          :collapsed="collapsed"
          :force-open="forceOpen"
          @select="emit('select', $event)"
          @toggle="emit('toggle', $event)"
        />
      </template>
      <button
        v-else
        type="button"
        class="app"
        :class="{ selected: entry.path === selected }"
        :aria-current="entry.path === selected ? 'page' : undefined"
        :data-app="entry.path"
        :title="entry.path"
        @click="emit('select', entry.path)"
      >
        <Icon name="app" class="kind" /><span class="name">{{ entry.label }}</span>
      </button>
    </li>
  </ul>
</template>
