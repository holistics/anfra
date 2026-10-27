<script setup lang="ts">
// The properties of what is selected: a picked element (its source, text and classes, written back
// to its file), or a frame, section, note or arrow (on the board).
import { computed, ref, watch } from 'vue';
import {
  FRAME_SIZES, addSection, arrange, bounds, duplicate, elementMarkdown, isShape, update,
  type Arrow, type BoardObject, type Frame, type Note, type Section, type Shape,
} from '../../model/index.ts';
import { useStore } from '../context.ts';
import Icon from './Icon.vue';

const store = useStore();
const { state, board } = store;

const selected = computed<BoardObject[]>(() => {
  void board.value;
  return store.selected();
});
const one = computed(() => (selected.value.length === 1 ? selected.value[0] : undefined));
const picked = computed(() => state.picked);

// --- The picked element's source -------------------------------------------------------------------

const src = ref<{ tag: string, opening: string, classes?: string, text?: string } | undefined>();
const classes = ref('');
const text = ref('');
watch(() => [picked.value?.info.ref.source, state.layout] as const, async ([loc]) => {
  if (!loc) {
    src.value = undefined;
    return;
  }
  const s = await store.source(loc);
  if (picked.value?.info.ref.source !== loc) return;
  src.value = s;
  if (document.activeElement?.closest('.ac-inspector') == null) {
    classes.value = s?.classes ?? '';
    text.value = s?.text ?? '';
  }
}, { immediate: true });

const openInEditor = (loc: string) => fetch(`/__open-in-editor?file=${encodeURIComponent(loc)}`);

async function saveText () {
  const p = picked.value;
  if (!p?.info.ref.source || !src.value?.text || text.value.trim() === src.value.text) return;
  const r = await fetch('/__canvas/api/edit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ loc: p.info.ref.source, edit: { kind: 'text', from: src.value.text, to: text.value } }),
  });
  store.flash(r.ok ? 'Saved to the file' : ((await r.json()).message ?? 'Couldn\'t write it'));
}

function commentOnPicked () {
  const p = picked.value;
  if (!p || !board.value) return;
  const f = board.value.frames.find((x) => x.id === p.frame);
  if (!f) return;
  state.composer = { anchor: { type: 'element', ...p.info.ref }, at: store.toScreen({ x: f.x + p.info.rect.x + p.info.rect.width, y: f.y + p.info.rect.y }), body: '' };
}

// --- Board objects ---------------------------------------------------------------------------------

const set = <T extends BoardObject>(patch: Partial<T>) => {
  if (board.value && one.value) store.commit(update<T>(board.value, one.value.id, patch as never));
};
const num = (e: Event) => Math.round(Number((e.target as HTMLInputElement).value));
const str = (e: Event) => (e.target as HTMLInputElement).value;
const scenarios = computed(() => [...Object.keys(state.settings.scenarios), 'live']);
const shapesSelected = computed(() => selected.value.filter(isShape) as Shape[]);

function wrap () {
  if (!board.value) return;
  const box = bounds(shapesSelected.value)!;
  const r = addSection(board.value, { name: 'Section', x: box.x - 80, y: box.y - 120, width: box.width + 160, height: box.height + 200 });
  store.commit({ ...r.board, sections: [r.section, ...r.board.sections.filter((s) => s.id !== r.section.id)] });
  state.selection = [r.section.id];
}

function copyShapes () {
  if (!board.value) return;
  const r = duplicate(board.value, state.selection);
  store.commit(r.board);
  state.selection = r.ids;
}

const frameOf = (o: BoardObject | undefined) => (o?.kind === 'frame' ? (o as Frame) : undefined);
const mcpCommand = computed(() => `claude mcp add --transport http anfra-canvas ${location.origin}${state.settings.mcp}`);
</script>

<template>
  <aside class="ac-panel ac-inspector" aria-label="Properties">
    <!-- A picked element -->
    <template v-if="picked">
      <div class="ac-section ac-stack">
        <div class="ac-row">
          <Icon name="code" :size="14" />
          <span class="ac-heading">{{ picked.info.ref.component ? `<${picked.info.ref.component}>` : `<${picked.info.ref.tag}>` }}</span>
          <span class="ac-spacer" />
          <button class="ac-btn ghost icon small" title="Deselect (Esc)" aria-label="Deselect" @click="state.picked = null"><Icon name="x" :size="14" /></button>
        </div>
        <template v-if="picked.info.ref.source">
          <button class="ac-source ac-mono" type="button" title="Open in your editor" @click="openInEditor(picked.info.ref.source)">
            {{ picked.info.ref.source }} <Icon name="external" :size="12" />
          </button>
          <pre v-if="src" class="ac-snippet ac-mono">{{ src.opening }}</pre>
        </template>
        <p v-else class="ac-caption">Drawn at run time, not written in a template: only a comment can change it</p>
      </div>
      <div class="ac-section ac-stack">
        <div class="ac-row">
          <button class="ac-btn primary small" type="button" @click="commentOnPicked"><Icon name="comment" :size="14" /> Comment</button>
          <button class="ac-btn small" type="button" @click="board && store.copy(elementMarkdown(board, picked.info.ref), 'Element copied as Markdown')"><Icon name="copy" :size="14" /> Markdown</button>
        </div>
      </div>
      <div v-if="src?.text !== undefined" class="ac-section ac-stack">
        <label class="ac-label" for="ac-text">Text</label>
        <input id="ac-text" v-model="text" class="ac-field" @keydown.enter="saveText" @blur="saveText" />
        <p class="ac-caption">Or double-click it on the screen. Saved to the file</p>
      </div>
      <div v-if="picked.info.ref.source" class="ac-section ac-stack">
        <label class="ac-label" for="ac-classes">Classes</label>
        <textarea id="ac-classes" v-model="classes" class="ac-field mono" rows="3" @keydown.meta.enter="store.editClasses(picked.info.ref, classes)" />
        <div class="ac-row">
          <span class="ac-caption">Static classes, saved to the file</span>
          <span class="ac-spacer" />
          <button class="ac-btn small" type="button" :disabled="classes === (src?.classes ?? '')" @click="store.editClasses(picked.info.ref, classes)">Apply</button>
        </div>
      </div>
      <div class="ac-section">
        <p class="ac-label">Computed</p>
        <dl class="ac-props">
          <template v-for="(v, k) in picked.info.style" :key="k">
            <dt>{{ k }}</dt>
            <dd class="ac-mono">{{ v }}</dd>
          </template>
        </dl>
      </div>
    </template>

    <!-- A frame -->
    <template v-else-if="frameOf(one)">
      <div class="ac-section ac-stack">
        <p class="ac-heading">Frame</p>
        <label class="ac-stack"><span class="ac-label">Name</span><input class="ac-field" :value="frameOf(one)!.name" @change="set<Frame>({ name: str($event) })" /></label>
        <label class="ac-stack">
          <span class="ac-label">Route</span>
          <input class="ac-field mono" :value="frameOf(one)!.route" list="ac-routes-inspector" @change="set<Frame>({ route: str($event) })" />
          <datalist id="ac-routes-inspector"><option v-for="r in [...state.settings.prototypes, ...state.settings.routes]" :key="r" :value="r" /></datalist>
        </label>
        <p v-if="state.routes[one!.id] && state.routes[one!.id] !== frameOf(one)!.route" class="ac-caption">
          Now at <span class="ac-mono">{{ state.routes[one!.id] }}</span>
          <button class="ac-btn ghost small" type="button" @click="set<Frame>({ route: state.routes[one!.id] })">Keep this route</button>
        </p>
        <label class="ac-stack">
          <span class="ac-label">Scenario</span>
          <select class="ac-field" :value="frameOf(one)!.scenario ?? 'default'" @change="set<Frame>({ scenario: str($event) })">
            <option v-for="s in scenarios" :key="s" :value="s">{{ state.settings.scenarios[s]?.label ?? (s === 'live' ? 'Live backend' : s) }}</option>
          </select>
        </label>
        <p v-if="state.settings.scenarios[frameOf(one)!.scenario ?? 'default']?.description" class="ac-caption">
          {{ state.settings.scenarios[frameOf(one)!.scenario ?? 'default'].description }}
        </p>
      </div>
      <div class="ac-section ac-stack">
        <span class="ac-label">Size</span>
        <div class="ac-row ac-wrap">
          <button
            v-for="s in FRAME_SIZES"
            :key="s.name"
            class="ac-btn small"
            type="button"
            :aria-pressed="frameOf(one)!.width === s.width && frameOf(one)!.height === s.height"
            @click="set<Frame>({ width: s.width, height: s.height })"
          >
            {{ s.name }}
          </button>
        </div>
        <div class="ac-row">
          <label class="ac-row"><span class="ac-caption">W</span><input class="ac-field" type="number" :value="frameOf(one)!.width" @change="set<Frame>({ width: num($event) })" /></label>
          <label class="ac-row"><span class="ac-caption">H</span><input class="ac-field" type="number" :value="frameOf(one)!.height" @change="set<Frame>({ height: num($event) })" /></label>
        </div>
      </div>
    </template>

    <!-- A section, note or arrow -->
    <div v-else-if="one?.kind === 'section'" class="ac-section ac-stack">
      <p class="ac-heading">Section</p>
      <label class="ac-stack"><span class="ac-label">Name</span><input class="ac-field" :value="(one as Section).name" @change="set<Section>({ name: str($event) })" /></label>
      <p class="ac-caption">Moving a section moves what lies inside it</p>
    </div>
    <div v-else-if="one?.kind === 'note'" class="ac-section ac-stack">
      <p class="ac-heading">Note</p>
      <textarea class="ac-field" rows="5" :value="(one as Note).text" @change="set<Note>({ text: str($event) })" />
    </div>
    <div v-else-if="one?.kind === 'arrow'" class="ac-section ac-stack">
      <p class="ac-heading">Arrow</p>
      <label class="ac-stack"><span class="ac-label">Label</span><input class="ac-field" :value="(one as Arrow).label ?? ''" placeholder="Click Invite" @change="set<Arrow>({ label: str($event) || undefined })" /></label>
    </div>

    <!-- Several -->
    <div v-else-if="shapesSelected.length > 1" class="ac-section ac-stack">
      <p class="ac-heading">{{ shapesSelected.length }} selected</p>
      <div class="ac-row">
        <button class="ac-btn icon small" title="Align left edges" aria-label="Align left edges" @click="board && store.commit(arrange(board, state.selection, 'left'))"><Icon name="alignLeft" :size="14" /></button>
        <button class="ac-btn icon small" title="Align top edges" aria-label="Align top edges" @click="board && store.commit(arrange(board, state.selection, 'top'))"><Icon name="alignTop" :size="14" /></button>
        <button class="ac-btn icon small" title="Lay out in a row" aria-label="Lay out in a row" @click="board && store.commit(arrange(board, state.selection, 'row'))"><Icon name="row" :size="14" /></button>
        <button class="ac-btn icon small" title="Lay out in a column" aria-label="Lay out in a column" @click="board && store.commit(arrange(board, state.selection, 'column'))"><Icon name="column" :size="14" /></button>
      </div>
      <button class="ac-btn small" @click="wrap"><Icon name="section" :size="14" /> Put in a section</button>
    </div>

    <!-- Nothing -->
    <template v-else>
      <div class="ac-section ac-stack">
        <p class="ac-heading">Connect Claude</p>
        <p class="ac-caption">Claude reads the comments, changes the code they point at, and resolves them. Run this once in the repo:</p>
        <pre class="ac-snippet ac-mono">{{ mcpCommand }}</pre>
        <button class="ac-btn small" type="button" @click="store.copy(mcpCommand, 'Command copied')"><Icon name="copy" :size="14" /> Copy</button>
        <p class="ac-caption">Then ask: "Handle the open feedback on the canvas". Or copy the comments as Markdown and paste them anywhere.</p>
      </div>
      <div class="ac-section">
        <p class="ac-label">Shortcuts</p>
        <dl class="ac-props">
          <dt>Select, Pan, Use</dt><dd><span class="ac-kbd">V</span> <span class="ac-kbd">H</span> <span class="ac-kbd">P</span></dd>
          <dt>Comment</dt><dd><span class="ac-kbd">C</span></dd>
          <dt>Frame, Section, Note, Arrow</dt><dd><span class="ac-kbd">F</span> <span class="ac-kbd">S</span> <span class="ac-kbd">N</span> <span class="ac-kbd">A</span></dd>
          <dt>Pan</dt><dd>Hold <span class="ac-kbd">Space</span> or scroll</dd>
          <dt>Zoom</dt><dd><span class="ac-kbd">⌘</span> + scroll, <span class="ac-kbd">⇧1</span> fit</dd>
          <dt>Edit text</dt><dd>Double-click it</dd>
          <dt>Duplicate, Delete</dt><dd><span class="ac-kbd">⌘D</span> <span class="ac-kbd">⌫</span></dd>
          <dt>Undo, Redo</dt><dd><span class="ac-kbd">⌘Z</span> <span class="ac-kbd">⇧⌘Z</span></dd>
        </dl>
      </div>
    </template>

    <div v-if="shapesSelected.length && !picked" class="ac-section ac-row">
      <button class="ac-btn small" @click="copyShapes"><Icon name="copy" :size="14" /> Duplicate</button>
      <button class="ac-btn small" @click="store.removeSelection()"><Icon name="trash" :size="14" /> Delete</button>
    </div>
  </aside>
</template>

<style>
.ac-source {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  width: fit-content;
  max-width: 100%;
  padding: 0;
  border: 0;
  background: none;
  color: var(--ac-violet-text);
  cursor: pointer;
  overflow-wrap: anywhere;
  text-align: left;
}
.ac-source:hover { text-decoration: underline; }
.ac-snippet {
  margin: 0;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--ac-sunken);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 160px;
  overflow: auto;
}
.ac-props { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; margin: 0; }
.ac-props dt { color: var(--ac-ink-muted); font-size: 12px; line-height: 20px; }
.ac-props dd { margin: 0; text-align: right; overflow-wrap: anywhere; font-size: 12px; line-height: 20px; }
.ac-wrap { flex-wrap: wrap; }
</style>
