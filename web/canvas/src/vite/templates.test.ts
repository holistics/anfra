import { mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { beforeEach, describe, expect, it } from 'vitest';
import { placeTemplate, emptyBoard, type Template } from '../model/index.ts';
import { BoardStore } from './boards.ts';
import { handleMcp, type McpContext } from './mcp.ts';
import { Templates, startFrom } from './templates.ts';

const TEMPLATE: Template = {
  name: 'Dataset exploration',
  description: 'Explore a dataset.',
  area: 'Reporting',
  cover: 'detail',
  screens: [
    { id: 'list', file: 'List', name: 'Datasets' },
    { id: 'detail', file: 'Explore', name: 'Explore', query: 'id=ecommerce' },
  ],
  arrows: [{ from: 'list', to: 'detail', label: 'Open one' }],
};

let root: string;
let templates: Templates;
let store: BoardStore;
let prototypes: { dir: string, route: string };

beforeEach(async () => {
  root = await mkdtemp(join(tmpdir(), 'canvas-templates-'));
  const dir = join(root, 'templates', 'dataset-exploration');
  await mkdir(join(dir, '_parts'), { recursive: true });
  await writeFile(join(dir, 'template.ts'), '// the template');
  await writeFile(join(dir, 'List.vue'), '<template><p>List</p></template>');
  await writeFile(join(dir, 'Explore.vue'), '<template><p>Explore</p></template>');
  await writeFile(join(dir, '_parts', 'Pane.vue'), '<template><p>Pane</p></template>');
  // Modules come from Vite in the canvas; here a template.ts answers the one template.
  templates = new Templates(join(root, 'templates'), async () => ({ template: TEMPLATE }), '/t');
  store = new BoardStore(join(root, 'boards'), () => {});
  prototypes = { dir: join(root, 'prototypes'), route: '/p' };
});

describe('templates', () => {
  it('lists each folder with its preview at its cover screen', async () => {
    const [t] = await templates.list();
    expect(t).toMatchObject({ id: 'dataset-exploration', name: 'Dataset exploration', preview: '/t/dataset-exploration/Explore?id=ecommerce' });
  });

  it('copies the folder but its template.ts, and puts the screens on a new board with their arrow', async () => {
    const r = await startFrom(templates, store, prototypes, { template: 'dataset-exploration', project: 'datasets/explore', board: 'explore' });
    expect(r.files.sort()).toEqual(['Explore.vue', 'List.vue', '_parts/Pane.vue']);
    expect(await readFile(join(prototypes.dir, 'datasets/explore/_parts/Pane.vue'), 'utf8')).toContain('Pane');
    await expect(readFile(join(prototypes.dir, 'datasets/explore/template.ts'))).rejects.toThrow();
    const b = await store.read('explore');
    expect(b).toMatchObject({ name: 'Dataset exploration', project: 'datasets' });
    expect(b.frames.map((f) => f.route)).toEqual(['/p/datasets/explore/List', '/p/datasets/explore/Explore?id=ecommerce']);
    expect(b.arrows).toMatchObject([{ from: r.frames.list, to: r.frames.detail, label: 'Open one' }]);
    expect(b.sections.map((s) => s.name)).toEqual(['Dataset exploration']);
  });

  it('copies again over the same files, and refuses a folder where a file differs', async () => {
    await startFrom(templates, store, prototypes, { template: 'dataset-exploration', project: 'explore', board: 'one' });
    const again = await startFrom(templates, store, prototypes, { template: 'dataset-exploration', project: 'explore', board: 'one', screens: ['detail'] });
    expect(again.files).toEqual([]);
    expect((await store.read('one')).frames).toHaveLength(3);
    await writeFile(join(prototypes.dir, 'explore/List.vue'), '<template><p>Changed</p></template>');
    await expect(startFrom(templates, store, prototypes, { template: 'dataset-exploration', project: 'explore', board: 'two' })).rejects.toThrow(/List.vue is already in that project/);
    await expect(startFrom(templates, store, prototypes, { template: 'dataset-exploration', project: 'Explore', board: 'two' })).rejects.toThrow(/project folder/);
    await expect(startFrom(templates, store, prototypes, { template: 'dataset-exploration', project: 'x', board: 'two', screens: ['nope'] })).rejects.toThrow(/no screen nope/);
    await expect(startFrom(templates, store, prototypes, { template: 'nope', project: 'x', board: 'two' })).rejects.toThrow('no template nope: there are dataset-exploration');
  });

  it('places one screen without a section', () => {
    const one = placeTemplate(emptyBoard('b'), { ...TEMPLATE, screens: [TEMPLATE.screens[0]] }, '/p/x');
    expect(one.board.frames).toHaveLength(1);
    expect(one.board.sections).toEqual([]);
  });

  it('is offered over MCP', async () => {
    const ctx: McpContext = {
      store, scenarios: async () => ({}), instructions: '', url: '/__canvas/',
      templates: { list: () => templates.list(), read: (id) => templates.read(id), start: (req) => startFrom(templates, store, prototypes, req) },
    };
    const call = async (name: string, args: Record<string, unknown> = {}) => {
      const res = await handleMcp({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } }, ctx) as { result: { content: { text: string }[], isError?: boolean } };
      return res.result.content[0].text;
    };
    expect(await call('list_templates')).toMatch(/`dataset-exploration`: Dataset exploration \(Reporting\)\. Explore a dataset\. Screens: list, detail\./);
    expect(JSON.parse(await call('get_template', { template: 'dataset-exploration' }))).toMatchObject({ id: 'dataset-exploration', screens: [{ id: 'list' }, { id: 'detail' }] });
    expect(await call('create_from_template', { template: 'dataset-exploration', project: 'explore', board: 'mcp' })).toMatch(/^Copied 3 files: .*\nFrames on mcp: list → \S+, detail → \S+\.\n\/__canvas\/#mcp$/);
    expect(await call('add_template_screen', { board: 'mcp', template: 'dataset-exploration', screen: 'detail', project: 'explore' })).toMatch(/^Copied 0 files \(all were there already\)\./);
  });
});
