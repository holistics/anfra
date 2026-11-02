<script setup lang="ts">
import type { HTMLAttributes } from 'vue';
import type { StatusPillVariants } from '.';
import { cn } from '@/lib/utils';
import { statusPillVariants } from '.';

const props = defineProps<{
  tone?: StatusPillVariants['tone'];
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <span
    data-slot="status-pill"
    :data-tone="tone"
    :class="cn(statusPillVariants({ tone }), props.class)"
  >
    <slot />
  </span>
</template>
