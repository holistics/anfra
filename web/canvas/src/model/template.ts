// A template is a prototype to start from: a folder of screens (and the parts only they use) with a
// template.ts saying what it is and how its screens sit on a board. Using one copies the folder into
// the app's prototypes, where it is the person's own to change, and puts its screens on a board.
import { addArrow, addFrame, addSection, bounds } from './ops.ts';
import { tidy } from './layout.ts';
import type { Board } from './board.ts';

export interface TemplateScreen {
  /** How arrows name it. */
  id: string;
  /** Its file in the folder, without `.vue`: `DatasetExploration`, `views/Detail`. */
  file: string;
  /** Its frame's label. */
  name: string;
  /** Added to its route: `datasetId=ecommerce_main`, `open=delete`. */
  query?: string;
  /** Its frame's scenario: `default` if left out. */
  scenario?: string;
  width?: number;
  height?: number;
}

export interface Template {
  name: string;
  description: string;
  /** Where in the product it is: Reporting, Development, AI, Settings. */
  area?: string;
  /** The screen shown on its card; the first if left out. */
  cover?: string;
  screens: TemplateScreen[];
  /** The flow between its screens, by screen id. */
  arrows?: { from: string, to: string, label?: string }[];
}

/** A template as the canvas offers it: its folder's name is its id. */
export interface TemplateSummary extends Template {
  id: string;
  /** Where its cover screen is served as it is in the template, for a preview. */
  preview: string;
}

/** A screen's route once the template is copied to `base` (`/p/datasets/explore`). */
export const screenRoute = (base: string, s: TemplateScreen) => `${base}/${s.file}${s.query ? `?${s.query}` : ''}`;

/**
 * The template's screens put on a board as frames of the copy at `base`, with its arrows, laid out
 * by the flow and, when there are several, grouped in a section named after it. Answers the
 * frames' ids by screen id.
 */
export function placeTemplate (board: Board, t: Template, base: string): { board: Board, frames: Record<string, string> } {
  let b = board;
  const frames: Record<string, string> = {};
  const before = new Set(b.frames.map((f) => f.id));
  for (const s of t.screens) {
    const r = addFrame(b, { route: screenRoute(base, s), name: s.name, scenario: s.scenario ?? 'default', width: s.width ?? 1440, height: s.height ?? 900 });
    b = r.board;
    frames[s.id] = r.frame.id;
  }
  for (const a of t.arrows ?? []) {
    if (frames[a.from] && frames[a.to]) b = addArrow(b, frames[a.from], frames[a.to], a.label).board;
  }
  // Lay out only what was just added, beside what the board had.
  const kept = b.frames.filter((f) => before.has(f.id));
  if (!kept.length) b = tidy(b);
  const added = b.frames.filter((f) => !before.has(f.id));
  const box = bounds(added);
  if (box && added.length > 1) b = addSection(b, { name: t.name, x: box.x - 80, y: box.y - 120, width: box.width + 160, height: box.height + 200 }).board;
  return { board: b, frames };
}
