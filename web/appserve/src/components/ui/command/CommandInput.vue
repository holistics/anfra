<script setup lang="ts">
import type { ListboxFilterProps } from 'reka-ui';

import type { HTMLAttributes } from 'vue';
import { SearchIcon } from '@lucide/vue';
import { reactiveOmit } from '@vueuse/core';
import { cn } from '@/lib/utils';
import { ListboxFilter, useForwardProps } from 'reka-ui';
import { useCommand } from '.';

defineOptions({
  inheritAttrs: false,
});

const props = defineProps<
  ListboxFilterProps & {
    class?: HTMLAttributes['class'];
  }
>();

const delegatedProps = reactiveOmit(props, 'class');

const forwardedProps = useForwardProps(delegatedProps);

const { filterState } = useCommand();
</script>

<template>
  <div
    data-slot="command-input-wrapper"
    class="flex h-11 shrink-0 items-center gap-2 border-b border-default px-3.5"
  >
    <SearchIcon class="size-4 shrink-0 icon-weak" />
    <ListboxFilter
      v-bind="{ ...forwardedProps, ...$attrs }"
      v-model="filterState.search"
      data-slot="command-input"
      auto-focus
      :class="
        cn(
          'h-full min-w-0 flex-1 bg-transparent text-body outline-hidden placeholder:text-placeholder disabled:cursor-not-allowed disabled:text-disabled',
          props.class,
        )
      "
    />
  </div>
</template>
