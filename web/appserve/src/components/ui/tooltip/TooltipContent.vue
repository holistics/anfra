<script setup lang="ts">
import type { TooltipContentEmits, TooltipContentProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { reactiveOmit } from '@vueuse/core';
import { TooltipArrow, TooltipContent, TooltipPortal, useForwardPropsEmits } from 'reka-ui';
import { cn } from '@/lib/utils';

defineOptions({
  inheritAttrs: false,
});

const props = withDefaults(
  defineProps<TooltipContentProps & { class?: HTMLAttributes['class'] }>(),
  {
    sideOffset: 0,
  },
);

const emits = defineEmits<TooltipContentEmits>();

const delegatedProps = reactiveOmit(props, 'class');
const forwarded = useForwardPropsEmits(delegatedProps, emits);
</script>

<template>
  <TooltipPortal>
    <TooltipContent
      data-slot="tooltip-content"
      v-bind="{ ...forwarded, ...$attrs }"
      :class="
        cn(
          'z-50 inline-flex w-fit max-w-xs origin-(--reka-tooltip-content-transform-origin) items-center gap-1.5 rounded-md bg-inverted-default px-3 py-1.5 text-caption text-inverted-default has-data-[slot=kbd]:pr-1.5 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-97 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-97 **:data-[slot=kbd]:relative **:data-[slot=kbd]:isolate **:data-[slot=kbd]:z-50 **:data-[slot=kbd]:rounded-sm data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-[state=delayed-open]:zoom-in-97',
          props.class,
        )
      "
    >
      <slot />

      <TooltipArrow
        class="z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-xs bg-inverted-default fill-inverted-default"
      />
    </TooltipContent>
  </TooltipPortal>
</template>
