// Deleting a board from the canvas, in the repo: its JSON, and, when asked, the folder of its
// screens. What is deleted is kept in memory until the dev server stops, so the home page can undo
// it: a file that was never committed can't come back with git.
import { execFile } from 'node:child_process';
import { mkdir, readdir, readFile, rm, stat, writeFile } from 'node:fs/promises';
import { dirname, join, relative, sep } from 'node:path';
import { promisify } from 'node:util';
import type { Board } from '../model/board.ts';

const run = promisify(execFile);

/** Where a file stands in git: committed as it is, committed and changed since, or never committed. */
export type GitState = 'committed' | 'changed' | 'new';

export interface BoardFiles {
  /** The board's own file, from the app's root. */
  board: { path: string, git: GitState };
  /**
   * The folder holding every screen of the board, offered to delete with it: only when it is not
   * the prototypes folder itself and no other board shows a screen in it. From the app's root.
   */
  folder?: { path: string, files: number, git: GitState };
}

/** A board deleted in this server's life, as it was, for Undo. */
export interface Deleted {
  board: { path: string, text: string };
  files: { path: string, content: Buffer }[];
}

const pathOf = (route: string) => route.split(/[?#]/)[0];

/** The deepest folder under `dir` that holds every one of `files`, or undefined when it is `dir`. */
export function commonFolder (dir: string, files: string[]): string | undefined {
  if (!files.length) return undefined;
  let common = dirname(files[0]);
  for (const f of files.slice(1)) {
    while (!(f.startsWith(common + sep))) {
      const up = dirname(common);
      if (up === common) return undefined;
      common = up;
    }
  }
  const r = relative(dir, common);
  return r && !r.startsWith('..') ? common : undefined;
}

/**
 * The folder of `board`'s screens (absolute), when it can go with the board: every screen of the
 * board is in it, it is below the prototypes folder, and no board in `others` shows a screen in it.
 */
export function screensFolder (board: Board, others: Board[], prototypes: { dir: string, route: string }): string | undefined {
  const prefix = `${prototypes.route}/`;
  const toFile = (route: string) => join(prototypes.dir, `${pathOf(route).slice(prefix.length)}.vue`);
  const mine = board.frames.map((f) => f.route).filter((r) => pathOf(r).startsWith(prefix));
  if (!mine.length || mine.length !== board.frames.length) return undefined;
  const folder = commonFolder(prototypes.dir, mine.map(toFile));
  if (!folder) return undefined;
  const theirs = others.flatMap((b) => b.frames.map((f) => f.route)).filter((r) => pathOf(r).startsWith(prefix)).map(toFile);
  return theirs.some((f) => f.startsWith(folder + sep)) ? undefined : folder;
}

/** Where `path` (absolute, a file or a folder) stands in git, from the repo at `root`. */
export async function gitState (root: string, path: string): Promise<GitState> {
  const rel = relative(root, path) || '.';
  try {
    const tracked = (await run('git', ['ls-files', '--', rel], { cwd: root })).stdout.trim();
    if (!tracked) return 'new';
    const status = (await run('git', ['status', '--porcelain', '--', rel], { cwd: root })).stdout.trim();
    return status ? 'changed' : 'committed';
  } catch {
    return 'new';
  }
}

export async function filesIn (dir: string): Promise<string[]> {
  const out: string[] = [];
  for (const e of await readdir(dir, { withFileTypes: true }).catch(() => [])) {
    const p = join(dir, e.name);
    if (e.isDirectory()) out.push(...await filesIn(p));
    else out.push(p);
  }
  return out;
}

/**
 * Keep a board's file, and the folder's files when given, as they are now, then delete the folder
 * and the folders above it that it leaves empty, up to `stopAt`. The board's own file is the
 * store's to delete.
 */
export async function keepAndDeleteFolder (boardPath: string, folder: string | undefined, stopAt?: string): Promise<Deleted> {
  const kept: Deleted = { board: { path: boardPath, text: await readFile(boardPath, 'utf8') }, files: [] };
  if (folder) {
    for (const f of await filesIn(folder)) kept.files.push({ path: f, content: await readFile(f) });
    await rm(folder, { recursive: true, force: true });
    for (let dir = dirname(folder); stopAt && dir.startsWith(stopAt + sep); dir = dirname(dir)) {
      if ((await readdir(dir).catch(() => ['?'])).length) break;
      await rm(dir, { recursive: true, force: true });
    }
  }
  return kept;
}

/** Put back what `keepAndDeleteFolder` kept, refusing when anything has come back in its place since. */
export async function restoreFiles (kept: Deleted): Promise<void> {
  for (const p of [kept.board.path, ...kept.files.map((f) => f.path)]) {
    if (await stat(p).catch(() => undefined)) throw new Error(`${p} exists again: move it away to undo`);
  }
  for (const f of kept.files) {
    await mkdir(dirname(f.path), { recursive: true });
    await writeFile(f.path, f.content);
  }
  await mkdir(dirname(kept.board.path), { recursive: true });
  await writeFile(kept.board.path, kept.board.text);
}
