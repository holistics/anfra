<script setup lang="ts">
import { ChevronRight, Folder, FolderOpen, LayoutDashboard } from '@lucide/vue';
import type { CatalogEntry } from '@/lib/catalog';
import { cn } from '@/lib/utils';
import {
  sidebarMenu, sidebarMenuButton, sidebarMenuSub, sidebarMenuSubButton,
} from './sidebar-menu';

const props = defineProps<{
  entries: CatalogEntry[];
  selected?: string;
  /** Folders the reader collapsed. */
  collapsed: Set<string>;
  /** How deep this list is: the top is the sidebar's menu, below it each folder's sub-menu. */
  depth?: number;
}>();

const emit = defineEmits<{ select: [path: string], toggle: [path: string] }>();

const open = (path: string) => !props.collapsed.has(path);
const button = () => (props.depth ? sidebarMenuSubButton : sidebarMenuButton);
const slot = () => (props.depth ? 'sidebar-menu-sub-button' : 'sidebar-menu-button');
</script>

<template>
  <!-- A tree indents on the left only: no sub-menu keeps its own right inset. -->
  <ul :class="depth ? cn(sidebarMenuSub, 'mr-0 pr-0') : sidebarMenu" role="group">
    <li v-for="entry in entries" :key="entry.path" class="relative">
      <template v-if="entry.kind === 'folder'">
        <button
          type="button"
          :data-slot="slot()"
          :class="cn(button(), 'group/folder')"
          :aria-expanded="open(entry.path)"
          :data-folder="entry.path"
          @click="emit('toggle', entry.path)"
        >
          <FolderOpen v-if="open(entry.path)" />
          <Folder v-else />
          <span class="truncate">{{ entry.name }}</span>
          <ChevronRight class="ml-auto transition-transform duration-160 ease-anfra-out group-aria-expanded/folder:rotate-90" />
        </button>
        <AppTree
          v-if="open(entry.path) && entry.children.length"
          :entries="entry.children"
          :selected="selected"
          :collapsed="collapsed"
          :depth="(depth ?? 0) + 1"
          @select="emit('select', $event)"
          @toggle="emit('toggle', $event)"
        />
      </template>
      <button
        v-else
        type="button"
        :data-slot="slot()"
        :class="button()"
        :data-active="entry.path === selected || undefined"
        :aria-current="entry.path === selected ? 'page' : undefined"
        :data-app="entry.path"
        :title="entry.path"
        @click="emit('select', entry.path)"
      >
        <LayoutDashboard /><span>{{ entry.label }}</span>
      </button>
    </li>
  </ul>
</template>
