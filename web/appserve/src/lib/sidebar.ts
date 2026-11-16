// The sidebar's width and whether it is open on a wide window, kept per device as Cloud keeps them.
// The reader drags its edge to a width within bounds that keep both the sidebar and the viewer
// usable, and collapses it from the edge, the header's toggle, the palette or Cmd/Ctrl+\. A narrow
// window's sidebar floats over the viewer and starts closed, so it keeps neither. Kept through
// vueuse's useStorage, as the theme is: with the site's storage blocked, the defaults hold for the
// page.
import { useStorage } from '@vueuse/core';
import { IS_MAC } from './shortcut';

export const DEFAULT_WIDTH = 256;
export const MIN_WIDTH = 192;
export const MAX_WIDTH = 480;

/** Cmd+\ on a Mac, Ctrl+\ elsewhere, as a shortcut combination. */
export const SIDEBAR_SHORTCUT = IS_MAC ? 'Meta+\\' : 'Ctrl+\\';

export const clampWidth = (px: number): number => Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, Math.round(px)));

/** What a kept width means: a number within bounds, anything else the default. */
export const readWidth = (value: string | null): number => {
  const px = Number(value);
  return px ? clampWidth(px) : DEFAULT_WIDTH;
};

const quiet = { writeDefaults: false, onError: () => {} };

/** The width last dragged to. Written when a drag ends, not on every move. */
export const keptWidth = useStorage('anfra-appserve:sidebar-width', DEFAULT_WIDTH, undefined, {
  ...quiet,
  serializer: { read: readWidth, write: (px) => String(clampWidth(px)) },
});

/** Whether the wide sidebar was left open: open unless the reader collapsed it. */
export const keptOpen = useStorage('anfra-appserve:sidebar-open', true, undefined, {
  ...quiet,
  serializer: { read: (value) => value !== 'false', write: String },
});

/**
 * Whether a key press toggles the sidebar: Cmd or Ctrl with \, nothing else held, as Cloud's
 * sidebar and the palette's Cmd/Ctrl+K take theirs. The key is matched by what it types or by
 * where it sits (`code`), so a layout that types \ with AltGr, which Windows reports as Ctrl+Alt,
 * reaches it on the key where US keyboards have \, while AltGr+\ alone still just types a
 * backslash. A held key's repeats don't toggle it again and again.
 */
export function isSidebarKey (event: KeyboardEvent): boolean {
  if (event.repeat || event.altKey || event.shiftKey || !(event.metaKey || event.ctrlKey)) return false;
  return event.key === '\\' || event.code === 'Backslash';
}
