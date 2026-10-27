<script setup lang="ts">
// A text field: one line, or several with `rows`. A prefix (W, px) or an icon can sit inside its
// frame; the frame carries the edge, hover and focus, the input inside has none. `small` is 24px
// for the board's own labels; `bare` has no frame, where the field is the whole surface (the command
// palette's search). Bind it with v-model, or with `value` and a change listener: class, style and
// data- attributes go to the frame, every other attribute and listener to the input itself.
import { ref, useAttrs } from 'vue';
import Icon, { type IconName } from './Icon.vue';

defineOptions({ inheritAttrs: false });
defineProps<{
  modelValue?: string | number;
  rows?: number;
  prefix?: string;
  icon?: IconName;
  mono?: boolean;
  small?: boolean;
  bare?: boolean;
}>();
const emit = defineEmits<{ 'update:modelValue': [string] }>();

const attrs = useAttrs();
const split = () => {
  const frame: Record<string, unknown> = {};
  const input: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(attrs)) (k === 'class' || k === 'style' || k.startsWith('data-') ? frame : input)[k] = v;
  return { frame, input };
};

const input = ref<HTMLInputElement | HTMLTextAreaElement>();
/** A press on the frame, around the input, still puts the caret in it. */
function onFrame (e: PointerEvent) {
  if (e.target !== input.value) {
    e.preventDefault();
    input.value?.focus();
  }
}
defineExpose({
  focus: () => input.value?.focus(),
  select: () => input.value?.select(),
  el: input,
});
</script>

<template>
  <span class="ac-field" :class="{ mono, sm: small, bare, multi: !!rows }" v-bind="split().frame" @pointerdown="onFrame">
    <Icon v-if="icon" :name="icon" :size="small ? 14 : 16" />
    <span v-else-if="prefix" class="ac-field-prefix">{{ prefix }}</span>
    <textarea
      v-if="rows"
      ref="input"
      :rows="rows"
      v-bind="split().input"
      :value="modelValue ?? (attrs.value as string)"
      @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
    />
    <input
      v-else
      ref="input"
      v-bind="split().input"
      :value="modelValue ?? (attrs.value as string)"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
  </span>
</template>
