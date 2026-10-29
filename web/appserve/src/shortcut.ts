// A key combination ("Alt+Shift+I", the form commands declare it in) as the keys to draw, one per
// Kbd: on a Mac the modifiers are its symbols, elsewhere their names.
const MAC: Record<string, string> = { Meta: '⌘', Cmd: '⌘', Ctrl: '⌃', Control: '⌃', Alt: '⌥', Option: '⌥', Shift: '⇧' };
const OTHER: Record<string, string> = { Meta: 'Win', Cmd: 'Ctrl', Control: 'Ctrl', Option: 'Alt' };
const NAMED: Record<string, string> = {
  Escape: 'Esc', Enter: '↵', ArrowUp: '↑', ArrowDown: '↓', ArrowLeft: '←', ArrowRight: '→', Backspace: '⌫', Space: '␣',
};

export const IS_MAC = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);

export function keyLabels (combo: string, mac = IS_MAC): string[] {
  return combo.split('+').filter(Boolean).map((key) => (mac ? MAC : OTHER)[key] ?? NAMED[key] ?? (key.length === 1 ? key.toUpperCase() : key));
}
