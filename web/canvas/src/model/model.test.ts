import { describe, expect, it } from 'vitest';
import {
  addArrow, addComment, addFrame, addNote, addSection, arrange, duplicate, emptyBoard, feedbackMarkdown, find,
  merge, move, parseBoard, remove, reply, resize, setStatus, type Board,
} from './index.ts';

const withTwoFrames = () => {
  let b = emptyBoard('Onboarding');
  const a = addFrame(b, { route: '/o/acme', name: 'Home', x: 0, y: 0, width: 1280, height: 800 });
  const c = addFrame(a.board, { route: '/o/acme/settings/users', name: 'Users', scenario: 'empty', x: 1400, y: 0, width: 1280, height: 800 });
  b = c.board;
  return { b, home: a.frame, users: c.frame };
};

describe('board ops', () => {
  it('leaves the board it was given unchanged', () => {
    const { b, home } = withTwoFrames();
    const before = JSON.stringify(b);
    move(b, [home.id], { x: 10, y: 10 });
    expect(JSON.stringify(b)).toBe(before);
  });

  it('moves a section with what lies inside it', () => {
    const { b, home, users } = withTwoFrames();
    const s = addSection(b, { x: -100, y: -100, width: 1500, height: 1000 });
    const moved = move(s.board, [s.section.id], { x: 50, y: 0 });
    expect(find(moved, home.id)).toMatchObject({ x: 50 });
    expect(find(moved, users.id)).toMatchObject({ x: 1400 }); // outside the section
  });

  it('removes the arrows and comments of a removed frame', () => {
    const { b, home, users } = withTwoFrames();
    let next = addArrow(b, home.id, users.id, 'Open settings').board;
    next = addComment(next, { anchor: { type: 'element', frame: users.id, tag: 'button' }, body: 'Bigger', author: 'Hung' }).board;
    next = remove(next, [users.id]);
    expect(next.arrows).toHaveLength(0);
    expect(next.comments).toHaveLength(0);
    expect(next.frames.map((f) => f.id)).toEqual([home.id]);
  });

  it('refuses an arrow to itself and keeps one arrow per pair', () => {
    const { b, home, users } = withTwoFrames();
    expect(addArrow(b, home.id, home.id).arrow).toBeUndefined();
    const once = addArrow(b, home.id, users.id).board;
    expect(addArrow(once, home.id, users.id).board.arrows).toHaveLength(1);
  });

  it('duplicates shapes with the arrows between them', () => {
    const { b, home, users } = withTwoFrames();
    const linked = addArrow(b, home.id, users.id).board;
    const r = duplicate(linked, [home.id, users.id]);
    expect(r.ids).toHaveLength(2);
    expect(r.board.frames).toHaveLength(4);
    expect(r.board.arrows).toHaveLength(2);
    expect(find(r.board, r.ids[0])).toMatchObject({ name: 'Home copy', x: 40, y: 40 });
  });

  it('keeps a minimum size', () => {
    const { b, home } = withTwoFrames();
    expect(find(resize(b, home.id, { x: 0, y: 0, width: 10, height: 10 }), home.id)).toMatchObject({ width: 200, height: 100 });
  });

  it('lays shapes out in a row with an even gap', () => {
    const { b, home, users } = withTwoFrames();
    const n = addNote(b, { x: 9000, y: 500, width: 300, height: 200 });
    const out = arrange(n.board, [users.id, home.id, n.note.id], 'row', 100);
    expect([home, users, n.note].map((s) => (find(out, s.id) as { x: number }).x)).toEqual([0, 1380, 2760]);
  });

  it('replies to and resolves a comment', () => {
    const { b, home } = withTwoFrames();
    const c = addComment(b, { anchor: { type: 'element', frame: home.id, tag: 'h1' }, body: 'Shorter title', author: 'Hung' });
    const done = setStatus(reply(c.board, c.comment.id, 'Claude', 'Shortened it'), c.comment.id, 'resolved');
    expect(find(done, c.comment.id)).toMatchObject({ status: 'resolved', replies: [{ author: 'Claude', body: 'Shortened it' }] });
  });
});

describe('parseBoard', () => {
  it('fills in what a hand-written board leaves out', () => {
    const b = parseBoard({ version: 1, name: 'X', frames: [{ id: 'f1', route: '/', x: 0, y: 0, width: 10, height: 10, name: 'f' }] });
    expect(b.rev).toBe(0);
    expect(b.frames[0].kind).toBe('frame');
    expect(b.comments).toEqual([]);
  });

  it('names what is wrong', () => {
    expect(() => parseBoard({ version: 2 })).toThrow(/version/);
    expect(() => parseBoard({ version: 1, frames: [{}] })).toThrow(/frames has an id/);
  });
});

describe('merge', () => {
  it('keeps both sides\' changes to different objects', () => {
    const { b: base, home, users } = withTwoFrames();
    const mine = move(base, [home.id], { x: 100, y: 0 });
    const c = addComment(base, { anchor: { type: 'canvas', x: 0, y: 0 }, body: 'Hi', author: 'Hung' });
    const theirs: Board = { ...remove(c.board, [users.id]), rev: 5 };
    const out = merge(base, mine, theirs);
    expect(find(out, home.id)).toMatchObject({ x: 100 });
    expect(find(out, users.id)).toBeUndefined();
    expect(out.comments).toHaveLength(1);
    expect(out.rev).toBe(5);
  });

  it('keeps what I added and removes what I removed', () => {
    const { b: base, home } = withTwoFrames();
    const n = addNote(remove(base, [home.id]), { text: 'mine' });
    const out = merge(base, n.board, { ...base, rev: 2 });
    expect(find(out, home.id)).toBeUndefined();
    expect(find(out, n.note.id)).toBeDefined();
  });
});

describe('feedbackMarkdown', () => {
  it('says where each comment points', () => {
    const { b, users } = withTwoFrames();
    const c = addComment(b, {
      anchor: { type: 'element', frame: users.id, tag: 'button', component: 'Button', text: 'Invite', source: 'src/pages/Users.vue:42:7' },
      body: 'Make this the primary action',
      author: 'Hung',
    });
    const md = feedbackMarkdown(c.board);
    expect(md).toContain('## Feedback on Onboarding');
    expect(md).toContain('- Element: `<Button>` "Invite"');
    expect(md).toContain('- Source: `src/pages/Users.vue:42:7`');
    expect(md).toContain('- Frame: Users (`/o/acme/settings/users`, scenario `empty`)');
    expect(md).toContain('> Make this the primary action');
    expect(md).toContain(`- Id: \`${c.comment.id}\` (open)`);
  });

  it('says when there is nothing open', () => {
    expect(feedbackMarkdown(emptyBoard('X'))).toBe('No open feedback on X.');
  });
});
