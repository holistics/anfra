// Which of the panel's tabs fit in its strip, as a browser's devtools does it: as many as fit in
// order, the selected one always among them, and the rest behind a More tabs button.

export interface TabFit {
  shown: string[];
  more: string[];
}

/**
 * `widths` is each tab's width, in order; `gap` the space between two; `more` the More button's
 * width, with its gap. Every tab shows when they all fit; else room is kept for the button, the
 * selected tab takes its place first, and the others follow in order while they fit.
 */
export function fitTabs (widths: readonly { id: string, width: number }[], available: number, gap: number, more: number, selected?: string): TabFit {
  const total = widths.reduce((sum, tab, i) => sum + tab.width + (i ? gap : 0), 0);
  if (total <= available) return { shown: widths.map((tab) => tab.id), more: [] };
  const room = available - more;
  const keep = new Set<string>();
  let used = 0;
  const take = (tab: { id: string, width: number }, always = false) => {
    const width = tab.width + (keep.size ? gap : 0);
    if (!always && used + width > room) return false;
    keep.add(tab.id);
    used += width;
    return true;
  };
  const chosen = widths.find((tab) => tab.id === selected);
  if (chosen) take(chosen, true);
  for (const tab of widths) {
    if (keep.has(tab.id)) continue;
    if (!take(tab)) break;
  }
  return {
    shown: widths.filter((tab) => keep.has(tab.id)).map((tab) => tab.id),
    more: widths.filter((tab) => !keep.has(tab.id)).map((tab) => tab.id),
  };
}
