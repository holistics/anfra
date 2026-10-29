// A key combination ("Alt+Shift+I", as a command declares it; the SDK takes any case and spaces) as
// the keys to draw, one per Kbd: on a Mac the modifiers are its symbols, elsewhere their names.
const MAC: Record<string, string> = { meta: '⌘', ctrl: '⌃', alt: '⌥', shift: '⇧' };
const OTHER: Record<string, string> = { meta: 'Win', ctrl: 'Ctrl', alt: 'Alt', shift: 'Shift' };
const NAMED: Record<string, string> = {
  escape: 'Esc', enter: '↵', arrowup: '↑', arrowdown: '↓', arrowleft: '←', arrowright: '→', backspace: '⌫', space: '␣',
};

export const IS_MAC = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);

export function keyLabels (combo: string, mac = IS_MAC): string[] {
  return combo.split('+').map((key) => key.trim()).filter(Boolean).map((key) => {
    const name = key.toLowerCase();
    return (mac ? MAC : OTHER)[name] ?? NAMED[name] ?? key.charAt(0).toUpperCase() + key.slice(1);
  });
}
