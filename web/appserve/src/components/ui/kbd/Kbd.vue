<script setup lang="ts">
import type { HTMLAttributes } from 'vue';
import { cn } from '@/lib/utils';

const props = defineProps<{
  class?: HTMLAttributes['class'];
}>();
</script>

<template>
  <kbd
    data-slot="kbd"
    :class="
      cn(
        'pointer-events-none inline-flex h-5 w-fit min-w-5 items-center justify-center gap-1 rounded-md bg-active px-1 font-sans text-caption font-medium text-weak select-none [&_svg:not([class*=size-])]:size-3',
        props.class,
      )
    "
  >
    <slot />
  </kbd>
</template>
