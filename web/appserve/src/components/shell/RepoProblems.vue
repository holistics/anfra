<script setup lang="ts">
import { TriangleAlert } from '@lucide/vue';
import { onClickOutside, onKeyStroke } from '@vueuse/core';
import { ref } from 'vue';
import type * as server from '@/lib/server';
import InlineCode from './InlineCode.vue';

defineProps<{ problems: server.AmlProblem[] }>();

const open = ref(false);
const root = ref<HTMLElement>();
onClickOutside(root, () => { open.value = false; });
onKeyStroke('Escape', () => { open.value = false; });

function where (problem: server.AmlProblem): string {
  if (!problem.file) return '';
  return problem.line ? `${problem.file}:${problem.line}${problem.column ? `:${problem.column}` : ''}` : problem.file;
}
</script>

<template>
  <div ref="root" class="relative">
    <button
      type="button"
      class="flex h-7 items-center gap-1.5 rounded-[14px] bg-danger-secondary-default px-2.5 text-[12px] font-semibold text-danger-default [&_svg]:size-4 [&_svg]:shrink-0"
      data-testid="aml-badge"
      :aria-expanded="open"
      @click="open = !open"
    >
      <TriangleAlert />{{ problems.length === 1 ? '1 problem' : `${problems.length} problems` }}
    </button>
    <section
      v-if="open"
      class="absolute top-9 right-0 z-10 max-h-[60vh] w-[min(480px,90vw)] overflow-auto rounded-[10px] border border-default bg-background px-3.5 py-3 shadow-e2"
      role="alert"
      data-testid="aml-banner"
      data-dismissable-layer
    >
      <strong>The repo has {{ problems.length === 1 ? 'a problem' : `${problems.length} problems` }}.</strong>
      <span class="block text-placeholder">Queries may fail until it's fixed.</span>
      <ul class="mt-2 list-disc pl-[18px]">
        <li v-for="(problem, i) in problems" :key="i" class="mb-1">
          <InlineCode v-if="where(problem)">{{ where(problem) }}</InlineCode> {{ problem.message }}
        </li>
      </ul>
    </section>
  </div>
</template>
