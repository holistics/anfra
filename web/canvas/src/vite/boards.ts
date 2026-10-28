// The boards on disk: one JSON file each in the app's boards folder. Every save goes through here,
// one at a time per board, and bumps the board's rev; a save from an older rev is refused with the
// current board, for the saver to merge onto and save again.
import { mkdir, readdir, readFile, stat, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { emptyBoard, parseBoard, type Board } from '../model/board.ts';

export const SLUG = /^[a-z0-9][a-z0-9-]{0,63}$/;

export interface BoardSummary {
  slug: string;
  name: string;
  frames: number;
  open: number;
  project?: string;
  owners?: string[];
  /** When its file last changed, in ms. */
  updated: number;
  /** Its screens' routes, and names, for search and the project a board without one is in. */
  routes: string[];
  screens: string[];
  /** Its first frame, which presenting the board starts at. */
  first?: string;
  /** A small map of the board for its card: sections and frames, as x, y, width, height. */
  map: { sections: number[][], frames: number[][] };
}

/** A board's details, as its home card edits them. */
export interface BoardDetails {
  name?: string;
  project?: string;
  owners?: string[];
}

export function withDetails (board: Board, details: BoardDetails): Board {
  const next: Board = { ...board };
  if (typeof details.name === 'string' && details.name.trim()) next.name = details.name.trim();
  if ('project' in details) {
    if (details.project?.trim()) next.project = details.project.trim();
    else delete next.project;
  }
  if ('owners' in details) {
    const owners = (details.owners ?? []).filter((o) => typeof o === 'string' && o.trim());
    if (owners.length) next.owners = owners;
    else delete next.owners;
  }
  return next;
}

const rect = (r: { x: number, y: number, width: number, height: number }) => [r.x, r.y, r.width, r.height].map(Math.round);

export class Conflict extends Error {
  readonly current: Board;

  constructor (current: Board) {
    super('the board changed since it was read');
    this.current = current;
  }
}

export class BoardStore {
  private queue = new Map<string, Promise<unknown>>();
  /** What this process last wrote per board, to tell its own writes from others' in the watcher. */
  readonly written = new Map<string, string>();

  readonly dir: string;
  private readonly changed: (slug: string, board: Board) => void;

  constructor (dir: string, changed: (slug: string, board: Board) => void) {
    this.dir = dir;
    this.changed = changed;
  }

  path (slug: string): string {
    if (!SLUG.test(slug)) throw new Error(`"${slug}" is not a board name: lowercase letters, digits and dashes`);
    return join(this.dir, `${slug}.json`);
  }

  async list (): Promise<BoardSummary[]> {
    let files: string[] = [];
    try {
      files = (await readdir(this.dir)).filter((f) => f.endsWith('.json'));
    } catch { /* no boards yet */ }
    const out: BoardSummary[] = [];
    for (const f of files.sort()) {
      const slug = f.slice(0, -5);
      try {
        const b = await this.read(slug);
        const { mtimeMs } = await stat(this.path(slug));
        out.push({
          slug,
          name: b.name,
          frames: b.frames.length,
          open: b.comments.filter((c) => c.status === 'open').length,
          ...(b.project ? { project: b.project } : {}),
          ...(b.owners ? { owners: b.owners } : {}),
          updated: Math.round(mtimeMs),
          routes: [...new Set(b.frames.map((f) => f.route))],
          screens: b.frames.map((f) => f.name),
          ...(b.frames[0] ? { first: b.frames[0].id } : {}),
          map: { sections: b.sections.map(rect), frames: b.frames.map(rect) },
        });
      } catch { /* a broken file is skipped here and reported when opened */ }
    }
    return out;
  }

  async read (slug: string): Promise<Board> {
    return parseBoard(JSON.parse(await readFile(this.path(slug), 'utf8')));
  }

  async exists (slug: string): Promise<boolean> {
    try {
      await readFile(this.path(slug));
      return true;
    } catch {
      return false;
    }
  }

  async create (slug: string, name: string, details: BoardDetails = {}): Promise<Board> {
    return this.serial(slug, async () => {
      if (await this.exists(slug)) throw new Error(`a board named ${slug} exists`);
      return this.write(slug, withDetails(emptyBoard(name), details));
    });
  }

  /** Save a whole board that was edited from rev `base`; refused with a Conflict if it moved on. */
  async save (slug: string, board: Board, base: number): Promise<Board> {
    return this.serial(slug, async () => {
      const current = await this.read(slug);
      if (current.rev !== base) throw new Conflict(current);
      return this.write(slug, { ...board, rev: current.rev });
    });
  }

  /** Change the latest board: how the MCP tools write, so they never act on a stale one. */
  async change (slug: string, fn: (b: Board) => Board): Promise<Board> {
    return this.serial(slug, async () => this.write(slug, fn(await this.read(slug))));
  }

  private async write (slug: string, board: Board): Promise<Board> {
    const next = parseBoard({ ...board, rev: board.rev + 1 });
    const text = `${JSON.stringify(next, null, 2)}\n`;
    await mkdir(this.dir, { recursive: true });
    this.written.set(slug, text);
    await writeFile(this.path(slug), text);
    this.changed(slug, next);
    return next;
  }

  private serial<T> (slug: string, fn: () => Promise<T>): Promise<T> {
    const run = (this.queue.get(slug) ?? Promise.resolve()).then(fn, fn);
    this.queue.set(slug, run.catch(() => {}));
    return run;
  }
}
