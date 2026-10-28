import { mkdir, mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { beforeEach, describe, expect, it } from 'vitest';
import { addFrame, emptyBoard, type Board } from '../model/index.ts';
import { commonFolder, filesIn, keepAndDeleteFolder, restoreFiles, screensFolder } from './board-files.ts';
import { BoardStore } from './boards.ts';
import { handleMcp, type McpContext } from './mcp.ts';

const withFrames = (...routes: string[]) => routes.reduce((b: Board, route) => addFrame(b, { route }).board, emptyBoard('b'));

describe('the files a board stands for', () => {
  const p = { dir: '/app/src/prototypes', route: '/p' };

  it('finds the deepest folder holding every file, never the prototypes folder itself', () => {
    expect(commonFolder(p.dir, ['/app/src/prototypes/datasets/bulk/A.vue', '/app/src/prototypes/datasets/bulk/views/B.vue'])).toBe('/app/src/prototypes/datasets/bulk');
    expect(commonFolder(p.dir, ['/app/src/prototypes/datasets/A.vue', '/app/src/prototypes/reports/B.vue'])).toBeUndefined();
  });

  it('offers the screens\' folder only when every screen is in it and no other board shows one there', () => {
    const mine = withFrames('/p/datasets/bulk/List?x=1', '/p/datasets/bulk/Detail');
    expect(screensFolder(mine, [], p)).toBe('/app/src/prototypes/datasets/bulk');
    expect(screensFolder(mine, [withFrames('/p/datasets/bulk/Detail')], p)).toBeUndefined();
    expect(screensFolder(mine, [withFrames('/p/datasets/Other')], p)).toBe('/app/src/prototypes/datasets/bulk');
    expect(screensFolder(withFrames('/p/datasets/bulk/List', '/reporting'), [], p)).toBeUndefined();
    expect(screensFolder(withFrames('/p/Home'), [], p)).toBeUndefined();
  });

  it('keeps what it deletes, and puts it back unless something took its place', async () => {
    const root = await mkdtemp(join(tmpdir(), 'canvas-delete-'));
    const folder = join(root, 'protos', 'feature', 'proto');
    await mkdir(join(folder, '_parts'), { recursive: true });
    await writeFile(join(folder, 'A.vue'), 'a');
    await writeFile(join(folder, '_parts', 'B.vue'), 'b');
    await writeFile(join(root, 'board.json'), '{}');
    const kept = await keepAndDeleteFolder(join(root, 'board.json'), folder, join(root, 'protos'));
    expect(await filesIn(folder)).toEqual([]);
    // The folder it left empty goes too, but never the prototypes folder itself.
    await expect(readFile(join(root, 'protos', 'feature'))).rejects.toThrow();
    expect(await filesIn(join(root, 'protos'))).toEqual([]);
    await expect(import('node:fs/promises').then((fs) => fs.stat(join(root, 'protos')))).resolves.toBeTruthy();
    expect(kept.files.map((f) => f.path).sort()).toEqual([join(folder, 'A.vue'), join(folder, '_parts', 'B.vue')]);
    // The board's own file is the store's to delete; here it is still there, so undo is refused.
    await expect(restoreFiles(kept)).rejects.toThrow(/exists again/);
    await import('node:fs/promises').then((fs) => fs.rm(join(root, 'board.json')));
    await restoreFiles(kept);
    expect(await readFile(join(folder, '_parts', 'B.vue'), 'utf8')).toBe('b');
    expect(await readFile(join(root, 'board.json'), 'utf8')).toBe('{}');
  });
});

describe('delete_board over MCP', () => {
  let ctx: McpContext;
  let calls: [string, boolean][];

  beforeEach(async () => {
    calls = [];
    const store = new BoardStore(await mkdtemp(join(tmpdir(), 'canvas-')), () => {});
    await store.create('bulk', 'Bulk edit');
    ctx = {
      store, scenarios: async () => ({}), instructions: '', url: '/__canvas/',
      boardFiles: async () => ({ board: { path: 'boards/bulk.json', git: 'new' }, folder: { path: 'src/prototypes/datasets/bulk', files: 4, git: 'committed' } }),
      deleteBoard: async (slug, withFolder) => {
        calls.push([slug, withFolder]);
        return { board: { path: 'boards/bulk.json', git: 'new' } };
      },
    };
  });

  const call = async (args: Record<string, unknown>) => {
    const res = await handleMcp({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'delete_board', arguments: args } }, ctx) as { result: { content: { text: string }[] } };
    return res.result.content[0].text;
  };

  it('says what it would delete, and deletes only once confirmed with the board\'s name', async () => {
    expect(await call({ board: 'bulk' })).toMatch(/would delete:\n- boards\/bulk\.json \(never committed\)\nIts screens' folder \(src\/prototypes\/datasets\/bulk\) stays; pass folder: true/);
    expect(await call({ board: 'bulk', folder: true, confirm: 'nope' })).toMatch(/- src\/prototypes\/datasets\/bulk \(committed\): 4 files\nAsk the person/);
    expect(calls).toEqual([]);
    expect(await call({ board: 'bulk', folder: true, confirm: 'bulk' })).toMatch(/^Deleted:/);
    expect(calls).toEqual([['bulk', true]]);
  });
});
