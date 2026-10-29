<script setup lang="ts">
// The panel's chrome: the panel's commands (Pick) and the plugins' tabs as a tab strip, and the
// open tab drawn by its plugin. What each tab shows, and the panel's state (the tab, the node
// selected, pick mode), are the SDK's; this only places them. Tabs that don't fit go behind a More tabs button, as in a browser's
// devtools, the selected one always staying in the strip.
import {
  computed, nextTick, onBeforeUnmount, onMounted, ref, watch,
} from 'vue';
import type { Command, CommandState, Panel, Tab } from '@holistics/anfra-sdk/host';
import Icon from './Icon.vue';
import { fitTabs } from './tabFit';

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

// The icon appserve draws for each icon a tab names; a tab naming another has none.
const TAB_ICONS: Record<string, InstanceType<typeof Icon>['$props']['name']> = {
  overview: 'overview', table: 'sheet', query: 'database', lineage: 'lineage', tree: 'tree', code: 'code',
};
const tabIcon = (tab: Tab) => (tab.icon ? TAB_ICONS[tab.icon] : undefined);
const COMMAND_ICONS: Record<string, InstanceType<typeof Icon>['$props']['name']> = { pick: 'pick' };
const commandIcon = (command: Command) => (command.icon ? COMMAND_ICONS[command.icon] : undefined) ?? 'app';

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
  element.className = 'panel-tab';
  element.dataset.testid = `panel-tab-${tab}`;
  content.value.append(element);
  unmountTab = panel.mountTab(tab, element);
}

watch([() => props.version, () => props.panel, content], draw, { immediate: true, flush: 'post' });

// The strip and a hidden copy of every tab in it, which gives each tab's width.
const bar = ref<HTMLElement>();
const measure = ref<HTMLElement>();
const fit = ref<{ shown: string[], more: string[] }>({ shown: [], more: [] });
const GAP = 2;
const MORE = 30;
function refit (): void {
  if (!bar.value || !measure.value) return;
  const buttons = [...measure.value.querySelectorAll<HTMLElement>('button')];
  const widths = props.tabs.map((tab, i) => ({ id: tab.id, width: buttons[i]?.offsetWidth ?? 0 }));
  fit.value = fitTabs(widths, bar.value.clientWidth, GAP, MORE, props.panel.tab);
}
const stripTabs = computed(() => props.tabs.filter((tab) => fit.value.shown.includes(tab.id) || !fit.value.shown.length));
const moreTabs = computed(() => props.tabs.filter((tab) => fit.value.more.includes(tab.id)));
const resize = new ResizeObserver(() => refit());
onMounted(() => { if (bar.value) resize.observe(bar.value); });
watch([() => props.version, () => props.tabs], () => { void nextTick(refit); }, { immediate: true });

const moreOpen = ref(false);
const menu = ref<HTMLElement>();
const items = () => [...(menu.value?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
async function toggleMore (): Promise<void> {
  moreOpen.value = !moreOpen.value;
  if (!moreOpen.value) return;
  await nextTick();
  items()[0]?.focus();
}
function choose (id: string): void {
  moreOpen.value = false;
  props.panel.show(id);
}
function onMenuKey (event: KeyboardEvent): void {
  const list = items();
  const at = list.indexOf(document.activeElement as HTMLElement);
  if (event.key === 'Escape') moreOpen.value = false;
  else if (event.key === 'ArrowDown') list[(at + 1) % list.length]?.focus();
  else if (event.key === 'ArrowUp') list[(at - 1 + list.length) % list.length]?.focus();
  else return;
  event.preventDefault();
}
function onDocumentPointer (event: PointerEvent): void {
  if (!(event.target as HTMLElement).closest('.panel-more')) moreOpen.value = false;
}
watch(moreOpen, (open) => {
  if (open) document.addEventListener('pointerdown', onDocumentPointer, true);
  else document.removeEventListener('pointerdown', onDocumentPointer, true);
});

onBeforeUnmount(() => {
  unmountTab?.();
  resize.disconnect();
  document.removeEventListener('pointerdown', onDocumentPointer, true);
});
</script>

<template>
  <aside class="plugin-panel" aria-label="Panel" data-testid="plugin-panel">
    <header class="panel-header">
      <div v-if="commands.length" class="panel-commands">
        <button
          v-for="command in commands"
          :key="command.id"
          type="button"
          class="icon-button"
          :class="{ active: commandState(command).pressed }"
          :aria-label="command.title"
          :title="commandState(command).disabled ?? command.title"
          :aria-pressed="commandState(command).pressed ?? false"
          :disabled="!!commandState(command).disabled"
          :data-testid="`command-${command.id}`"
          @click="emit('run', command)"
        >
          <Icon :name="commandIcon(command)" />
        </button>
      </div>
      <div ref="bar" class="panel-tabbar">
        <div class="panel-tabs" role="tablist">
          <button
            v-for="tab in stripTabs"
            :key="tab.id"
            type="button"
            role="tab"
            :aria-selected="panel.tab === tab.id"
            :data-testid="`tab-${tab.id}`"
            @click="panel.show(tab.id)"
          >
            <Icon v-if="tabIcon(tab)" :name="tabIcon(tab)!" />
            {{ tab.title }}
          </button>
        </div>
        <div v-if="moreTabs.length" class="more panel-more">
          <button
            type="button"
            class="a-button"
            data-variant="ghost"
            data-size="icon-xs"
            aria-label="More tabs"
            title="More tabs"
            aria-haspopup="menu"
            data-testid="more-tabs"
            :aria-expanded="moreOpen"
            @click="toggleMore"
          >
            <Icon name="overflow" />
          </button>
          <div v-if="moreOpen" ref="menu" class="menu a-menu" role="menu" data-testid="more-tabs-menu" @keydown="onMenuKey">
            <button
              v-for="tab in moreTabs"
              :key="tab.id"
              type="button"
              class="a-menu-item"
              role="menuitem"
              :data-testid="`tab-${tab.id}`"
              @click="choose(tab.id)"
            >
              <Icon v-if="tabIcon(tab)" :name="tabIcon(tab)!" />
              {{ tab.title }}
            </button>
          </div>
        </div>
        <div ref="measure" class="panel-tabs panel-tabs-measure" aria-hidden="true">
          <button v-for="tab in tabs" :key="tab.id" type="button" tabindex="-1">
            <Icon v-if="tabIcon(tab)" :name="tabIcon(tab)!" />{{ tab.title }}
          </button>
        </div>
      </div>
      <button type="button" class="a-button" data-variant="ghost" data-size="icon-xs" aria-label="Close panel" @click="panel.hide()">
        <Icon name="close" />
      </button>
    </header>
    <div ref="content" class="panel-content" />
  </aside>
</template>
