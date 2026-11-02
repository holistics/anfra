<script setup lang="ts">
import type { ListboxContentProps } from 'reka-ui';
import type { HTMLAttributes } from 'vue';
import { reactiveOmit, useResizeObserver } from '@vueuse/core';
import { cn } from '@/lib/utils';
import { ListboxContent, useForwardProps } from 'reka-ui';
import { onMounted, ref } from 'vue';

const props = defineProps<ListboxContentProps & { class?: HTMLAttributes['class'] }>();

const delegatedProps = reactiveOmit(props, 'class');

const forwarded = useForwardProps(delegatedProps);

// Whether the list runs on past its top or bottom edge, where it fades out (fade-y).
const list = ref<InstanceType<typeof ListboxContent>>();
const items = ref<HTMLElement>();
const above = ref(false);
const below = ref(false);
function measure() {
  const el = list.value?.$el as HTMLElement | undefined;
  if (!el) return;
  above.value = el.scrollTop > 1;
  below.value = el.scrollTop + el.clientHeight < el.scrollHeight - 1;
}
useResizeObserver(items, measure);
onMounted(measure);
</script>

<template>
  <ListboxContent
    ref="list"
    data-slot="command-list"
    v-bind="forwarded"
    :data-more-above="above || undefined"
    :data-more-below="below || undefined"
    :class="cn('min-h-0 flex-1 scroll-py-1 overflow-x-hidden overflow-y-auto p-1 outline-none fade-y', props.class)"
    @scroll.passive="measure"
  >
    <div ref="items" role="presentation">
      <slot />
    </div>
  </ListboxContent>
</template>
