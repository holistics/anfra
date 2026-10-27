<script setup lang="ts">
// On or off, at once: a property that is set or not, like a border.
defineProps<{ modelValue: boolean, label: string }>();
const emit = defineEmits<{ 'update:modelValue': [boolean] }>();
</script>

<template>
  <button class="ac-switch" type="button" role="switch" :aria-checked="modelValue" :aria-label="label" @click="emit('update:modelValue', !modelValue)" />
</template>
