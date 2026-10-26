<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { coreApiBackend } from '@holistics/anfra-sdk/api';
import type {
  DatasetDescriptor, InspectedApp, Rendering, RenderRequest, User,
} from '@holistics/anfra-sdk/common';
import { createHost, type Host, type MountedDataApp } from '@holistics/anfra-sdk/host';
import {
  corePlugin, exportPlugin, inspectPlugin, lineagePlugin, renderPlugin,
} from '@holistics/anfra-sdk/plugins';
import type { InspectedStructure, LocateTarget } from './inspect';
import { api, definition } from './server';

const props = defineProps<{
  path: string,
  inspecting: boolean,
  picking: boolean,
  datasets: Record<string, DatasetDescriptor>,
  reader: User,
  allowOrigins: string[],
}>();
const emit = defineEmits<{
  load: [],
  inspect: [apps: InspectedApp[], structure: InspectedStructure],
  pick: [node: string | null],
  error: [message: string],
}>();

const container = ref<HTMLElement>();
let mounted: MountedDataApp | undefined;

// The page's host runtime: every plugin anfra has, over the core API, under the repo's allowed
// origins. Made again when the datasets are (a semantic layer change), since the Backend resolves
// fields against them, or the origins are.
let host: { datasets: Record<string, DatasetDescriptor>, origins: string, runtime: Host } | undefined;
function hostFor (datasets: Record<string, DatasetDescriptor>, allowOrigins: string[]): Host {
  const origins = allowOrigins.join(' ');
  if (host?.datasets !== datasets || host.origins !== origins) {
    const backend = coreApiBackend(api, { datasets });
    host = {
      datasets,
      origins,
      runtime: createHost({
        plugins: [corePlugin(backend), exportPlugin(backend), lineagePlugin(backend), renderPlugin(), inspectPlugin()],
        allowOrigins,
      }),
    };
  }
  return host.runtime;
}

// Mounted once per path (the component is keyed by it, and by each reload): the definition is read
// as it is now, and provisioned with the datasets as they are now.
onMounted(async () => {
  try {
    const html = await definition(props.path);
    if (!container.value) return;
    mounted = hostFor(props.datasets, props.allowOrigins).mount({
      container: container.value,
      definition: html,
      datasets: props.datasets,
      user: props.reader,
      title: 'Data App',
    });
    // The panel's state, as App.vue holds it for now: the snapshot while open, and a pick.
    const panel = mounted.panel;
    let picking = panel.picking;
    panel.subscribe(() => {
      if (panel.snapshot) emit('inspect', panel.snapshot.apps, panel.snapshot.structure);
      if (picking && !panel.picking) emit('pick', panel.selection);
      picking = panel.picking;
    });
    mounted.frame.classList.add('data-app');
    mounted.frame.dataset.testid = 'data-app-frame';
    mounted.frame.addEventListener('load', () => emit('load'));
    if (props.inspecting) panel.show();
    if (props.picking) panel.pick(true);
  } catch (err) {
    emit('error', (err as Error).message);
  }
});

watch(() => props.inspecting, (open) => { if (open) mounted?.panel.show(); else mounted?.panel.hide(); });
watch(() => props.picking, (on) => { if (mounted && mounted.panel.picking !== on) mounted.panel.pick(on); });

onBeforeUnmount(() => mounted?.unmount());

// The inspect panel and the toolbar reach the page through here: highlight a node, scroll to one,
// or to an entity wherever it is drawn; and render the page or a node, which the frame saves.
defineExpose({
  highlight: (node: string | null) => { mounted?.panel.highlight(node); },
  locate: (target: LocateTarget) => { mounted?.panel.locate(target); },
  render: (request: RenderRequest): Promise<Rendering> => (mounted
    ? mounted.call<Rendering>('render.draw', request)
    : Promise.reject(new Error('The Data App is not loaded yet.'))),
});
</script>

<template>
  <div ref="container" class="data-app-container" />
</template>
