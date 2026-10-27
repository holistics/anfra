<script setup lang="ts">
// One palette item drawn with the app's own components, small and inert. A component that fails
// to draw out of context (one that needs a provider, say) shows its name instead.
import { onErrorCaptured, ref } from 'vue';
import { useStore } from '../context.ts';

defineProps<{ id: string }>();
const store = useStore();
const failed = ref(false);
onErrorCaptured(() => {
  failed.value = true;
  return false;
});
</script>

<template>
  <div class="ac-preview" aria-hidden="true">
    <div v-if="store.previews && !failed" class="ac-preview-stage" inert>
      <component :is="store.previews" :id="id" />
    </div>
    <span v-else class="ac-caption">No preview</span>
  </div>
</template>
