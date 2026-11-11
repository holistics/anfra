// The theme the reader picked: Light, Dark, or System, which follows the device, as Cloud's foot
// offers them. A per-device preference, kept in localStorage: 'light' or 'dark', and no key for
// System, so a choice made before System existed still holds. The one in effect is <html>'s
// data-theme, which anfra-design's tokens and the `dark:` variant read.
import { usePreferredDark, useStorage } from '@vueuse/core';
import { computed, watchEffect } from 'vue';

export type ThemeChoice = 'light' | 'dark' | 'system';

export const THEME_KEY = 'anfra-appserve:theme';

/** What a stored value means: 'light' or 'dark' as picked, anything else (no key) System. */
export const readChoice = (value: string | null): 'light' | 'dark' | null => (value === 'light' || value === 'dark' ? value : null);

// null removes the key. Without storage, the choice holds for this page only.
const stored = useStorage<'light' | 'dark' | null>(THEME_KEY, null, undefined, {
  serializer: { read: readChoice, write: (value) => value ?? '' },
  onError: () => {},
});
const systemDark = usePreferredDark();

const choice = computed<ThemeChoice>(() => stored.value ?? 'system');
const theme = computed<'light' | 'dark'>(() => {
  if (choice.value === 'system') return systemDark.value ? 'dark' : 'light';
  return choice.value;
});

watchEffect(() => { document.documentElement.dataset.theme = theme.value; });

export function useTheme () {
  function setTheme (next: ThemeChoice): void {
    stored.value = next === 'system' ? null : next;
  }
  return { choice, theme, setTheme };
}
