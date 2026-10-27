import { mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { beforeEach, describe, expect, it } from 'vitest';
import { addComment, emptyBoard } from '../model/index.ts';
import { BoardStore, Conflict } from './boards.ts';
import { handleMcp, type McpContext } from './mcp.ts';

let ctx: McpContext;
let changes: string[];

beforeEach(async () => {
  changes = [];
  const store = new BoardStore(await mkdtemp(join(tmpdir(), 'canvas-')), (slug) => changes.push(slug));
  ctx = { store, scenarios: async () => ({ default: { label: 'Signed in', ops: {} }, empty: { ops: {} } }), instructions: 'Read the guide', url: '/__canvas/' };
});

const call = async (name: string, args: Record<string, unknown> = {}) => {
  const res = await handleMcp({ jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } }, ctx) as { result: { content: { text: string }[], isError?: boolean } };
  return { text: res.result.content[0].text, error: !!res.result.isError };
};

describe('the MCP server', () => {
  it('initializes with the app\'s instructions and lists its tools', async () => {
    const init = await handleMcp({ jsonrpc: '2.0', id: 0, method: 'initialize', params: { protocolVersion: '2025-03-26' } }, ctx) as { result: Record<string, unknown> };
    expect(init.result).toMatchObject({ protocolVersion: '2025-03-26', capabilities: { tools: {} }, instructions: 'Read the guide' });
    expect(await handleMcp({ jsonrpc: '2.0', method: 'notifications/initialized' }, ctx)).toBeUndefined();
    const list = await handleMcp({ jsonrpc: '2.0', id: 2, method: 'tools/list' }, ctx) as { result: { tools: { name: string }[] } };
    expect(list.result.tools.map((t) => t.name)).toContain('list_feedback');
  });

  it('lays out a board', async () => {
    await call('create_board', { board: 'invite', name: 'Invite flow' });
    const section = await call('add_section', { board: 'invite', name: 'Happy path' });
    const id = /section (\S+)\./.exec(section.text)![1];
    const frame = await call('add_frame', { board: 'invite', route: '/o/acme', section: id });
    expect(frame.error).toBe(false);
    const board = JSON.parse((await call('get_board', { board: 'invite' })).text);
    expect(board.sections[0].contains).toHaveLength(1);
    expect(board.frames[0]).toMatchObject({ route: '/o/acme', scenario: 'default' });
    expect(changes.every((s) => s === 'invite')).toBe(true);
  });

  it('grows a section to hold the frames put in it', async () => {
    await call('create_board', { board: 'b', name: 'B' });
    const id = /section (\S+)\./.exec((await call('add_section', { board: 'b', name: 'Edge cases' })).text)![1];
    for (let i = 0; i < 4; i++) await call('add_frame', { board: 'b', route: `/o/acme/${i}`, section: id });
    const board = JSON.parse((await call('get_board', { board: 'b' })).text);
    expect(board.sections[0].contains).toHaveLength(4);
  });

  it('reads feedback and resolves it with a note', async () => {
    await ctx.store.create('b', 'B');
    let commentId = '';
    await ctx.store.change('b', (b) => {
      const r = addComment(b, { anchor: { type: 'canvas', x: 0, y: 0 }, body: 'Fix the title', author: 'Hung' });
      commentId = r.comment.id;
      return r.board;
    });
    expect((await call('list_feedback')).text).toContain('Fix the title');
    expect((await call('resolve_feedback', { board: 'b', id: commentId, note: 'Done' })).error).toBe(false);
    const saved = JSON.parse(await readFile(ctx.store.path('b'), 'utf8'));
    expect(saved.comments[0]).toMatchObject({ status: 'resolved', replies: [{ author: 'Claude', body: 'Done' }] });
    expect((await call('list_feedback', { board: 'b' })).text).toContain('No open feedback');
  });

  it('answers a tool\'s failure as an error result', async () => {
    await ctx.store.create('b', 'B');
    expect(await call('resolve_feedback', { board: 'b', id: 'nope', note: 'x' })).toEqual({ text: 'no comment nope on B', error: true });
    expect(await call('create_board', { board: '../x' })).toMatchObject({ error: true });
  });

  it('lists scenarios', async () => {
    expect((await call('list_scenarios')).text).toBe('- `default`: Signed in\n- `empty`');
  });
});

describe('the board store', () => {
  it('refuses a save from an older rev with the current board', async () => {
    const first = await ctx.store.create('b', 'B');
    await ctx.store.save('b', { ...first, name: 'B2' }, first.rev);
    await expect(ctx.store.save('b', { ...first, name: 'B3' }, first.rev)).rejects.toBeInstanceOf(Conflict);
  });

  it('reads a board someone wrote by hand', async () => {
    await writeFile(join(ctx.store.dir, 'hand.json'), JSON.stringify({ ...emptyBoard('Hand') }));
    expect(await ctx.store.list()).toEqual([{ slug: 'hand', name: 'Hand', frames: 0, open: 0 }]);
  });
});
