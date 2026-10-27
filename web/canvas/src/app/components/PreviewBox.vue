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

<style>
.ac-preview {
  position: relative;
  display: grid;
  place-items: center;
  height: 80px;
  overflow: hidden;
  border-radius: 10px 10px 0 0;
  background: var(--ac-sunken);
  pointer-events: none;
}
.ac-preview-stage {
  /* Blocks (cards, alerts, fields) fill the width; inline ones (buttons, badges) sit in the middle */
  display: flow-root;
  text-align: center;
  width: 320px;
  min-height: 106px;
  padding: 12px;
  zoom: 0.75;
  color: initial;
  font-size: initial;
  line-height: initial;
}
.ac-preview-stage > * { max-width: 100%; text-align: start; }
</style>
