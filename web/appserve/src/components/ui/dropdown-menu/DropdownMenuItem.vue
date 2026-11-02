<script setup lang="ts">
import type { DropdownMenuItemProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { reactiveOmit } from '@vueuse/core';
import { DropdownMenuItem, useForwardProps } from 'reka-ui';
import { cn } from '@/lib/utils';

const props = withDefaults(
  defineProps<
    DropdownMenuItemProps & {
      class?: HTMLAttributes['class'];
      inset?: boolean;
      variant?: 'default' | 'destructive';
    }
  >(),
  {
    variant: 'default',
  },
);

const delegatedProps = reactiveOmit(props, 'inset', 'variant', 'class');

const forwardedProps = useForwardProps(delegatedProps);
</script>

<template>
  <DropdownMenuItem
    data-slot="dropdown-menu-item"
    :data-inset="inset ? '' : undefined"
    :data-variant="variant"
    v-bind="forwardedProps"
    :class="
      cn(
        'group/dropdown-menu-item relative flex cursor-default items-center gap-1.5 whitespace-nowrap rounded-[calc(var(--radius-xl)-4px)] px-1.5 py-1 text-body outline-hidden select-none focus:bg-accent focus:text-accent-foreground not-data-[variant=destructive]:focus:**:text-accent-foreground data-disabled:pointer-events-none data-disabled:text-disabled data-inset:pl-7 data-[variant=destructive]:text-danger-default data-[variant=destructive]:focus:bg-danger-secondary-default data-[variant=destructive]:focus:text-danger-default [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4 data-disabled:*:[svg]:icon-disabled data-[variant=destructive]:*:[svg]:icon-danger-default',
        props.class,
      )
    "
  >
    <slot />
  </DropdownMenuItem>
</template>
