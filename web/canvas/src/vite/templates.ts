// The templates on disk: each a folder of the app's templates dir with a template.ts, offered to
// start from. Using one copies its folder (all but template.ts) into the app's prototypes, at a
// project folder of the person's choosing, then puts its screens on a board.
import { copyFile, mkdir, readdir, readFile, stat } from 'node:fs/promises';
import { dirname, join, relative } from 'node:path';
import { placeTemplate, type Template, type TemplateSummary } from '../model/template.ts';
import { SLUG, type BoardStore } from './boards.ts';

/** A folder under the prototypes: lowercase words and dashes, nested with slashes. */
export const PROJECT = /^[a-z0-9]+(?:-[a-z0-9]+)*(?:\/[a-z0-9]+(?:-[a-z0-9]+)*)*$/;

export class Templates {
  readonly dir: string;
  private load: (file: string) => Promise<Record<string, unknown>>;
  private route: string;

  /**
   * @param dir the templates folder, absolute
   * @param load loads a module of the app (through Vite), by its absolute path
   * @param route where the templates' screens are served (`/t`), for previews
   */
  constructor (dir: string, load: (file: string) => Promise<Record<string, unknown>>, route: string) {
    this.dir = dir;
    this.load = load;
    this.route = route;
  }

  async list (): Promise<TemplateSummary[]> {
    let names: string[] = [];
    try {
      names = (await readdir(this.dir, { withFileTypes: true })).filter((d) => d.isDirectory() && !d.name.startsWith('_')).map((d) => d.name);
    } catch { /* no templates */ }
    const out: TemplateSummary[] = [];
    for (const id of names.sort()) {
      const t = await this.read(id).catch(() => undefined);
      if (t) out.push(t);
    }
    return out;
  }

  async read (id: string): Promise<TemplateSummary> {
    const file = join(this.dir, id, 'template.ts');
    if (!/^[\w-]+$/.test(id) || !await stat(file).catch(() => undefined)) {
      const ids = (await this.list()).map((t) => t.id);
      throw new Error(`no template ${id}${ids.length ? `: there are ${ids.join(', ')}` : ''}`);
    }
    const mod = await this.load(file);
    const t = (mod.template ?? mod.default) as Template | undefined;
    if (!t?.screens?.length) throw new Error(`${id}/template.ts exports no template with screens`);
    const cover = t.screens.find((s) => s.id === t.cover) ?? t.screens[0];
    return { ...t, id, preview: `${this.route}/${id}/${cover.file}${cover.query ? `?${cover.query}` : ''}` };
  }

  /**
   * Copy a template's folder into `to` (absolute). A file already there with other content stops
   * the copy before anything is written; one with the same content is left as it is.
   * Answers the files written, relative to `to`.
   */
  async copy (id: string, to: string): Promise<string[]> {
    const from = join(this.dir, id);
    const files = (await walk(from)).filter((f) => relative(from, f) !== 'template.ts');
    const write: string[] = [];
    for (const f of files) {
      const target = join(to, relative(from, f));
      const there = await readFile(target).catch(() => undefined);
      if (!there) write.push(f);
      else if (!there.equals(await readFile(f))) throw new Error(`${relative(to, target)} is already in that project: pick another project folder`);
    }
    for (const f of write) {
      const target = join(to, relative(from, f));
      await mkdir(dirname(target), { recursive: true });
      await copyFile(f, target);
    }
    return write.map((f) => relative(from, f));
  }
}

async function walk (dir: string): Promise<string[]> {
  const out: string[] = [];
  for (const d of await readdir(dir, { withFileTypes: true })) {
    const p = join(dir, d.name);
    if (d.isDirectory()) out.push(...await walk(p));
    else if ((await stat(p)).isFile()) out.push(p);
  }
  return out;
}

export interface StartFrom {
  /** The template's id. */
  template: string;
  /** The folder under the prototypes the copy goes in: `datasets/explore`. */
  project: string;
  /** The board its screens go on: made if there is none, added to if there is. */
  board: string;
  /** The board's title, when it is made. */
  name?: string;
  /** Only these screens go on the board, by id; all of them if left out. The whole folder is copied. */
  screens?: string[];
}

export interface Started {
  board: string;
  /** The files written, under the project folder. */
  files: string[];
  /** The frames made, by the template's screen ids. */
  frames: Record<string, string>;
}

/**
 * Start from a template: copy it into the project folder, then put its screens on the board.
 * `prototypes` is the prototypes folder (absolute) and the route it is served at.
 */
export async function startFrom (templates: Templates, store: BoardStore, prototypes: { dir: string, route: string }, req: StartFrom): Promise<Started> {
  if (!PROJECT.test(req.project)) throw new Error('a project folder is lowercase words and dashes, nested with slashes: datasets/explore');
  if (!SLUG.test(req.board)) throw new Error('a board name is lowercase letters, digits and dashes');
  const all = await templates.read(req.template);
  const only = req.screens?.length ? new Set(req.screens) : undefined;
  const unknown = req.screens?.filter((id) => !all.screens.some((s) => s.id === id)) ?? [];
  if (unknown.length) throw new Error(`${req.template} has no screen ${unknown.join(', ')}: it has ${all.screens.map((s) => s.id).join(', ')}`);
  const t = only ? { ...all, screens: all.screens.filter((s) => only.has(s.id)) } : all;
  const files = await templates.copy(req.template, join(prototypes.dir, req.project));
  if (!await store.exists(req.board)) await store.create(req.board, req.name ?? t.name, { project: req.project.split('/')[0] });
  let frames: Record<string, string> = {};
  await store.change(req.board, (b) => {
    const r = placeTemplate(b, t, `${prototypes.route}/${req.project}`);
    frames = r.frames;
    return r.board;
  });
  return { board: req.board, files, frames };
}
