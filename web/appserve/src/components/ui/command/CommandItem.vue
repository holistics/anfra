<script setup lang="ts">
import type { ListboxItemEmits, ListboxItemProps } from 'reka-ui';

import type { HTMLAttributes } from 'vue';
import { reactiveOmit, useCurrentElement } from '@vueuse/core';
import { cn } from '@/lib/utils';
import { ListboxItem, useForwardPropsEmits, useId } from 'reka-ui';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useCommand, useCommandGroup } from '.';

// filterValue: the text a search matches, when not the item's own text.
const props = defineProps<ListboxItemProps & { class?: HTMLAttributes['class'], filterValue?: string }>();
const emits = defineEmits<ListboxItemEmits>();

const delegatedProps = reactiveOmit(props, 'class', 'filterValue');

const forwarded = useForwardPropsEmits(delegatedProps, emits);

const id = useId();
const { filterState, allItems, allGroups } = useCommand();
const groupContext = useCommandGroup();

const isRender = computed(() => {
  if (!filterState.search) {
    return true;
  } else {
    const filteredCurrentItem = filterState.filtered.items.get(id);
    // If the filtered items is undefined means not in the all times map yet
    // Do the first render to add into the map
    if (filteredCurrentItem === undefined) {
      return true;
    }

    // Check with filter
    return filteredCurrentItem > 0;
  }
});

const itemRef = ref();
const currentElement = useCurrentElement(itemRef);
onMounted(() => {
  const element = currentElement.value;
  if (!(element instanceof HTMLElement)) return;

  // The text a search matches, kept current: an entry renamed while the list is open (Hide sidebar,
  // then Show sidebar) is found by its new name. Made in this hook, the watch stops with the item.
  watch(
    () => [props.filterValue, props.value],
    () => allItems.value.set(id, props.filterValue ?? element.textContent ?? props.value?.toString() ?? ''),
    { immediate: true },
  );

  const groupId = groupContext?.id;
  if (groupId) {
    if (!allGroups.value.has(groupId)) {
      allGroups.value.set(groupId, new Set([id]));
    } else {
      allGroups.value.get(groupId)?.add(id);
    }
  }
});
onUnmounted(() => {
  allItems.value.delete(id);
});
</script>

<template>
  <ListboxItem
    v-if="isRender"
    v-bind="forwarded"
    :id="id"
    ref="itemRef"
    data-slot="command-item"
    :class="
      cn(
        'group/command-item relative flex min-h-8 cursor-pointer items-center gap-2 rounded-[calc(var(--radius-xl)-4px)] px-2 py-1.5 text-body outline-hidden select-none data-highlighted:bg-hover data-highlighted:text-default data-disabled:pointer-events-none data-disabled:cursor-default data-disabled:text-disabled [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg]:icon-weak data-disabled:[&_svg]:icon-disabled [&_svg:not([class*=size-])]:size-4',
        props.class,
      )
    "
    @select="
      () => {
        filterState.search = '';
      }
    "
  >
    <slot />
  </ListboxItem>
</template>
