import { afterEach, describe, expect, it } from 'vitest';
import { nextTick } from 'vue';
import { readChoice, THEME_KEY, useTheme } from './theme';

afterEach(() => useTheme().setTheme('system'));

describe('the theme', () => {
  it('reads a kept choice, and anything else as System', () => {
    expect(readChoice('dark')).toBe('dark');
    expect(readChoice('light')).toBe('light');
    expect(readChoice(null)).toBeNull();
    expect(readChoice('sepia')).toBeNull();
  });

  it('follows the device until the reader picks Light or Dark, and keeps the pick', async () => {
    const { choice, theme, setTheme } = useTheme();
    expect(choice.value).toBe('system');
    expect(theme.value).toBe('light'); // jsdom's device prefers no dark
    setTheme('dark');
    await nextTick();
    expect(theme.value).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(window.localStorage.getItem(THEME_KEY)).toBe('dark');
  });

  it('forgets the pick for System, so the device decides again', async () => {
    const { choice, setTheme } = useTheme();
    setTheme('light');
    setTheme('system');
    await nextTick();
    expect(choice.value).toBe('system');
    expect(window.localStorage.getItem(THEME_KEY)).toBeNull();
    expect(document.documentElement.dataset.theme).toBe('light');
  });
});
