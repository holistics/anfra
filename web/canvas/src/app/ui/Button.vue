<script setup lang="ts">
// The canvas's one button: outline (the default) for most actions, primary for the one main action
// of a form, ghost in bars and beside a heading. 32px, or 24px with `small` on the board's own
// labels. With no text it is an icon button, named by `label`. Its tooltip says `tip` (an icon
// button's label by default) and its shortcut, `keys`. A tool that is on takes `aria-pressed`.
import Icon, { type IconName } from './Icon.vue';
import { vTip } from './tooltip.ts';

withDefaults(defineProps<{
  variant?: 'outline' | 'primary' | 'ghost';
  icon?: IconName;
  label?: string;
  tip?: string;
  keys?: string;
  small?: boolean;
  type?: 'button' | 'submit';
}>(), { variant: 'outline', icon: undefined, label: undefined, tip: undefined, keys: undefined, type: 'button' });
</script>

<template>
  <button
    v-tip="{ text: tip ?? ($slots.default ? '' : label ?? ''), keys }"
    class="ac-btn"
    :class="[variant, { icon: !$slots.default, sm: small }]"
    :type="type"
    :aria-label="$slots.default ? undefined : label"
  >
    <Icon v-if="icon" :name="icon" :size="small ? 14 : 16" />
    <slot />
  </button>
</template>
