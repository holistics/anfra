<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { coreApiBackend } from '@holistics/anfra-sdk/api';
import type {
  DatasetDescriptor, ThemeInput, User,
} from '@holistics/anfra-sdk/common';
import { createHost, type Host, type MountedDataApp } from '@holistics/anfra-sdk/host';
import { installedPlugins } from '@/lib/plugins';
import { api, definition, report } from '@/lib/server';

const props = defineProps<{
  path: string,
  datasets: Record<string, DatasetDescriptor>,
  reader: User,
  allowOrigins: string[],
  // The plugins the repo's settings leave on, beside core.
  plugins: string[],
  // Whether to send this Data App's reports to anfra.log (not with ANFRA_NO_APP_LOGS).
  appLogs: boolean,
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

// The page's host runtime: core and the plugins the repo's settings leave on, over the core API,
// under the repo's allowed origins. Made again when the datasets are (a semantic layer change), since
// the Backend resolves fields against them, or the origins or the plugins are.
let host: { datasets: Record<string, DatasetDescriptor>, origins: string, plugins: string, runtime: Host } | undefined;
function hostFor (datasets: Record<string, DatasetDescriptor>, allowOrigins: string[], on: string[]): Host {
  const origins = allowOrigins.join(' ');
  const plugins = on.join(' ');
  if (host?.datasets !== datasets || host.origins !== origins || host.plugins !== plugins) {
    const backend = coreApiBackend(api, { datasets });
    host = {
      datasets,
      origins,
      plugins,
      runtime: createHost({
        plugins: installedPlugins(on, backend),
        allowOrigins,
        theme: props.theme,
        // anfra serve is a development tool, even released: the author's console lines are theirs
        // to read, in the panel and in anfra.log.
        captureConsole: true,
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
    const runtime = hostFor(props.datasets, props.allowOrigins, props.plugins);
    runtime.setTheme(props.theme);
    mounted = runtime.mount({
      container: container.value,
      definition: html,
      datasets: props.datasets,
      user: props.reader,
      title: 'Data App',
    });
    mounted.frame.dataset.testid = 'data-app-frame';
    mounted.frame.addEventListener('load', () => emit('load'));
    // Into anfra.log, with this Data App's path: where an agent reads what went wrong, after the
    // line that says which load of the file it was.
    const { path } = props;
    if (props.appLogs) {
      report(path, 'mounted');
      mounted.on('anfra.error', (event) => report(path, 'error', event));
      mounted.on('anfra.console', (event) => report(path, 'console', event));
    }
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
  <!-- The frame fills it: the Data App's own page, white whatever the theme. -->
  <div ref="container" class="flex min-h-0 min-w-0 flex-1 [&>iframe]:min-h-0 [&>iframe]:w-full [&>iframe]:min-w-0 [&>iframe]:flex-1 [&>iframe]:border-0 [&>iframe]:bg-white" />
</template>
