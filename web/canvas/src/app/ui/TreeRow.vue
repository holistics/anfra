<script setup lang="ts">
// A row of the layers tree: 12px in for each level, a chevron where it has children, its icon and
// name (`strong` for a screen, a section or a component), and a quiet note at its end.
import Icon, { type IconName } from './Icon.vue';

defineProps<{
  label: string;
  icon: IconName;
  depth?: number;
  meta?: string;
  strong?: boolean;
  current?: boolean;
  /** It has children, which show while it is `open`. */
  expandable?: boolean;
  open?: boolean;
}>();
const emit = defineEmits<{ toggle: [], pick: [] }>();
</script>

<template>
  <div class="ac-tree-row" :aria-current="current || undefined" :style="{ paddingLeft: `${(depth ?? 0) * 12 + 4}px` }">
    <button v-if="expandable" class="ac-tree-toggle" type="button" :aria-expanded="open" :aria-label="open ? 'Hide its layers' : 'Show its layers'" @click="emit('toggle')">
      <Icon name="chevronRight" :size="14" />
    </button>
    <span v-else class="ac-tree-toggle" />
    <button class="ac-tree-name" type="button" @click="emit('pick')">
      <Icon :name="icon" :size="14" />
      <span class="ac-truncate" :class="{ strong }">{{ label }}</span>
      <span v-if="meta" class="ac-tree-meta ac-truncate">{{ meta }}</span>
    </button>
  </div>
</template>
