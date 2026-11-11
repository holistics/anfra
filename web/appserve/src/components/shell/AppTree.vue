<script setup lang="ts">
// The repo's Data Apps as their folders hold them: a folder opens and closes in place (its icon
// gives way to a chevron under the pointer or the focus), an app opens in the viewer, the current
// one marked. The top level is the sidebar's menu; each folder's contents are a sub-menu below it.
import { ChevronRight, Folder, FolderOpen } from '@lucide/vue';
import { AppIcon } from '@/lib/appIcon';
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
// Under the pointer or the focus, the folder's icon hides and the chevron shows in its place.
const folderIcon = 'size-4 icon-weak group-hover/folder:invisible group-focus-visible/folder:invisible';
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
          <span class="relative grid size-4 shrink-0 place-items-center">
            <FolderOpen v-if="open(entry.path)" :class="folderIcon" />
            <Folder v-else :class="folderIcon" />
            <ChevronRight
              class="invisible absolute size-4 icon-weak transition-transform duration-160 ease-anfra-out group-hover/folder:visible group-focus-visible/folder:visible group-aria-expanded/folder:rotate-90"
              data-chevron
            />
          </span>
          <span class="truncate">{{ entry.name }}</span>
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
        <AppIcon /><span>{{ entry.label }}</span>
      </button>
    </li>
  </ul>
</template>
