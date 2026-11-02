<script setup lang="ts">
import type { DropdownMenuSubContentEmits, DropdownMenuSubContentProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { reactiveOmit } from '@vueuse/core';
import { DropdownMenuSubContent, useForwardPropsEmits } from 'reka-ui';
import { cn } from '@/lib/utils';

const props = defineProps<DropdownMenuSubContentProps & { class?: HTMLAttributes['class'] }>();
const emits = defineEmits<DropdownMenuSubContentEmits>();

const delegatedProps = reactiveOmit(props, 'class');

const forwarded = useForwardPropsEmits(delegatedProps, emits);
</script>

<template>
  <DropdownMenuSubContent
    data-slot="dropdown-menu-sub-content"
    v-bind="forwarded"
    :class="
      cn(
        'z-50 max-w-(--reka-dropdown-menu-content-available-width) min-w-[96px] origin-(--reka-dropdown-menu-content-transform-origin) overflow-hidden rounded-xl bg-popover p-1 text-popover-foreground shadow-e2 duration-100 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-97 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-97',
        props.class,
      )
    "
  >
    <slot />
  </DropdownMenuSubContent>
</template>
