// The panels beside the board: how wide each is, and whether it shows. They are each person's own,
// kept in the browser, so the canvas opens as they left it.

export type Side = 'left' | 'right';

export interface Panels {
  left: number;
  right: number;
  leftOpen: boolean;
  rightOpen: boolean;
}

/** Each panel's narrowest, widest and first width, in px. */
export const PANEL_WIDTH: Record<Side, { min: number, max: number, initial: number }> = {
  left: { min: 220, max: 480, initial: 288 },
  right: { min: 248, max: 480, initial: 296 },
};

const KEY = 'anfra-canvas:panels';

export const clampWidth = (side: Side, width: number) =>
  Math.round(Math.min(PANEL_WIDTH[side].max, Math.max(PANEL_WIDTH[side].min, width)));

export function loadPanels (): Panels {
  const panels: Panels = { left: PANEL_WIDTH.left.initial, right: PANEL_WIDTH.right.initial, leftOpen: true, rightOpen: true };
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) ?? '{}') as Partial<Panels>;
    for (const side of ['left', 'right'] as const) {
      if (typeof saved[side] === 'number') panels[side] = clampWidth(side, saved[side]);
      if (typeof saved[`${side}Open`] === 'boolean') panels[`${side}Open`] = saved[`${side}Open`]!;
    }
  } catch {
    // No storage (a private window): the first widths, both shown.
  }
  return panels;
}

export function savePanels (panels: Panels) {
  try {
    localStorage.setItem(KEY, JSON.stringify(panels));
  } catch {
    // Not kept: the canvas opens with the first widths next time.
  }
}

export function togglePanel (panels: Panels, side: Side) {
  panels[`${side}Open`] = !panels[`${side}Open`];
}

/** Hide both, for the board alone; or, when both are hidden, show both. */
export function togglePanels (panels: Panels) {
  const show = !panels.leftOpen && !panels.rightOpen;
  panels.leftOpen = show;
  panels.rightOpen = show;
}
