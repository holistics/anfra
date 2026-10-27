<script setup lang="ts">
// The properties of what is selected: a picked element (its source, text and classes, written back
// to its file), or a frame, section, note or arrow (on the board).
import { computed, ref, watch } from 'vue';
import {
  FRAME_SIZES, addSection, arrange, bounds, duplicate, elementMarkdown, isShape, update,
  type Arrow, type BoardObject, type Frame, type Note, type Section, type Shape,
} from '../../model/index.ts';
import { scenarioOptions, useStore } from '../context.ts';
import Icon from './Icon.vue';
import Select from './Select.vue';

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

/** A source location split for showing: the file and line first, its folder under it. */
const where = (loc: string) => {
  const i = loc.lastIndexOf('/');
  return { file: loc.slice(i + 1), dir: i > 0 ? loc.slice(0, i) : '' };
};
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
const options = computed(() => scenarioOptions(store));
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
      <div class="ac-section">
        <div class="ac-section-head">
          <Icon name="code" class="ac-inspector-icon" />
          <span class="ac-heading ac-truncate">{{ picked.info.ref.component ? `<${picked.info.ref.component}>` : `<${picked.info.ref.tag}>` }}</span>
          <span class="ac-spacer" />
          <button class="ac-btn ghost icon" type="button" title="Deselect (Esc)" aria-label="Deselect the element" @click="state.picked = null"><Icon name="x" /></button>
        </div>
        <template v-if="picked.info.ref.source">
          <button class="ac-source" type="button" :title="`Open ${picked.info.ref.source} in your editor`" @click="openInEditor(picked.info.ref.source)">
            <span class="ac-source-file ac-mono ac-truncate">{{ where(picked.info.ref.source).file }}</span>
            <Icon name="external" :size="14" />
            <span class="ac-source-dir ac-caption ac-truncate"><bdi>{{ where(picked.info.ref.source).dir }}</bdi></span>
          </button>
          <pre v-if="src" class="ac-snippet ac-mono">{{ src.opening }}</pre>
        </template>
        <p v-else class="ac-caption">Drawn at run time, not written in a template: only a comment can change it</p>
        <div class="ac-row">
          <button class="ac-btn" type="button" @click="commentOnPicked"><Icon name="comment" /> Comment</button>
          <button class="ac-btn" type="button" @click="board && store.copy(elementMarkdown(board, picked.info.ref), 'Element copied as Markdown')"><Icon name="copy" /> Markdown</button>
        </div>
      </div>
      <div v-if="src?.text !== undefined" class="ac-section">
        <div class="ac-stack">
          <label class="ac-label" for="ac-text">Text</label>
          <input id="ac-text" v-model="text" class="ac-field" @keydown.enter="saveText" @blur="saveText" />
          <p class="ac-caption">Saved to the file. Or double-click the text on the screen</p>
        </div>
      </div>
      <div v-if="picked.info.ref.source" class="ac-section">
        <div class="ac-stack">
          <label class="ac-label" for="ac-classes">Classes</label>
          <textarea id="ac-classes" v-model="classes" class="ac-field mono" rows="3" @keydown.meta.enter="store.editClasses(picked.info.ref, classes)" />
          <div class="ac-row">
            <span class="ac-caption ac-spacer">Static classes, saved to the file</span>
            <button class="ac-btn" type="button" :disabled="classes === (src?.classes ?? '')" @click="store.editClasses(picked.info.ref, classes)">Apply</button>
          </div>
        </div>
      </div>
      <div class="ac-section">
        <p class="ac-heading">Computed</p>
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
      <div class="ac-section">
        <div class="ac-section-head">
          <Icon name="frame" class="ac-inspector-icon" />
          <span class="ac-heading">Frame</span>
          <span class="ac-spacer" />
          <button class="ac-btn ghost" type="button" title="Play the flow from this frame" @click="store.play(one!.id)"><Icon name="play" /> Play</button>
        </div>
        <label class="ac-stack"><span class="ac-label">Name</span><input class="ac-field" :value="frameOf(one)!.name" @change="set<Frame>({ name: str($event) })" /></label>
        <label class="ac-stack">
          <span class="ac-label">Route</span>
          <input class="ac-field mono" :value="frameOf(one)!.route" list="ac-routes-inspector" @change="set<Frame>({ route: str($event) })" />
          <datalist id="ac-routes-inspector"><option v-for="r in [...state.settings.prototypes, ...state.settings.routes]" :key="r" :value="r" /></datalist>
        </label>
        <div v-if="state.routes[one!.id] && state.routes[one!.id] !== frameOf(one)!.route" class="ac-moved">
          <p class="ac-caption">Now at <span class="ac-mono">{{ state.routes[one!.id] }}</span></p>
          <button class="ac-btn" type="button" @click="set<Frame>({ route: state.routes[one!.id] })">Keep this route</button>
        </div>
        <div class="ac-stack">
          <span class="ac-label" :id="`ac-scenario-${one!.id}`">Scenario</span>
          <Select
            :model-value="frameOf(one)!.scenario ?? 'default'"
            :options="options"
            label="Scenario"
            :min-width="264"
            @update:model-value="(v) => set<Frame>({ scenario: v })"
          />
        </div>
      </div>
      <div class="ac-section">
        <p class="ac-heading">Size</p>
        <div class="ac-seg fill" role="group" aria-label="Size">
          <button
            v-for="s in FRAME_SIZES"
            :key="s.name"
            type="button"
            :aria-pressed="frameOf(one)!.width === s.width && frameOf(one)!.height === s.height"
            :title="`${s.width} × ${s.height}`"
            @click="set<Frame>({ width: s.width, height: s.height })"
          >
            {{ s.name }}
          </button>
        </div>
        <div class="ac-pair">
          <label class="ac-affix"><span>W</span><input class="ac-field" type="number" aria-label="Width" :value="frameOf(one)!.width" @change="set<Frame>({ width: num($event) })" /></label>
          <label class="ac-affix"><span>H</span><input class="ac-field" type="number" aria-label="Height" :value="frameOf(one)!.height" @change="set<Frame>({ height: num($event) })" /></label>
        </div>
      </div>
    </template>

    <!-- A section, note or arrow -->
    <div v-else-if="one?.kind === 'section'" class="ac-section">
      <div class="ac-section-head"><Icon name="section" class="ac-inspector-icon" /><span class="ac-heading">Section</span></div>
      <label class="ac-stack"><span class="ac-label">Name</span><input class="ac-field" :value="(one as Section).name" @change="set<Section>({ name: str($event) })" /></label>
      <p class="ac-caption">Moving a section moves what lies inside it</p>
    </div>
    <div v-else-if="one?.kind === 'note'" class="ac-section">
      <div class="ac-section-head"><Icon name="note" class="ac-inspector-icon" /><span class="ac-heading">Note</span></div>
      <textarea class="ac-field" rows="6" aria-label="Note" :value="(one as Note).text" @change="set<Note>({ text: str($event) })" />
    </div>
    <div v-else-if="one?.kind === 'arrow'" class="ac-section">
      <div class="ac-section-head"><Icon name="arrow" class="ac-inspector-icon" /><span class="ac-heading">Arrow</span></div>
      <label class="ac-stack"><span class="ac-label">Label</span><input class="ac-field" :value="(one as Arrow).label ?? ''" placeholder="What the person does, like Invite" @change="set<Arrow>({ label: str($event) || undefined })" /></label>
      <p class="ac-caption">The label names this way on when the flow is played</p>
    </div>

    <!-- Several -->
    <div v-else-if="shapesSelected.length > 1" class="ac-section">
      <p class="ac-heading"><span class="ac-num">{{ shapesSelected.length }}</span> selected</p>
      <div class="ac-row ac-row-tight">
        <button class="ac-btn icon" type="button" title="Align left edges" aria-label="Align left edges" @click="board && store.commit(arrange(board, state.selection, 'left'))"><Icon name="alignLeft" /></button>
        <button class="ac-btn icon" type="button" title="Align top edges" aria-label="Align top edges" @click="board && store.commit(arrange(board, state.selection, 'top'))"><Icon name="alignTop" /></button>
        <button class="ac-btn icon" type="button" title="Lay out in a row" aria-label="Lay out in a row" @click="board && store.commit(arrange(board, state.selection, 'row'))"><Icon name="row" /></button>
        <button class="ac-btn icon" type="button" title="Lay out in a column" aria-label="Lay out in a column" @click="board && store.commit(arrange(board, state.selection, 'column'))"><Icon name="column" /></button>
      </div>
      <button class="ac-btn ac-fit" type="button" @click="wrap"><Icon name="section" /> Put in a section</button>
    </div>

    <!-- Nothing -->
    <template v-else>
      <div class="ac-section">
        <div class="ac-section-head"><Icon name="plug" class="ac-inspector-icon" /><span class="ac-heading">Connect Claude</span></div>
        <p class="ac-caption">Claude reads the comments, changes the code they point at, and resolves them. Run this once in the repo:</p>
        <div class="ac-command">
          <pre class="ac-snippet ac-mono">{{ mcpCommand }}</pre>
          <button class="ac-btn ghost icon" type="button" title="Copy the command" aria-label="Copy the command" @click="store.copy(mcpCommand, 'Command copied')"><Icon name="copy" /></button>
        </div>
        <p class="ac-caption">Then ask Claude to handle the open feedback on the canvas. Or copy the comments as Markdown and paste them anywhere</p>
      </div>
      <div class="ac-section">
        <p class="ac-heading">Shortcuts</p>
        <dl class="ac-keys">
          <div><dt>Select, pan, use the app</dt><dd><kbd class="ac-kbd">V</kbd><kbd class="ac-kbd">H</kbd><kbd class="ac-kbd">P</kbd></dd></div>
          <div><dt>Comment</dt><dd><kbd class="ac-kbd">C</kbd></dd></div>
          <div><dt>Frame, section, note, arrow</dt><dd><kbd class="ac-kbd">F</kbd><kbd class="ac-kbd">S</kbd><kbd class="ac-kbd">N</kbd><kbd class="ac-kbd">A</kbd></dd></div>
          <div><dt>Pan</dt><dd><kbd class="ac-kbd">Space</kbd> or scroll</dd></div>
          <div><dt>Zoom</dt><dd><kbd class="ac-kbd">⌘</kbd> scroll</dd></div>
          <div><dt>Fit, zoom to selection</dt><dd><kbd class="ac-kbd">⇧1</kbd><kbd class="ac-kbd">⇧2</kbd></dd></div>
          <div><dt>Edit text</dt><dd>Double-click</dd></div>
          <div><dt>Duplicate, delete</dt><dd><kbd class="ac-kbd">⌘D</kbd><kbd class="ac-kbd">⌫</kbd></dd></div>
          <div><dt>Undo, redo</dt><dd><kbd class="ac-kbd">⌘Z</kbd><kbd class="ac-kbd">⇧⌘Z</kbd></dd></div>
          <div><dt>Playing: next, back, stop</dt><dd><kbd class="ac-kbd">1</kbd><kbd class="ac-kbd">←</kbd><kbd class="ac-kbd">Esc</kbd></dd></div>
        </dl>
      </div>
    </template>

    <div v-if="shapesSelected.length && !picked" class="ac-section">
      <div class="ac-row">
        <button class="ac-btn" type="button" @click="copyShapes"><Icon name="copy" /> Duplicate</button>
        <button class="ac-btn" type="button" @click="store.removeSelection()"><Icon name="trash" /> Delete</button>
      </div>
    </div>
  </aside>
</template>

<style>
.ac-inspector-icon { flex-shrink: 0; color: var(--ac-icon-weak); }
.ac-source {
  display: grid;
  grid-template-columns: minmax(0, auto) 14px;
  align-items: center;
  column-gap: 6px;
  width: fit-content;
  max-width: 100%;
  padding: 0;
  border: 0;
  background: none;
  color: var(--ac-violet-text);
  text-align: left;
  cursor: pointer;
}
.ac-source-file { font-size: 14px; line-height: 20px; }
.ac-source > svg { color: var(--ac-violet-icon); }
.ac-source-dir { grid-column: 1 / -1; direction: rtl; text-align: left; color: var(--ac-ink-muted); }
@media (hover: hover) and (pointer: fine) {
  .ac-source:hover > .ac-source-file { text-decoration: underline; text-underline-offset: 3px; }
}
.ac-snippet {
  margin: 0;
  padding: 8px 10px;
  border-radius: 10px;
  background: var(--ac-sunken);
  box-shadow: inset 0 0 0 1px var(--ac-line-weak);
  color: var(--ac-ink-weak);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 160px;
  overflow: auto;
}
.ac-command { position: relative; }
.ac-command > .ac-snippet { padding-right: 40px; }
.ac-command > .ac-btn { position: absolute; top: 0; right: 0; }
.ac-moved { display: grid; gap: 8px; padding: 8px 10px; border-radius: 10px; background: var(--ac-sunken); }
.ac-moved > .ac-btn { justify-self: start; }
.ac-pair { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.ac-row-tight { gap: 4px; }
.ac-fit { justify-self: start; }
.ac-props { display: grid; grid-template-columns: auto 1fr; gap: 4px 12px; margin: 0; }
.ac-props dt { color: var(--ac-ink-muted); font-size: 12px; line-height: 20px; }
.ac-props dd { margin: 0; text-align: right; overflow-wrap: anywhere; line-height: 20px; font-variant-numeric: tabular-nums; }
.ac-keys { display: grid; gap: 8px; margin: 0; }
.ac-keys > div { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 20px; }
.ac-keys dt { color: var(--ac-ink-weak); font-size: 12px; line-height: 16px; }
.ac-keys dd { display: inline-flex; align-items: center; gap: 4px; flex-shrink: 0; margin: 0; color: var(--ac-ink-muted); font-size: 12px; line-height: 16px; }
</style>
