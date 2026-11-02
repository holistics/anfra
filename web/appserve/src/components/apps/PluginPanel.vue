<script setup lang="ts">
// The panel's chrome: the panel's commands (Pick) and the plugins' tabs as a tab strip, and the
// open tab drawn by its plugin. What each tab shows, and the panel's state (the tab, the node
// selected, pick mode), are the SDK's; this only places them. Tabs that don't fit go behind a More
// tabs button, as in a browser's devtools, the selected one always staying in the strip.
import { X } from '@lucide/vue';
import { useResizeObserver } from '@vueuse/core';
import {
  computed, nextTick, onBeforeUnmount, ref, watch,
} from 'vue';
import type { Command, CommandState, Panel, Tab } from '@holistics/anfra-sdk/host';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';
import { moreTabsIcon, panelCommandIcon, tabIcon } from '@/lib/pluginUI';
import { fitTabs, type TabFit } from '@/lib/tabFit';

const props = defineProps<{
  panel: Panel,
  tabs: readonly Tab[],
  // The commands placed in the strip, before the tabs, as icon buttons.
  commands: readonly Command[],
  commandState: (command: Command) => CommandState,
  // Bumped on each change to the panel, so the strip follows it.
  version: number,
}>();
const emit = defineEmits<{ run: [command: Command] }>();

const content = ref<HTMLElement>();
let unmountTab: (() => void) | undefined;
let shown: { panel: Panel, tab: string | undefined } | undefined;

// The open tab, drawn into its own element: a tab draws in that element's shadow root, so a new
// element per tab keeps one tab's root from being reused by another. Drawn again for another
// mount's panel, even at the same tab.
function draw (): void {
  const { panel } = props;
  const tab = panel.tab;
  if ((shown?.panel === panel && shown.tab === tab) || !content.value) return;
  unmountTab?.();
  unmountTab = undefined;
  content.value.replaceChildren();
  shown = { panel, tab };
  if (!tab) return;
  const element = document.createElement('div');
  element.className = 'h-full';
  element.dataset.testid = `panel-tab-${tab}`;
  content.value.append(element);
  unmountTab = panel.mountTab(tab, element);
}

watch([() => props.version, () => props.panel, content], draw, { immediate: true, flush: 'post' });

// Which tabs fit in the strip, from a hidden copy of every tab that gives each one's width.
const bar = ref<HTMLElement>();
const measure = ref<HTMLElement>();
const fit = ref<TabFit>({ shown: [], more: [] });
const GAP = 2;
const MORE = 30;
function refit (): void {
  if (!bar.value || !measure.value) return;
  const buttons = [...measure.value.querySelectorAll<HTMLElement>('button')];
  const widths = props.tabs.map((tab, i) => ({ id: tab.id, width: buttons[i]?.offsetWidth ?? 0 }));
  fit.value = fitTabs(widths, bar.value.clientWidth, GAP, MORE, props.panel.tab);
}
useResizeObserver(bar, refit);
watch([() => props.version, () => props.tabs], () => { void nextTick(refit); }, { immediate: true });
const stripTabs = computed(() => props.tabs.filter((tab) => !fit.value.shown.length || fit.value.shown.includes(tab.id)));
const moreTabs = computed(() => props.tabs.filter((tab) => fit.value.more.includes(tab.id)));

const tabClass = (on: boolean) => cn(
  'flex h-7 shrink-0 items-center gap-1.5 rounded-md px-2 text-caption whitespace-nowrap [&_svg]:size-3.5',
  on ? 'bg-active font-medium text-default' : 'text-weak hover:text-default',
);

onBeforeUnmount(() => unmountTab?.());
</script>

<template>
  <aside aria-label="Panel" data-testid="plugin-panel" class="flex w-full min-w-0 flex-col">
    <header class="flex h-11 shrink-0 items-center gap-1 border-b border-default px-2">
      <div v-if="commands.length" class="mr-1 flex shrink-0 items-center gap-1">
        <Button
          v-for="command in commands"
          :key="command.id"
          size="icon-xs"
          variant="ghost"
          :aria-label="command.title"
          :title="commandState(command).disabled ?? command.title"
          :aria-pressed="commandState(command).pressed ?? false"
          :disabled="!!commandState(command).disabled"
          :data-testid="`command-${command.id}`"
          @click="emit('run', command)"
        >
          <component :is="panelCommandIcon(command)" />
        </Button>
      </div>
      <div ref="bar" class="relative flex min-w-0 flex-1 items-center gap-1">
        <div role="tablist" class="flex min-w-0 items-center gap-1 overflow-hidden">
          <button
            v-for="tab in stripTabs"
            :key="tab.id"
            type="button"
            role="tab"
            :aria-selected="panel.tab === tab.id"
            :class="tabClass(panel.tab === tab.id)"
            :data-testid="`tab-${tab.id}`"
            @click="panel.show(tab.id)"
          >
            <component :is="tabIcon(tab)" v-if="tabIcon(tab)" />
            {{ tab.title }}
          </button>
        </div>
        <DropdownMenu v-if="moreTabs.length">
          <DropdownMenuTrigger as-child>
            <Button size="icon-xs" variant="ghost" aria-label="More tabs" title="More tabs" data-testid="more-tabs">
              <component :is="moreTabsIcon" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" data-testid="more-tabs-menu">
            <DropdownMenuItem
              v-for="tab in moreTabs"
              :key="tab.id"
              :data-testid="`tab-${tab.id}`"
              @select="panel.show(tab.id)"
            >
              <component :is="tabIcon(tab)" v-if="tabIcon(tab)" />
              {{ tab.title }}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <div ref="measure" aria-hidden="true" class="pointer-events-none invisible absolute top-0 left-0 flex gap-1">
          <button v-for="tab in tabs" :key="tab.id" type="button" tabindex="-1" :class="tabClass(false)">
            <component :is="tabIcon(tab)" v-if="tabIcon(tab)" />
            {{ tab.title }}
          </button>
        </div>
      </div>
      <Button size="icon-xs" variant="ghost" aria-label="Close panel" @click="panel.hide()">
        <X />
      </Button>
    </header>
    <div ref="content" class="min-h-0 flex-1" />
  </aside>
</template>
