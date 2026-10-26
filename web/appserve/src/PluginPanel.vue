<script setup lang="ts">
// The panel's chrome: the plugins' tabs as a tab strip, and the open tab drawn by its plugin. What
// each tab shows, and the panel's state (the tab, the node selected, pick mode), are the SDK's;
// this only places them.
import { onBeforeUnmount, ref, watch } from 'vue';
import type { Panel, Tab } from '@holistics/anfra-sdk/host';
import Icon from './Icon.vue';

const props = defineProps<{
  panel: Panel,
  tabs: readonly Tab[],
  // Bumped on each change to the panel, so the strip follows it.
  version: number,
}>();

const content = ref<HTMLElement>();
let unmountTab: (() => void) | undefined;
let shown: string | undefined;

// The open tab, drawn into its own element: a tab draws in that element's shadow root, so a new
// element per tab keeps one tab's root from being reused by another.
function draw (): void {
  const tab = props.panel.tab;
  if (tab === shown || !content.value) return;
  unmountTab?.();
  unmountTab = undefined;
  content.value.replaceChildren();
  shown = tab;
  if (!tab) return;
  const element = document.createElement('div');
  element.className = 'panel-tab';
  element.dataset.testid = `panel-tab-${tab}`;
  content.value.append(element);
  unmountTab = props.panel.mountTab(tab, element);
}

watch([() => props.version, content], draw, { immediate: true, flush: 'post' });
onBeforeUnmount(() => unmountTab?.());
</script>

<template>
  <aside class="plugin-panel" aria-label="Panel" data-testid="plugin-panel">
    <header class="panel-header">
      <div class="panel-tabs" role="tablist">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          type="button"
          role="tab"
          :aria-selected="panel.tab === tab.id"
          :data-testid="`tab-${tab.id}`"
          @click="panel.show(tab.id)"
        >
          {{ tab.title }}
        </button>
      </div>
      <button type="button" class="icon-button" aria-label="Close panel" @click="panel.hide()">
        <Icon name="close" />
      </button>
    </header>
    <div ref="content" class="panel-content" />
  </aside>
</template>
