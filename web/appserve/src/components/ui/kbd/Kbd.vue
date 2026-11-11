<script setup lang="ts">
import type { HTMLAttributes } from 'vue';
import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <!-- In a tooltip, a key is a tint of the tooltip's own text on its inverse ink, not a light chip. -->
  <kbd
    data-slot="kbd"
    :class="
      cn(
        'pointer-events-none inline-flex h-5 w-fit min-w-5 items-center justify-center gap-1 rounded-md bg-active px-1 font-sans text-caption font-medium text-weak select-none [&_svg:not([class*=size-])]:size-3',
        'in-data-[slot=tooltip-content]:bg-(--anfra-color-text-inverted-default)/15 in-data-[slot=tooltip-content]:text-inverted-default',
        props.class,
      )
    "
  >
    <slot />
  </kbd>
</template>
