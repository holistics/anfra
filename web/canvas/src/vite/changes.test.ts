import { execFileSync } from 'node:child_process';
import { mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { CommitError, Stills, changedLines, commit, discard, parseDiff, readChanges, undoDiscard } from './changes.ts';
import { touched } from './source.ts';

const DIFF = `diff --git a/src/A.vue b/src/A.vue
index 1111111..2222222 100644
--- a/src/A.vue
+++ b/src/A.vue
@@ -1,4 +1,4 @@
 <template>
-  <h1>Set up</h1>
+  <h1>Welcome</h1>
   <p>Two steps</p>
 </template>
diff --git a/old.ts b/new.ts
similarity 90%
rename from old.ts
rename to new.ts
`;

const SFC = `<template>
  <main>
    <h1>Welcome</h1>
    <p>Two steps</p>
  </main>
</template>
`;

/** A repository with one commit on main, in a folder of its own. */
async function repo () {
  const dir = await mkdtemp(join(tmpdir(), 'canvas-git-'));
  const git = (...args: string[]) => execFileSync('git', args, { cwd: dir, encoding: 'utf8' });
  git('init', '-q', '-b', 'main');
  git('config', 'user.name', 'Test');
  git('config', 'user.email', 'test@example.com');
  git('config', 'commit.gpgsign', 'false');
  await writeFile(join(dir, 'A.vue'), SFC);
  git('add', '.');
  git('commit', '-q', '-m', 'first');
  return { dir, git };
}

describe('parseDiff', () => {
  it('reads files, their status, and each line with its numbers', () => {
    const [a, renamed] = parseDiff(DIFF);
    expect(a).toMatchObject({ path: 'src/A.vue', status: 'modified', added: 1, removed: 1 });
    expect(a.hunks[0].lines.map((l) => [l.kind, l.old, l.new])).toEqual([
      [' ', 1, 1], ['-', 2, undefined], ['+', undefined, 2], [' ', 3, 3], [' ', 4, 4],
    ]);
    expect(renamed).toMatchObject({ path: 'new.ts', from: 'old.ts', status: 'renamed' });
  });

  it('marks the new file\'s lines a change touched', () => {
    expect([...changedLines(parseDiff(DIFF)[0])]).toEqual([2]);
  });
});

describe('touched', () => {
  it('answers the elements a changed line is in, or only the outermost', () => {
    expect(touched(SFC, 'A.vue', new Set([3]))).toEqual(['A.vue:3:5']);
    expect(touched(SFC, 'A.vue', new Set([2, 3, 4, 5]))).toEqual(['A.vue:2:3', 'A.vue:3:5', 'A.vue:4:5']);
    expect(touched(SFC, 'A.vue', new Set([2, 3, 4, 5]), true)).toEqual(['A.vue:2:3']);
  });
});

describe('readChanges and commit', () => {
  it('reads what changed since the last commit, and on the branch', async () => {
    const { dir, git } = await repo();
    git('switch', '-q', '-c', 'design/welcome');
    await writeFile(join(dir, 'A.vue'), SFC.replace('Welcome', 'Hello'));
    await writeFile(join(dir, 'B.vue'), '<template><p>New</p></template>\n');

    const now = await readChanges(dir, 'head');
    expect(now).toMatchObject({ repo: true, branch: 'design/welcome', defaultBranch: 'main', scope: 'head', needsBranch: false });
    expect(now.files.map((f) => [f.path, f.status])).toEqual([['A.vue', 'modified'], ['B.vue', 'added']]);
    expect(now.pending).toEqual(['A.vue', 'B.vue']);
    expect(now.marks).toEqual({ 'A.vue': ['A.vue:3:5'], 'B.vue': ['B.vue:1:11'] });

    const done = await commit(dir, { message: 'say hello' });
    expect(done.branch).toBe('design/welcome');
    const after = await readChanges(dir, 'branch');
    expect(after.pending).toEqual([]);
    expect(after.files.map((f) => f.path)).toEqual(['A.vue', 'B.vue']);
    expect(after.commits.map((c) => c.subject)).toEqual(['say hello']);
  });

  it('asks for a new branch on the default branch, and commits there', async () => {
    const { dir, git } = await repo();
    await writeFile(join(dir, 'A.vue'), SFC.replace('Welcome', 'Hello'));
    expect((await readChanges(dir, 'head')).needsBranch).toBe(true);
    await expect(commit(dir, { message: 'say hello' })).rejects.toBeInstanceOf(CommitError);
    await expect(commit(dir, { message: ' ', branch: 'design/x' })).rejects.toThrow('Write what changed');
    expect(await commit(dir, { message: 'say hello', branch: 'design/hello' })).toMatchObject({ branch: 'design/hello' });
    expect(git('log', '-1', '--format=%s', 'design/hello').trim()).toBe('say hello');
    expect(git('rev-list', '--count', 'main').trim()).toBe('1');
  });
});

describe('discard', () => {
  it('puts files back as the last commit has them, and undoes that', async () => {
    const { dir } = await repo();
    await writeFile(join(dir, 'A.vue'), SFC.replace('Welcome', 'Hello'));
    await writeFile(join(dir, 'B.vue'), '<template><p>New</p></template>\n');
    await expect(discard(dir, ['../A.vue'])).rejects.toBeInstanceOf(CommitError);
    await expect(discard(dir, ['C.vue'])).rejects.toThrow('no changes to discard');

    const kept = await discard(dir, ['A.vue', 'B.vue']);
    expect(await readFile(join(dir, 'A.vue'), 'utf8')).toBe(SFC);
    await expect(readFile(join(dir, 'B.vue'))).rejects.toThrow();
    expect((await readChanges(dir, 'head')).files).toEqual([]);

    await undoDiscard(dir, kept);
    expect(await readFile(join(dir, 'A.vue'), 'utf8')).toContain('Hello');
    expect(await readFile(join(dir, 'B.vue'), 'utf8')).toContain('New');

    // Changed again since: the undo would lose that, so it is refused.
    const again = await discard(dir, ['A.vue']);
    await writeFile(join(dir, 'A.vue'), SFC.replace('Welcome', 'Hi'));
    await expect(undoDiscard(dir, again)).rejects.toThrow('changed since');
  });
});

describe('Stills', () => {
  it('keeps a frame\'s first still since the commit, and lets other commits\' go', async () => {
    const stills = new Stills(await mkdtemp(join(tmpdir(), 'canvas-stills-')));
    expect(await stills.keep('c1', 'board', 'f1', '<svg>before</svg>')).toBe(true);
    expect(await stills.keep('c1', 'board', 'f1', '<svg>later</svg>')).toBe(false);
    expect((await stills.read('c1', 'board', 'f1')).toString()).toBe('<svg>before</svg>');
    expect(await stills.list('c1')).toEqual({ board: ['f1'] });
    expect(await stills.list('c2')).toEqual({});
    await expect(readFile(join(stills.dir, 'c1', 'board', 'f1.svg'))).rejects.toThrow();
  });
});
