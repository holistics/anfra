<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { coreApiBackend } from '@holistics/anfra-sdk/api';
import type { DatasetDescriptor, InspectedApp, User } from '@holistics/anfra-sdk/common';
import { mountDataApp, type MountedDataApp } from '@holistics/anfra-sdk/host';
import { api, definition } from './server';

const props = defineProps<{
  path: string,
  inspecting: boolean,
  datasets: Record<string, DatasetDescriptor>,
  reader: User,
}>();
const emit = defineEmits<{ load: [], inspect: [apps: InspectedApp[]], error: [message: string] }>();

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
      onInspect: (apps) => emit('inspect', apps),
      title: 'Data App',
    });
    mounted.frame.classList.add('data-app');
    mounted.frame.dataset.testid = 'data-app-frame';
    mounted.frame.addEventListener('load', () => emit('load'));
    mounted.setInspecting(props.inspecting);
  } catch (err) {
    emit('error', (err as Error).message);
  }
});

watch(() => props.inspecting, (open) => mounted?.setInspecting(open));

onBeforeUnmount(() => mounted?.unmount());
</script>

<template>
  <div ref="container" class="data-app-container" />
</template>
