<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { coreApiBackend } from '@holistics/anfra-sdk/api';
import type { DatasetDescriptor, InspectedApp, User } from '@holistics/anfra-sdk/common';
import type { InspectedStructure, LocateTarget } from './inspect';
import { mountDataApp, type MountedDataApp } from '@holistics/anfra-sdk/host';
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

// Mounted once per path (the component is keyed by it, and by each reload): the definition is read
// as it is now, and provisioned with the datasets as they are now.
onMounted(async () => {
  try {
    const { html, baseHref } = await definition(props.path);
    if (!container.value) return;
    mounted = mountDataApp({
      container: container.value,
      definition: html,
      baseHref,
      datasets: props.datasets,
      user: props.reader,
      backend: coreApiBackend(api, { datasets: props.datasets }),
      onInspect: (apps, structure) => emit('inspect', apps, structure),
      onPick: (node) => emit('pick', node),
      title: 'Data App',
    });
    mounted.frame.classList.add('data-app');
    mounted.frame.dataset.testid = 'data-app-frame';
    mounted.frame.addEventListener('load', () => emit('load'));
    mounted.setInspecting(props.inspecting);
    if (props.picking) mounted.setPicking(true);
  } catch (err) {
    emit('error', (err as Error).message);
  }
});

watch(() => props.inspecting, (open) => mounted?.setInspecting(open));
watch(() => props.picking, (on) => mounted?.setPicking(on));

onBeforeUnmount(() => mounted?.unmount());

// The inspect panel reaches the page through here: highlight a node, scroll to one, or to an
// entity wherever it is drawn. Nothing comes back but the next snapshot.
defineExpose({
  highlight: (node: string | null) => mounted?.highlight(node),
  locate: (target: LocateTarget) => mounted?.locate(target),
});
</script>

<template>
  <div ref="container" class="data-app-container" />
</template>
