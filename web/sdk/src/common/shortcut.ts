/**
 * Key combinations for commands' shortcuts, `Alt+Shift+I`: modifiers (`Ctrl`, `Alt`, `Shift`,
 * `Meta`) then one key. Shared by the host, which matches its own page's keys, and the frame, which
 * forwards the combinations the host registered while focus is inside it.
 */

const MODIFIERS = ['ctrl', 'alt', 'shift', 'meta'] as const;

/**
 * A combination in one canonical form, `alt+shift+i`; undefined when it isn't one. A letter or digit
 * is matched by its physical key, so `Alt+Shift+I` holds on macOS, where Alt changes the character.
 */
export function normalizeShortcut (shortcut: unknown): string | undefined {
  if (typeof shortcut !== 'string') return undefined;
  const parts = shortcut.split('+').map((part) => part.trim().toLowerCase()).filter(Boolean);
  const key = parts.pop();
  const isModifier = (part: string) => (MODIFIERS as readonly string[]).includes(part);
  if (!key || isModifier(key) || !parts.every(isModifier)) return undefined;
  return [...MODIFIERS.filter((modifier) => parts.includes(modifier)), key].join('+');
}

/** Whether a key event's target takes text, where a shortcut would eat what the reader types. */
export function isTyping (target: EventTarget | null): boolean {
  const element = target as Partial<HTMLElement> | null;
  if (!element || typeof element.tagName !== 'string') return false;
  return element.isContentEditable === true || ['INPUT', 'TEXTAREA', 'SELECT'].includes(element.tagName.toUpperCase());
}

/** The combination a key event makes, in the same form. */
export function eventShortcut (event: KeyboardEvent): string {
  const code = typeof event.code === 'string' ? event.code : '';
  const key = /^Key[A-Z]$/.test(code) ? code.slice(3).toLowerCase()
    : /^Digit[0-9]$/.test(code) ? code.slice(5)
      : (event.key ?? '').toLowerCase();
  return [
    event.ctrlKey && 'ctrl', event.altKey && 'alt', event.shiftKey && 'shift', event.metaKey && 'meta', key,
  ].filter(Boolean).join('+');
}

// The keys whose event names them in words, by their name in a combination.
const NAMED_KEYS = new Map(['Enter', 'Escape', 'Tab', 'Backspace', 'Delete', 'Insert', 'Home', 'End', 'PageUp', 'PageDown',
  'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', ...Array.from({ length: 12 }, (_, i) => `F${i + 1}`)]
  .map((name) => [name.toLowerCase(), name]));

/**
 * The key event a combination makes, as `eventShortcut` reads it back: `key` and `code` as a browser
 * sets them (`Escape`, `K` with Shift, `KeyK`), so a listener matching either sees the key it expects.
 */
export function shortcutEvent (shortcut: string): KeyboardEventInit {
  const parts = shortcut.split('+');
  const name = parts.pop()!;
  const shiftKey = parts.includes('shift');
  const named = NAMED_KEYS.get(name);
  const key = /^[a-z]$/.test(name) && shiftKey ? name.toUpperCase() : named ?? name;
  const code = /^[a-z]$/.test(name) ? `Key${name.toUpperCase()}` : /^[0-9]$/.test(name) ? `Digit${name}` : named ?? '';
  return {
    key, code, ctrlKey: parts.includes('ctrl'), altKey: parts.includes('alt'), shiftKey, metaKey: parts.includes('meta'),
  };
}
