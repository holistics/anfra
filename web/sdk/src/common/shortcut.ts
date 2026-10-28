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
