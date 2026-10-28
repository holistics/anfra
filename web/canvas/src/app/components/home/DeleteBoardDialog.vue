<script setup lang="ts">
// Deleting a board, from its card on the home page. It says what goes from the repo and whether each
// was committed, since a file never committed can't come back with git. The folder of the board's
// screens is offered only when every screen of the board is in it and no other board shows one;
// it is left unticked. The home page can undo the delete until the canvas's server stops.
import { computed, onMounted, ref } from 'vue';
import type { BoardFiles, GitState } from '../../../vite/board-files.ts';
import type { BoardSummary } from '../../../vite/boards.ts';
import { useStore } from '../../context.ts';
import Button from '../../ui/Button.vue';
import Switch from '../../ui/Switch.vue';

const props = defineProps<{ board: BoardSummary }>();
const emit = defineEmits<{ close: [], deleted: [withFolder: boolean] }>();

const store = useStore();
const files = ref<BoardFiles>();
const withFolder = ref(false);
const busy = ref(false);
onMounted(async () => {
  files.value = await store.boardFiles(props.board.slug);
});

const GIT: Record<GitState, string> = {
  committed: 'Committed: git can bring it back',
  changed: 'Changed since the last commit: git brings back only the committed version',
  new: 'Never committed: git can\'t bring it back',
};
const risky = computed(() => files.value?.board.git !== 'committed' || (withFolder.value && files.value?.folder?.git !== 'committed'));

async function confirm () {
  busy.value = true;
  const ok = await store.deleteBoard(props.board.slug, withFolder.value);
  busy.value = false;
  if (ok) emit('deleted', withFolder.value);
}
</script>

<template>
  <div class="ac-command-scrim" data-ui @pointerdown.self="emit('close')">
    <form class="ac-board-dialog ac-popover form ac-pop-in delete-board-dialog" role="alertdialog" :aria-label="`Delete ${board.name}`" @submit.prevent="confirm" @keydown.esc="emit('close')">
      <p class="ac-heading">Delete {{ board.name }}?</p>
      <p v-if="!files" class="ac-caption">Reading what it would delete…</p>
      <template v-else>
        <div class="ac-board-field">
          <span class="ac-label">Deletes from the repo</span>
          <p class="ac-delete-item"><code class="ac-mono">{{ files.board.path }}</code><span class="ac-caption">{{ GIT[files.board.git] }}</span></p>
          <p class="ac-caption">Its screens stay unless you delete their folder too.</p>
        </div>
        <label v-if="files.folder" class="ac-board-field ac-delete-folder">
          <span class="ac-row">
            <Switch v-model="withFolder" label="Also delete its screens' folder" />
            <span class="ac-label">Also delete its screens' folder</span>
          </span>
          <span class="ac-delete-item"><code class="ac-mono">{{ files.folder.path }}/</code><span class="ac-caption">{{ files.folder.files }} {{ files.folder.files === 1 ? 'file' : 'files' }} · {{ GIT[files.folder.git] }}</span></span>
        </label>
        <p class="ac-caption">{{ risky ? 'Undo on the home page brings it back until the canvas stops; after that it is gone.' : 'Undo on the home page brings it back; git can too.' }}</p>
      </template>
      <div class="ac-row ac-board-actions">
        <span class="ac-spacer" />
        <Button variant="ghost" @click="emit('close')">Cancel</Button>
        <Button variant="primary" type="submit" class="danger" :disabled="!files || busy">Delete board</Button>
      </div>
    </form>
  </div>
</template>
