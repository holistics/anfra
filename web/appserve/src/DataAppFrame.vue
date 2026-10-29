<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { coreApiBackend } from '@holistics/anfra-sdk/api';
import type {
  DatasetDescriptor, ThemeInput, User,
} from '@holistics/anfra-sdk/common';
import { createHost, type Host, type MountedDataApp } from '@holistics/anfra-sdk/host';
import {
  corePlugin, exportPlugin, inspectPlugin, lineagePlugin, renderPlugin,
} from '@holistics/anfra-sdk/plugins';
import { api, definition } from './server';

const props = defineProps<{
  path: string,
  datasets: Record<string, DatasetDescriptor>,
  reader: User,
  allowOrigins: string[],
  theme: ThemeInput,
}>();
const emit = defineEmits<{
  load: [],
  // The host runtime and this Data App's mount: the shell draws the plugins' commands and the panel on them.
  mounted: [host: Host, mounted: MountedDataApp],
  unmounted: [mounted: MountedDataApp],
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
        theme: props.theme,
        // The command palette's keys (App.vue), so they open it with focus in the Data App too.
        keys: ['Meta+K', 'Ctrl+K'],
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
    const runtime = hostFor(props.datasets, props.allowOrigins);
    runtime.setTheme(props.theme);
    mounted = runtime.mount({
      container: container.value,
      definition: html,
      datasets: props.datasets,
      user: props.reader,
      title: 'Data App',
    });
    mounted.frame.classList.add('data-app');
    mounted.frame.dataset.testid = 'data-app-frame';
    mounted.frame.addEventListener('load', () => emit('load'));
    emit('mounted', runtime, mounted);
  } catch (err) {
    emit('error', (err as Error).message);
  }
});

watch(() => props.theme, (theme) => { host?.runtime.setTheme(theme); });

onBeforeUnmount(() => {
  if (!mounted) return;
  mounted.unmount();
  emit('unmounted', mounted);
});

</script>

<template>
  <div ref="container" class="data-app-container" />
</template>
