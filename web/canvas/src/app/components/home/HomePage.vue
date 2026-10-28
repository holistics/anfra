<script setup lang="ts">
// The canvas's home, as a file browser: every board as a card, the ones opened last, and each
// project's boards and screens, with a search over all of them. A view lives in the page's address,
// so Back returns to it.
import { computed, ref } from 'vue';
import type { BoardDetails, BoardSummary } from '../../../vite/boards.ts';
import { useStore } from '../../context.ts';
import { listed, projects as allProjects, recentBoards, screenProject, screens as screensOf, viewHash } from '../../home.ts';
import type { HomeView } from '../../store.ts';
import Button from '../../ui/Button.vue';
import Field from '../../ui/Field.vue';
import Icon from '../../ui/Icon.vue';
import Select from '../../ui/Select.vue';
import BoardCard from './BoardCard.vue';
import BoardDialog from './BoardDialog.vue';
import DeleteBoardDialog from './DeleteBoardDialog.vue';

const store = useStore();
const { state } = store;
const view = computed<HomeView>(() => state.home ?? { kind: 'all' });
const query = ref('');
const order = ref<'updated' | 'name'>('updated');
const dialog = ref<{ slug?: string, initial: BoardDetails } | null>(null);
const recent = ref(recentBoards());

const projectList = computed(() => allProjects(state.boards, state.settings));
const boards = computed(() => listed(state.boards, state.settings, view.value, query.value, order.value, recent.value));
const shownScreens = computed(() => screensOf(state.settings, view.value, query.value));

const title = computed(() => {
  if (query.value.trim()) return `Results for “${query.value.trim()}”`;
  if (view.value.kind === 'recent') return 'Recents';
  if (view.value.kind === 'project') return view.value.id ?? 'Project';
  return 'All boards';
});
const caption = computed(() => {
  const n = boards.value.length;
  const b = `${n} ${n === 1 ? 'board' : 'boards'}`;
  if (view.value.kind === 'project' && !query.value.trim()) {
    const s = shownScreens.value.length;
    return `${b} · ${s} ${s === 1 ? 'screen' : 'screens'}`;
  }
  return b;
});

const go = (v: HomeView) => {
  query.value = '';
  location.hash = viewHash(v);
};
const isAt = (v: HomeView) => !query.value.trim() && view.value.kind === v.kind && (view.value.id ?? '') === (v.id ?? '');

function newBoard () {
  const v = view.value;
  dialog.value = {
    initial: {
      project: v.kind === 'project' ? v.id : '',
    },
  };
}
// Deleting a board: its confirm, then a note with Undo while the server keeps what went.
const deleting = ref<BoardSummary | null>(null);
const undoable = ref<{ slug: string, name: string, withFolder: boolean } | null>(null);
let undoTimer: ReturnType<typeof setTimeout> | undefined;
function deleted (b: BoardSummary, withFolder: boolean) {
  deleting.value = null;
  undoable.value = { slug: b.slug, name: b.name, withFolder };
  clearTimeout(undoTimer);
  undoTimer = setTimeout(() => (undoable.value = null), 15_000);
}
async function undo () {
  const u = undoable.value;
  if (!u) return;
  undoable.value = null;
  await store.restoreBoard(u.slug);
}

const edit = (b: BoardSummary) => {
  dialog.value = { slug: b.slug, initial: { name: b.name, project: b.project ?? '' } };
};

/** A screen's name and where it is: `DatasetList`, `datasets/`. */
const screenParts = (route: string) => {
  const path = route.slice((state.settings.prototypeRoute ?? '').length + 1);
  const i = path.lastIndexOf('/');
  return { name: path.slice(i + 1), folder: i > 0 ? path.slice(0, i + 1) : '' };
};
const pathOf = (route: string) => route.split(/[?#]/)[0];
const boardsShowing = (route: string) => state.boards.filter((b) => b.routes?.some((r) => pathOf(r) === route)).length;
</script>

<template>
  <div class="ac-home">
    <nav class="ac-home-nav ac-panel" aria-label="Boards">
      <Field v-model="query" icon="search" type="search" placeholder="Search boards and screens" aria-label="Search boards and screens" />
      <ul class="ac-home-links">
        <li>
          <button type="button" class="ac-home-link" :aria-current="isAt({ kind: 'all' }) ? 'page' : undefined" @click="go({ kind: 'all' })">
            <Icon name="grid" /><span class="ac-truncate">All boards</span><span class="ac-item-hint ac-num">{{ state.boards.length }}</span>
          </button>
        </li>
        <li>
          <button type="button" class="ac-home-link" :aria-current="isAt({ kind: 'recent' }) ? 'page' : undefined" @click="go({ kind: 'recent' })">
            <Icon name="clock" /><span class="ac-truncate">Recents</span>
          </button>
        </li>
      </ul>
      <template v-if="projectList.length">
        <p class="ac-home-group ac-caption">Projects</p>
        <ul class="ac-home-links">
          <li v-for="p in projectList" :key="p.name">
            <button type="button" class="ac-home-link" :aria-current="isAt({ kind: 'project', id: p.name }) ? 'page' : undefined" @click="go({ kind: 'project', id: p.name })">
              <Icon name="folder" /><span class="ac-truncate">{{ p.name }}</span><span class="ac-item-hint ac-num">{{ p.boards }}</span>
            </button>
          </li>
        </ul>
      </template>
    </nav>

    <section class="ac-home-main" :aria-label="title">
      <header class="ac-home-head">
        <div class="ac-home-title">
          <h1 class="ac-title ac-truncate">{{ title }}</h1>
          <p class="ac-caption">{{ caption }}</p>
        </div>
        <span class="ac-spacer" />
        <Select
          v-if="view.kind !== 'recent' || query"
          v-model="order"
          :options="[{ value: 'updated', label: 'Last edited' }, { value: 'name', label: 'Name' }]"
          label="Sort"
          look="ghost"
          :min-width="160"
        />
        <Button variant="primary" icon="plus" @click="newBoard">New board</Button>
      </header>

      <div v-if="shownScreens.length" class="ac-home-block">
        <h2 class="ac-heading">Screens</h2>
        <ul class="ac-screen-list">
          <li v-for="r in shownScreens" :key="r" class="ac-screen-row">
            <Icon name="screen" />
            <span class="ac-truncate"><span class="ac-label">{{ screenParts(r).name }}</span> <span class="ac-caption">{{ screenParts(r).folder }}</span></span>
            <span v-if="query.trim() && screenProject(r, state.settings.prototypeRoute ?? '')" class="ac-chip">{{ screenProject(r, state.settings.prototypeRoute ?? '') }}</span>
            <span class="ac-caption ac-num">{{ boardsShowing(r) ? `On ${boardsShowing(r)} ${boardsShowing(r) === 1 ? 'board' : 'boards'}` : 'On no board' }}</span>
            <a class="ac-btn ghost sm" :href="r" target="_blank" rel="noopener">Run it <Icon name="external" :size="14" /></a>
          </li>
        </ul>
      </div>

      <div class="ac-home-block">
        <h2 v-if="shownScreens.length" class="ac-heading">Boards</h2>
        <ul v-if="boards.length" class="ac-board-grid">
          <li v-for="b in boards" :key="b.slug">
            <BoardCard :board="b" @edit="edit(b)" @delete="deleting = b" />
          </li>
        </ul>
        <div v-else class="ac-start ac-home-empty">
          <p class="ac-title">{{ query.trim() ? 'Nothing found' : view.kind === 'recent' ? 'No boards opened yet' : 'No boards here yet' }}</p>
          <p class="ac-caption">
            {{ query.trim() ? 'Search looks in board names, projects and screens' : 'A board holds the screens of one feature: its happy path and its edge cases, side by side' }}
          </p>
          <Button v-if="!query.trim()" variant="primary" icon="plus" @click="newBoard">New board</Button>
        </div>
      </div>
    </section>

    <BoardDialog v-if="dialog" :slug="dialog.slug" :initial="dialog.initial" @close="dialog = null" />
    <DeleteBoardDialog v-if="deleting" :board="deleting" @close="deleting = null" @deleted="(withFolder) => deleted(deleting!, withFolder)" />
    <div v-if="undoable" class="ac-toast ac-home-toast" data-ui role="status">
      <span>{{ undoable.name }} deleted{{ undoable.withFolder ? ', with its screens' : '' }}</span>
      <Button variant="ghost" small @click="undo">Undo</Button>
    </div>
    <div v-else-if="state.toast" :key="state.toast" class="ac-toast ac-home-toast" data-ui role="status">{{ state.toast }}</div>
  </div>
</template>
