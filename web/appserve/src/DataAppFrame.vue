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
}>();
const emit = defineEmits<{
  load: [],
  inspect: [apps: InspectedApp[], structure: InspectedStructure],
  pick: [node: string | null],
  error: [message: string],
}>();

const container = ref<HTMLElement>();
let mounted: MountedDataApp | undefined;

// The page's host runtime: every plugin anfra has, over the core API. Made again when the datasets
// are (a semantic layer change), since the Backend resolves fields against them.
let host: { datasets: Record<string, DatasetDescriptor>, runtime: Host } | undefined;
function hostFor (datasets: Record<string, DatasetDescriptor>): Host {
  if (host?.datasets !== datasets) {
    const backend = coreApiBackend(api, { datasets });
    host = {
      datasets,
      runtime: createHost({
        plugins: [corePlugin(backend), exportPlugin(backend), lineagePlugin(backend), renderPlugin(), inspectPlugin()],
      }),
    };
  }
  return host.runtime;
}

// Mounted once per path (the component is keyed by it, and by each reload): the definition is read
// as it is now, and provisioned with the datasets as they are now.
onMounted(async () => {
  try {
    const { html, baseHref } = await definition(props.path);
    if (!container.value) return;
    mounted = hostFor(props.datasets).mount({
      container: container.value,
      definition: html,
      baseHref,
      datasets: props.datasets,
      user: props.reader,
      title: 'Data App',
    });
    mounted.on('inspect.snapshot', ({ apps, structure }: { apps: InspectedApp[], structure: InspectedStructure }) => {
      if (props.inspecting) emit('inspect', apps, structure);
    });
    mounted.on('inspect.picked', ({ node }: { node: string | null }) => {
      if (props.inspecting) emit('pick', node);
    });
    mounted.frame.classList.add('data-app');
    mounted.frame.dataset.testid = 'data-app-frame';
    mounted.frame.addEventListener('load', () => emit('load'));
    void mounted.call('inspect.watch', props.inspecting);
    if (props.picking) void mounted.call('inspect.pick', true);
  } catch (err) {
    emit('error', (err as Error).message);
  }
});

watch(() => props.inspecting, (open) => { void mounted?.call('inspect.watch', open); });
watch(() => props.picking, (on) => { void mounted?.call('inspect.pick', on); });

onBeforeUnmount(() => mounted?.unmount());

// The inspect panel and the toolbar reach the page through here: highlight a node, scroll to one,
// or to an entity wherever it is drawn; and render the page or a node, which the frame saves.
defineExpose({
  highlight: (node: string | null) => { void mounted?.call('inspect.highlight', node); },
  locate: (target: LocateTarget) => { void mounted?.call('inspect.locate', target); },
  render: (request: RenderRequest): Promise<Rendering> => (mounted
    ? mounted.call<Rendering>('render.draw', request)
    : Promise.reject(new Error('The Data App is not loaded yet.'))),
});
</script>

<template>
  <div ref="container" class="data-app-container" />
</template>
