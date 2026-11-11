<script setup lang="ts">
import type { TooltipProviderProps } from 'reka-ui';
import { TooltipProvider } from 'reka-ui';

// The first tooltip waits 400ms; moving to the next one within 300ms opens it at once.
const props = withDefaults(defineProps<TooltipProviderProps>(), {
  delayDuration: 400,
  skipDelayDuration: 300,
});
</script>

<template>
  <TooltipProvider v-bind="props">
    <slot />
  </TooltipProvider>
</template>
