<script setup lang="ts">
// The properties of what is selected: a picked element (ElementPanel, written back to its file),
// or a frame, section, note or arrow (on the board).
import { computed } from 'vue';
import {
  FRAME_SIZES, arrange, bounds, isShape, update,
  type Arrow, type BoardObject, type Frame, type Note, type Section, type Shape,
} from '../../model/index.ts';
import { duplicateSelection, wrapSelection } from '../commands.ts';
import { scenarioOptions, useStore } from '../context.ts';
import Icon from './Icon.vue';
import Select from './Select.vue';
import ElementPanel from './ElementPanel.vue';

const store = useStore();
const { state, board } = store;

const selected = computed<BoardObject[]>(() => {
  void board.value;
  return store.selected();
});
const one = computed(() => (selected.value.length === 1 ? selected.value[0] : undefined));
const picked = computed(() => state.picked);

// --- Board objects ---------------------------------------------------------------------------------

const set = <T extends BoardObject>(patch: Partial<T>) => {
  if (board.value && one.value) store.commit(update<T>(board.value, one.value.id, patch as never));
};
const num = (e: Event) => Math.round(Number((e.target as HTMLInputElement).value));
const str = (e: Event) => (e.target as HTMLInputElement).value;
const options = computed(() => scenarioOptions(store));
const shapesSelected = computed(() => selected.value.filter(isShape) as Shape[]);

const wrap = () => wrapSelection(store);
const copyShapes = () => duplicateSelection(store);

const frameOf = (o: BoardObject | undefined) => (o?.kind === 'frame' ? (o as Frame) : undefined);
const mcpCommand = computed(() => `claude mcp add --transport http anfra-canvas ${location.origin}${state.settings.mcp}`);
</script>

<template>
  <aside class="ac-panel ac-inspector" aria-label="Properties">
    <!-- A picked element -->
    <ElementPanel v-if="picked" :key="`${picked.frame}|${picked.info.ref.source ?? picked.info.ref.selector}|${picked.info.ref.index ?? 0}`" />

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
          <div><dt>Every command</dt><dd><kbd class="ac-kbd">⌘K</kbd></dd></div>
          <div><dt>More on what you click</dt><dd><kbd class="ac-kbd">Right-click</kbd></dd></div>
          <div><dt>Select, pan, use the app</dt><dd><kbd class="ac-kbd">V</kbd><kbd class="ac-kbd">H</kbd><kbd class="ac-kbd">P</kbd></dd></div>
          <div><dt>Comment</dt><dd><kbd class="ac-kbd">C</kbd></dd></div>
          <div><dt>Frame, section, note, arrow</dt><dd><kbd class="ac-kbd">F</kbd><kbd class="ac-kbd">S</kbd><kbd class="ac-kbd">N</kbd><kbd class="ac-kbd">A</kbd></dd></div>
          <div><dt>Pan</dt><dd><kbd class="ac-kbd">Space</kbd> or scroll</dd></div>
          <div><dt>Zoom</dt><dd><kbd class="ac-kbd">⌘</kbd> scroll</dd></div>
          <div><dt>Fit, zoom to selection</dt><dd><kbd class="ac-kbd">⇧1</kbd><kbd class="ac-kbd">⇧2</kbd></dd></div>
          <div><dt>Edit text</dt><dd>Double-click</dd></div>
          <div><dt>Duplicate, delete</dt><dd><kbd class="ac-kbd">⌘D</kbd><kbd class="ac-kbd">⌫</kbd></dd></div>
          <div><dt>Tidy up the flow</dt><dd><kbd class="ac-kbd">⌃⌥T</kbd></dd></div>
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
