<script setup lang="ts">
// A board on the home page: its cover (a still of its first screen, else its map), its name, its
// project and when it last changed, its open comments, and ways to present it, change its details or
// delete it.
import { computed, ref } from 'vue';
import type { BoardSummary } from '../../../vite/boards.ts';
import { useStore } from '../../context.ts';
import { ago, boardProject } from '../../home.ts';
import Button from '../../ui/Button.vue';
import { vTip } from '../../ui/tooltip.ts';
import BoardMap from './BoardMap.vue';

const props = defineProps<{ board: BoardSummary }>();
const emit = defineEmits<{ edit: [], delete: [] }>();

const { state } = useStore();
const cover = ref(true);
const project = computed(() => boardProject(props.board, state.settings.prototypeRoute ?? ''));
const href = computed(() => `#${props.board.slug}`);
const present = () => {
  location.hash = `${props.board.slug}/present/${props.board.first}`;
};
</script>

<template>
  <article class="ac-board-card">
    <a class="ac-board-cover" :href="href" :aria-label="`Open ${board.name}`">
      <img v-if="cover && board.frames" :src="`/__canvas/api/thumb?board=${board.slug}&v=${board.updated}`" alt="" loading="lazy" @error="cover = false">
      <BoardMap v-else :map="board.map" />
    </a>
    <div class="ac-board-meta">
      <div class="ac-row">
        <a class="ac-board-name ac-truncate" :href="href">{{ board.name }}</a>
        <span v-if="board.open" v-tip="`${board.open} open ${board.open === 1 ? 'comment' : 'comments'}`" class="ac-count">{{ board.open }}</span>
      </div>
      <p class="ac-caption ac-truncate">
        {{ [project, `${board.frames} ${board.frames === 1 ? 'screen' : 'screens'}`, `Edited ${ago(board.updated).toLowerCase()}`].filter(Boolean).join(' · ') }}
      </p>
      <div class="ac-row ac-board-foot">
        <span class="ac-spacer" />
        <Button v-if="board.first" variant="ghost" small icon="present" label="Present" @click="present" />
        <Button variant="ghost" small icon="pencil" label="Edit details" @click="emit('edit')" />
        <Button variant="ghost" small icon="trash" label="Delete board" @click="emit('delete')" />
      </div>
    </div>
  </article>
</template>
