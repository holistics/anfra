// The sidebar's width and whether it is open on a wide window, kept per device as Cloud keeps them.
// The reader drags its edge to a width within bounds that keep both the sidebar and the viewer
// usable, and collapses it from the edge, the header's toggle, the palette or Cmd/Ctrl+\. A narrow
// window's sidebar floats over the viewer and starts closed, so it keeps neither.
import { IS_MAC } from './shortcut';

const WIDTH_KEY = 'anfra-appserve:sidebar-width';
const OPEN_KEY = 'anfra-appserve:sidebar-open';

export const DEFAULT_WIDTH = 256;
export const MIN_WIDTH = 192;
export const MAX_WIDTH = 480;

/** A drag moves at least this far; less is a click, which collapses the sidebar. */
export const DRAG_PX = 3;

/** Cmd+\ on a Mac, Ctrl+\ elsewhere, as a shortcut combination. */
export const SIDEBAR_SHORTCUT = IS_MAC ? 'Meta+\\' : 'Ctrl+\\';

export const clampWidth = (px: number): number => Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, Math.round(px)));

// Storage is read inside each try: with the site's storage blocked, even reaching localStorage
// throws, and the shell must still start.
const local = (storage?: Storage): Storage => storage ?? globalThis.localStorage;

/** The width last dragged to, or the default. */
export function storedWidth (storage?: Storage): number {
  try {
    const px = Number(local(storage).getItem(WIDTH_KEY));
    return px ? clampWidth(px) : DEFAULT_WIDTH;
  } catch {
    return DEFAULT_WIDTH; // no storage: the default
  }
}

export function rememberWidth (px: number, storage?: Storage): void {
  try {
    local(storage).setItem(WIDTH_KEY, String(clampWidth(px)));
  } catch { /* no storage: it holds for this page only */ }
}

/** Whether the wide sidebar was left open: open unless the reader collapsed it. */
export function storedOpen (storage?: Storage): boolean {
  try {
    return local(storage).getItem(OPEN_KEY) !== 'false';
  } catch {
    return true;
  }
}

export function rememberOpen (open: boolean, storage?: Storage): void {
  try {
    local(storage).setItem(OPEN_KEY, String(open));
  } catch { /* no storage: it holds for this page only */ }
}

/** Whether a key press toggles the sidebar: Cmd+\ or Ctrl+\, nothing else held, as Cloud's sidebar and the palette's Cmd+K take theirs. */
export function isSidebarKey (event: KeyboardEvent): boolean {
  return (event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && event.key === '\\';
}
