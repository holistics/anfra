<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { serveBridge, type BridgeHandle } from './bridge';

const props = defineProps<{ path: string, inspecting: boolean }>();
const emit = defineEmits<{ load: [], inspect: [apps: unknown[]] }>();

const frame = ref<HTMLIFrameElement>();
let bridge: BridgeHandle | undefined;

// Serve the bridge from the moment the frame element exists, before its scripts can post anything.
// Switching Data Apps remounts the element (it is keyed by path), which re-arms this.
watch(frame, (el) => {
  bridge?.stop();
  bridge = el ? serveBridge(el, (apps) => emit('inspect', apps)) : undefined;
  bridge?.setInspecting(props.inspecting);
});

watch(() => props.inspecting, (open) => bridge?.setInspecting(open));

onBeforeUnmount(() => bridge?.stop());
</script>

<template>
  <iframe
    :key="path"
    ref="frame"
    class="data-app"
    title="Data App"
    data-testid="data-app-frame"
    sandbox="allow-scripts"
    :src="`/_anfra/data-apps/${path.split('/').map(encodeURIComponent).join('/')}`"
    @load="emit('load')"
  />
</template>
