<script setup lang="ts">
import { RefreshCw } from '@lucide/vue';
import type { Command, CommandState } from '@holistics/anfra-sdk/host';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { commandIcon, type CommandMenu } from '@/lib/pluginUI';
import type * as server from '@/lib/server';
import { IS_MAC, keyLabels } from '@/lib/shortcut';
import LoadingBar from './LoadingBar.vue';
import RepoProblems from './RepoProblems.vue';

defineProps<{
  hasApp: boolean;
  title: string;
  crumbs: { path: string, name: string }[];
  problems: server.AmlProblem[];
  loading: boolean;
  commands: readonly Command[];
  menus: readonly CommandMenu[];
  commandState: (command: Command) => CommandState;
}>();

const emit = defineEmits<{
  revealFolder: [path: string];
  reload: [];
  run: [command: Command];
}>();
</script>

<template>
  <header class="relative flex h-12 shrink-0 items-center gap-2 border-b border-default px-3">
    <!-- The sidebar's toggle, with its peek (SidebarPeek). Then the open Data App's name only, as Cloud's header has it. -->
    <slot name="toggle" />
    <h2 class="max-w-1/2 shrink-0 truncate text-label text-default" data-testid="viewer-title">{{ title }}</h2>
    <nav v-if="crumbs.length" class="flex min-w-0 items-center overflow-hidden text-caption whitespace-nowrap text-weak" aria-label="Folder">
      <template v-for="(crumb, i) in crumbs" :key="crumb.path">
        <span v-if="i" class="px-1 text-placeholder" aria-hidden="true">/</span>
        <button
          type="button"
          class="hover:text-default hover:underline"
          data-testid="crumb"
          :data-crumb="crumb.path"
          :title="`Show ${crumb.name} in the sidebar`"
          @click="emit('revealFolder', crumb.path)"
        >
          {{ crumb.name }}
        </button>
      </template>
    </nav>
    <div class="ml-auto flex items-center gap-1">
      <RepoProblems v-if="problems.length" :problems="problems" />
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="Reload Data App"
        title="Reload Data App"
        data-testid="reload"
        :disabled="!hasApp"
        @click="emit('reload')"
      >
        <RefreshCw />
      </Button>
      <Button
        v-for="command in commands"
        :key="command.id"
        variant="ghost"
        size="icon-sm"
        :aria-label="command.title"
        :title="commandState(command).disabled ?? (command.shortcut ? `${command.title} (${keyLabels(command.shortcut).join(IS_MAC ? '' : '+')})` : command.title)"
        :data-testid="`command-${command.id}`"
        :aria-pressed="commandState(command).pressed ?? false"
        :disabled="!hasApp || !!commandState(command).disabled"
        @click="emit('run', command)"
      >
        <component :is="commandIcon(command)" />
      </Button>
      <DropdownMenu v-for="menu in menus" :key="menu.key">
        <DropdownMenuTrigger as-child>
          <Button
            variant="ghost"
            size="icon-sm"
            :aria-label="menu.label"
            :title="menu.label"
            :data-testid="`more-${menu.key}`"
            :disabled="!hasApp"
          >
            <component :is="menu.icon" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" :data-testid="`more-${menu.key}-menu`">
          <DropdownMenuItem
            v-for="command in menu.commands"
            :key="command.id"
            :title="commandState(command).disabled"
            :disabled="!!commandState(command).disabled"
            :data-testid="`command-${command.id}`"
            @select="emit('run', command)"
          >
            {{ command.title }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    <LoadingBar v-if="loading" aria-label="Loading Data App" data-testid="loading" />
  </header>
</template>
