// What changed in the app's files, from git, for the canvas's Changes tab: the files changed since
// the last commit (or since the branch left the default branch), with their diffs, the elements of
// the screens each change touched, and the branch's commits. And committing them from the canvas.
// Everything runs in the app's folder (the Vite root), so only the app's files are seen.
import { execFile } from 'node:child_process';
import { mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join } from 'node:path';
import { promisify } from 'node:util';
import { COLLECTIONS, parseBoard, type Board, type Collection } from '../model/board.ts';
import { touched } from './source.ts';

export interface Line {
  kind: '+' | '-' | ' ';
  text: string;
  /** Its number in the old file and in the new one, where it has one. */
  old?: number;
  new?: number;
}

export interface Hunk {
  header: string;
  lines: Line[];
}

export interface FileDiff {
  path: string;
  status: 'added' | 'modified' | 'deleted' | 'renamed';
  /** For a renamed file: its name before. */
  from?: string;
  added: number;
  removed: number;
  binary?: boolean;
  hunks: Hunk[];
  /** A board's own file (the canvas's, not the app's): what changed on it, in words. */
  board?: BoardChange;
}

export interface BoardChange {
  slug: string;
  name: string;
  /** A phrase each: "4 screens moved", "1 note added". */
  summary: string[];
}

export interface CommitInfo {
  sha: string;
  short: string;
  subject: string;
  author: string;
  when: string;
}

export interface Changes {
  /** The app's folder is in a git repository: without one there is nothing to read. */
  repo: boolean;
  branch: string;
  /** The last commit's id: the stills of the frames are kept by it. */
  head: string;
  /** The branch work starts from: `main`, or what the remote calls its default. */
  defaultBranch: string;
  /** Since what: the last commit, or where this branch left the default branch. */
  scope: 'head' | 'branch';
  files: FileDiff[];
  /** The files changed since the last commit, whatever the scope: what a commit would take. */
  pending: string[];
  /** A commit needs a new branch first: this is the default branch, or no branch. */
  needsBranch: boolean;
  /** Per changed `.vue` file, the places of the elements its changes touched. */
  marks: Record<string, string[]>;
  /** The commits on this branch that the default branch doesn't have, newest first. */
  commits: CommitInfo[];
  /** The frames with a still from before the changes, by board (the plugin adds them). */
  stills: Record<string, string[]>;
}

const exec = promisify(execFile);
const git = async (cwd: string, args: string[]) => (await exec('git', args, { cwd, maxBuffer: 32 * 1024 * 1024 })).stdout;
const tryGit = async (cwd: string, args: string[]) => {
  try {
    return (await git(cwd, args)).trim();
  } catch {
    return '';
  }
};

/** Unified diff text as files and hunks. */
export function parseDiff (text: string): FileDiff[] {
  const files: FileDiff[] = [];
  let file: FileDiff | undefined;
  let hunk: Hunk | undefined;
  let oldAt = 0;
  let newAt = 0;
  for (const raw of text.split('\n')) {
    if (raw.startsWith('diff --git ')) {
      const m = /^diff --git a\/(.+) b\/(.+)$/.exec(raw);
      file = { path: m?.[2] ?? '', status: 'modified', added: 0, removed: 0, hunks: [] };
      files.push(file);
      hunk = undefined;
      continue;
    }
    if (!file) continue;
    if (!hunk) {
      if (raw.startsWith('new file')) file.status = 'added';
      else if (raw.startsWith('deleted file')) file.status = 'deleted';
      else if (raw.startsWith('rename from ')) {
        file.status = 'renamed';
        file.from = raw.slice('rename from '.length);
      } else if (raw.startsWith('Binary files')) file.binary = true;
    }
    const h = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$/.exec(raw);
    if (h) {
      hunk = { header: raw, lines: [] };
      file.hunks.push(hunk);
      oldAt = Number(h[1]);
      newAt = Number(h[2]);
      continue;
    }
    if (!hunk) continue;
    if (raw.startsWith('+')) {
      hunk.lines.push({ kind: '+', text: raw.slice(1), new: newAt++ });
      file.added++;
    } else if (raw.startsWith('-')) {
      hunk.lines.push({ kind: '-', text: raw.slice(1), old: oldAt++ });
      file.removed++;
    } else if (raw.startsWith(' ')) {
      hunk.lines.push({ kind: ' ', text: raw.slice(1), old: oldAt++, new: newAt++ });
    }
  }
  return files;
}

/** The lines of the new file a diff changed: added ones, and where lines were taken out. */
export function changedLines (diff: FileDiff): Set<number> {
  const out = new Set<number>();
  for (const h of diff.hunks) {
    let next = 0;
    for (const l of [...h.lines].reverse()) {
      if (l.new !== undefined) next = l.new;
      if (l.kind === '+') out.add(l.new!);
      else if (l.kind === '-' && next) out.add(next);
    }
  }
  return out;
}

export async function defaultBranch (cwd: string): Promise<string> {
  const remote = await tryGit(cwd, ['symbolic-ref', '--short', 'refs/remotes/origin/HEAD']);
  if (remote) return remote.replace(/^origin\//, '');
  for (const name of ['main', 'master']) if (await tryGit(cwd, ['rev-parse', '--verify', '--quiet', name])) return name;
  return 'main';
}

/** A new file, as a diff that adds every line of it. */
async function added (cwd: string, path: string): Promise<FileDiff> {
  const text = await readFile(join(cwd, path), 'utf8').catch(() => '');
  if (text.includes('\0') || text.length > 400_000) return { path, status: 'added', added: 0, removed: 0, binary: true, hunks: [] };
  const lines = text.replace(/\n$/, '').split('\n');
  return {
    path, status: 'added', added: lines.length, removed: 0,
    hunks: [{ header: `@@ -0,0 +1,${lines.length} @@`, lines: lines.map((t, i) => ({ kind: '+', text: t, new: i + 1 })) }],
  };
}

const NOUNS: Record<Collection, string> = { frames: 'screen', sections: 'section', notes: 'note', arrows: 'arrow', comments: 'comment' };

/** What changed on a board between two versions of its file, in words. Either may be missing. */
export function describeBoard (before: unknown, after: unknown): string[] {
  const read = (v: unknown): Board | undefined => {
    try {
      return v === undefined ? undefined : parseBoard(v);
    } catch {
      return undefined;
    }
  };
  const was = read(before);
  const now = read(after);
  const count = (n: number, noun: string, verb = '') => `${n} ${noun}${n === 1 ? '' : 's'}${verb ? ` ${verb}` : ''}`;
  if (!now) return [];
  if (!was) return COLLECTIONS.filter((k) => now[k].length).map((k) => count(now[k].length, NOUNS[k]));

  const out: string[] = [];
  if (now.name !== was.name) out.push('renamed');
  if (now.project !== was.project) out.push('project changed');
  // Each object is told by its id: come, gone, or changed in place (its spot, its size, the rest).
  const items = (b: Board, k: Collection) => b[k] as unknown as (Record<string, unknown> & { id: string })[];
  const rest = (o: object, keys: string[]) => JSON.stringify(Object.entries(o).filter(([k]) => !keys.includes(k)));
  for (const k of COLLECTIONS) {
    const old = new Map(items(was, k).map((o) => [o.id, o]));
    const ids = new Set(items(now, k).map((o) => o.id));
    const n = { added: 0, removed: [...old.keys()].filter((id) => !ids.has(id)).length, moved: 0, resized: 0, changed: 0 };
    for (const o of items(now, k)) {
      const p = old.get(o.id);
      if (!p) n.added++;
      else if (rest(o, ['x', 'y', 'width', 'height']) !== rest(p, ['x', 'y', 'width', 'height'])) n.changed++;
      else if (o.width !== p.width || o.height !== p.height) n.resized++;
      else if (o.x !== p.x || o.y !== p.y) n.moved++;
    }
    for (const verb of ['added', 'removed', 'moved', 'resized', 'changed'] as const) {
      if (n[verb]) out.push(count(n[verb], NOUNS[k], verb));
    }
  }
  return out.length ? out : ['saved again, nothing else changed'];
}

/**
 * What changed, since the last commit or (scope `branch`) since this branch left the default
 * branch. `boards` is the folder of the canvas's boards: their files are told apart, in words.
 */
export async function readChanges (cwd: string, scope: 'head' | 'branch', o: { boards?: string } = {}): Promise<Changes> {
  const repo = !!await tryGit(cwd, ['rev-parse', '--git-dir']);
  const branch = await tryGit(cwd, ['branch', '--show-current']);
  const main = await defaultBranch(cwd);
  const head = await tryGit(cwd, ['rev-parse', 'HEAD']);
  const upstream = await tryGit(cwd, ['rev-parse', '--verify', '--quiet', `origin/${main}`]) ? `origin/${main}` : main;
  const forkedAt = !branch || branch === main ? '' : await tryGit(cwd, ['merge-base', upstream, 'HEAD']);
  const base = scope === 'branch' && forkedAt ? forkedAt : 'HEAD';

  const files = parseDiff(await git(cwd, ['diff', base, '--relative', '--no-color', '--find-renames', '-U3']).catch(() => ''));
  const untracked = (await tryGit(cwd, ['ls-files', '--others', '--exclude-standard'])).split('\n').filter(Boolean);
  files.push(...await Promise.all(untracked.map((p) => added(cwd, p))));
  files.sort((p, q) => p.path.localeCompare(q.path));
  const pending = base === 'HEAD'
    ? files.map((f) => f.path)
    : [...(await tryGit(cwd, ['diff', 'HEAD', '--relative', '--name-only'])).split('\n').filter(Boolean), ...untracked].sort();

  // A new file is marked by its outermost elements: every element in it is new.
  const marks: Record<string, string[]> = {};
  await Promise.all(files.filter((f) => f.path.endsWith('.vue') && f.status !== 'deleted').map(async (f) => {
    const code = await readFile(join(cwd, f.path), 'utf8').catch(() => '');
    const places = code ? touched(code, f.path, changedLines(f), f.status === 'added') : [];
    if (places.length) marks[f.path] = places;
  }));

  // A board's file: what changed on it, from the file at the base and the file now.
  const boardOf = (p: string) => (o.boards && p.startsWith(`${o.boards}/`) && p.endsWith('.json') ? p.slice(o.boards.length + 1, -5) : undefined);
  await Promise.all(files.filter((f) => boardOf(f.path)).map(async (f) => {
    const json = (text: string) => {
      try {
        return text ? JSON.parse(text) as unknown : undefined;
      } catch {
        return undefined;
      }
    };
    const before = f.status === 'added' ? undefined : json(await tryGit(cwd, ['show', `${base}:./${f.from ?? f.path}`]));
    const after = f.status === 'deleted' ? undefined : json(await readFile(join(cwd, f.path), 'utf8').catch(() => ''));
    const name = (v: unknown) => (typeof v === 'object' && v && typeof (v as Board).name === 'string' ? (v as Board).name : undefined);
    const slug = boardOf(f.path)!;
    f.board = { slug, name: name(after) ?? name(before) ?? slug, summary: describeBoard(before, after) };
  }));

  const log = forkedAt ? await tryGit(cwd, ['log', '--format=%H%x1f%h%x1f%s%x1f%an%x1f%cr', `${forkedAt}..HEAD`, '--', '.']) : '';
  const commits = log.split('\n').filter(Boolean).map((l) => {
    const [sha, short, subject, author, when] = l.split('\x1f');
    return { sha, short, subject, author, when };
  });

  return {
    repo, branch: branch || 'HEAD', head: head || 'none', defaultBranch: main, scope: base === 'HEAD' ? 'head' : 'branch',
    files, pending, needsBranch: !branch || branch === main, marks, commits, stills: {},
  };
}

/** One commit's changes to the app's files. */
export async function readCommit (cwd: string, sha: string): Promise<FileDiff[]> {
  if (!/^[0-9a-f]{7,40}$/.test(sha)) throw new Error(`"${sha}" is not a commit`);
  return parseDiff(await git(cwd, ['show', sha, '--format=', '--relative', '--no-color', '--find-renames', '-U3']));
}

export class CommitError extends Error {}

/**
 * Commit the app's changed files (or the ones given), on a new branch first if one is named. On the
 * default branch a new branch is asked for: work there goes through a pull request.
 */
export async function commit (cwd: string, o: { message: string, branch?: string, files?: string[] }): Promise<{ sha: string, branch: string }> {
  const message = o.message.trim();
  if (!message) throw new CommitError('Write what changed');
  const current = await tryGit(cwd, ['branch', '--show-current']);
  const main = await defaultBranch(cwd);
  if (o.branch) {
    if (!await tryGit(cwd, ['check-ref-format', '--branch', o.branch])) throw new CommitError(`"${o.branch}" can't be a branch name`);
    await run(cwd, ['switch', '-c', o.branch]);
  } else if (!current || current === main) {
    throw new CommitError(`You're on ${current || 'no branch'}: name a new branch for this work`);
  }
  const paths = o.files?.length ? o.files : ['.'];
  await run(cwd, ['add', '--all', '--', ...paths]);
  await run(cwd, ['commit', '-m', message, '--', ...paths]);
  return { sha: await tryGit(cwd, ['rev-parse', '--short', 'HEAD']), branch: await tryGit(cwd, ['branch', '--show-current']) };
}

// --- Discarding: files put back as the last commit has them --------------------------------------

/** A file as a discard found it and left it, for its undo: null where there was no file. */
export interface Discarded {
  path: string;
  before: Buffer | null;
  after: Buffer | null;
}

/**
 * Put files back as the last commit has them: a changed or deleted one restored, a new one removed
 * (a renamed one is both, under its two names). Only the app's files with changes since the last
 * commit: a file with none, or outside the app's folder, is refused before anything is touched.
 */
export async function discard (cwd: string, files: string[]): Promise<Discarded[]> {
  if (!files.length) throw new CommitError('Which files?');
  for (const path of files) {
    if (!path || isAbsolute(path) || path.split(/[\\/]/).includes('..')) throw new CommitError(`"${path}" isn't one of the app's files`);
    if (!await tryGit(cwd, ['status', '--porcelain', '--untracked-files=all', '--', path])) throw new CommitError(`${path} has no changes to discard`);
  }
  const out: Discarded[] = [];
  for (const path of files) {
    const file = join(cwd, path);
    const before = await readFile(file).catch(() => null);
    if (await tryGit(cwd, ['cat-file', '-t', `HEAD:./${path}`])) {
      await run(cwd, ['restore', '--source=HEAD', '--staged', '--worktree', '--', path]);
    } else {
      await tryGit(cwd, ['rm', '--cached', '--quiet', '--ignore-unmatch', '--', path]);
      await rm(file, { force: true });
    }
    out.push({ path, before, after: await readFile(file).catch(() => null) });
  }
  return out;
}

/** Undo a discard: each file as it was before, refused when any of them changed since. */
export async function undoDiscard (cwd: string, kept: Discarded[]): Promise<void> {
  const same = (a: Buffer | null, b: Buffer | null) => (a && b ? a.equals(b) : a === b);
  for (const k of kept) {
    if (!same(await readFile(join(cwd, k.path)).catch(() => null), k.after)) throw new CommitError(`${k.path} changed since it was discarded: undo it in your editor`);
  }
  for (const k of kept) {
    const file = join(cwd, k.path);
    if (!k.before) {
      await rm(file, { force: true });
      continue;
    }
    await mkdir(dirname(file), { recursive: true });
    await writeFile(file, k.before);
  }
}

/** A git command whose failure (a hook that refused, say) is told as it is. */
async function run (cwd: string, args: string[]) {
  try {
    return await git(cwd, args);
  } catch (e) {
    const err = e as { stderr?: string, stdout?: string, message: string };
    const said = `${err.stderr ?? ''}\n${err.stdout ?? ''}`.trim();
    throw new CommitError(said || err.message);
  }
}

// --- Stills: each frame as the last commit drew it -----------------------------------------------

/**
 * Stills of the frames from before the changes, taken the moment a change first reaches a screen:
 * `<dir>/<commit>/<board>/<frame>.svg`. A frame's first still since the commit is the one kept;
 * a later one would already show changes.
 */
export class Stills {
  readonly dir: string;

  constructor (dir: string) {
    this.dir = dir;
  }

  private file = (head: string, board: string, frame: string) => join(this.dir, head, board, `${frame}.svg`);

  read (head: string, board: string, frame: string): Promise<Buffer> {
    return readFile(this.file(head, board, frame));
  }

  /** Keep a frame's still, unless it has one: answers whether this one was kept. */
  async keep (head: string, board: string, frame: string, svg: string): Promise<boolean> {
    const file = this.file(head, board, frame);
    await mkdir(dirname(file), { recursive: true });
    try {
      await writeFile(file, svg, { flag: 'wx' });
      return true;
    } catch (e) {
      if ((e as NodeJS.ErrnoException).code === 'EEXIST') return false;
      throw e;
    }
  }

  /** The frames with a still, by board, for this commit. Stills of any other commit are let go. */
  async list (head: string): Promise<Record<string, string[]>> {
    const names = await readdir(this.dir).catch(() => [] as string[]);
    await Promise.all(names.filter((n) => n !== head).map((n) => rm(join(this.dir, n), { recursive: true, force: true })));
    const out: Record<string, string[]> = {};
    for (const board of await readdir(join(this.dir, head)).catch(() => [] as string[])) {
      const frames = (await readdir(join(this.dir, head, board)).catch(() => [] as string[])).filter((f) => f.endsWith('.svg'));
      if (frames.length) out[board] = frames.map((f) => f.slice(0, -'.svg'.length));
    }
    return out;
  }

  /** Nothing is changed any more: nothing to compare with. */
  clear (head: string): Promise<void> {
    return rm(join(this.dir, head), { recursive: true, force: true });
  }
}
