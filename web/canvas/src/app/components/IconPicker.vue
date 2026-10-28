<script setup lang="ts">
// A prop that names one of the kit's icons: the icon and its name, and under them, when open, every
// icon drawn small with a search, as the kit's own icon component draws it.
import { computed, ref } from 'vue';
import { useStore } from '../context.ts';
import Field from '../ui/Field.vue';
import Icon from '../ui/Icon.vue';
import { vTip } from '../ui/tooltip.ts';

defineProps<{ modelValue: string, label?: string }>();
const emit = defineEmits<{ 'update:modelValue': [string] }>();

const store = useStore();
const open = ref(false);
const query = ref('');
const names = computed(() => store.state.settings.icons?.names ?? []);
const shown = computed(() => {
  const q = query.value.trim().toLowerCase();
  return q ? names.value.filter((n) => n.toLowerCase().includes(q)) : names.value;
});

function choose (name: string) {
  emit('update:modelValue', name);
  open.value = false;
  query.value = '';
}
</script>

<template>
  <div class="ac-icon-picker">
    <button type="button" class="ac-select" :aria-expanded="open" :aria-label="label" @click="open = !open">
      <span v-if="modelValue && store.previews" class="ac-icon-cell" aria-hidden="true" inert>
        <component :is="store.previews" :id="`icon:${modelValue}`" />
      </span>
      <span class="ac-truncate" :class="{ placeholder: !modelValue }">{{ modelValue || 'None' }}</span>
      <Icon name="chevron" :size="16" class="ac-select-chevron" />
    </button>
    <div v-if="open" class="ac-icon-panel">
      <Field v-model="query" icon="search" type="search" placeholder="Search icons" aria-label="Search icons" />
      <div class="ac-icon-grid" role="listbox" :aria-label="label">
        <button v-if="modelValue" v-tip="'No icon'" type="button" role="option" class="ac-icon-choice" :aria-selected="false" @click="choose('')">
          <Icon name="x" :size="16" />
        </button>
        <button
          v-for="n in shown"
          :key="n"
          v-tip="n"
          type="button"
          role="option"
          class="ac-icon-choice"
          :aria-selected="n === modelValue"
          @click="choose(n)"
        >
          <span class="ac-icon-cell" inert><component :is="store.previews" :id="`icon:${n}`" /></span>
        </button>
      </div>
      <p v-if="!shown.length" class="ac-caption">No icon matches</p>
    </div>
  </div>
</template>
