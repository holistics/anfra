<script setup lang="ts">
import type { PopoverContentEmits, PopoverContentProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { reactiveOmit } from '@vueuse/core';
import { cn } from '@/lib/utils';
import { PopoverContent, PopoverPortal, useForwardPropsEmits } from 'reka-ui';

defineOptions({
  inheritAttrs: false,
});

const props = withDefaults(
  defineProps<PopoverContentProps & { class?: HTMLAttributes['class'] }>(),
  {
    align: 'center',
    sideOffset: 4,
  },
);
const emits = defineEmits<PopoverContentEmits>();

const delegatedProps = reactiveOmit(props, 'class');

const forwarded = useForwardPropsEmits(delegatedProps, emits);
</script>

<template>
  <PopoverPortal>
    <PopoverContent
      data-slot="popover-content"
      v-bind="{ ...$attrs, ...forwarded }"
      :class="
        cn(
          'z-50 flex w-auto max-w-[min(20rem,calc(100vw-2rem))] flex-col gap-2.5 rounded-xl bg-popover p-3 text-body text-popover-foreground shadow-e2 outline-none',
          props.class,
        )
      "
    >
      <slot />
    </PopoverContent>
  </PopoverPortal>
</template>
