/**
 * The theme a host hands plugin UI: a light or dark scheme, and a fixed set of tokens, each a CSS
 * custom property `--anfra-<token>`. A host maps its own design tokens onto them once; tabs, and
 * frame halves in the plugin layer, use only these, so every plugin's UI follows every host. A
 * definition may read them (`Anfra.theme`), and is never styled by them unasked.
 */

export type ThemeScheme = 'light' | 'dark';

/** The tokens, by name: each is the CSS custom property `--anfra-<name>`. */
export const THEME_TOKENS = [
  'color-bg', 'color-fg', 'color-muted', 'color-border', 'color-accent', 'color-danger', 'radius', 'font', 'font-mono',
] as const;

export type ThemeToken = typeof THEME_TOKENS[number];

export type ThemeTokens = Record<ThemeToken, string>;

/** A theme as a host gives it: the scheme, and the tokens it sets (the others keep the scheme's defaults). */
export interface ThemeInput {
  scheme: ThemeScheme;
  tokens?: Partial<ThemeTokens>;
}

/** A theme with every token: what plugin UI and `Anfra.theme` read. */
export interface Theme {
  readonly scheme: ThemeScheme;
  readonly tokens: Readonly<ThemeTokens>;
}

const FONT = 'system-ui, -apple-system, "Segoe UI", sans-serif';
const FONT_MONO = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

/** Each scheme's tokens where the host sets none. */
export const DEFAULT_TOKENS: Readonly<Record<ThemeScheme, Readonly<ThemeTokens>>> = {
  light: {
    'color-bg': '#ffffff',
    'color-fg': '#18181b',
    'color-muted': '#71717a',
    'color-border': '#e4e4e7',
    'color-accent': '#5b3df5',
    'color-danger': '#c4261d',
    radius: '8px',
    font: FONT,
    'font-mono': FONT_MONO,
  },
  dark: {
    'color-bg': '#1c1c20',
    'color-fg': '#ececef',
    'color-muted': '#9a9aa5',
    'color-border': '#2f2f36',
    'color-accent': '#a594ff',
    'color-danger': '#f97066',
    radius: '8px',
    font: FONT,
    'font-mono': FONT_MONO,
  },
};

// A token's value goes into a style attribute: no way out of its declaration, nor a URL to fetch.
const UNSAFE_VALUE = /[;{}<>]|url\(|expression\(|@import|\\/i;

/**
 * A theme with every token, from what a host or the bridge gives: the scheme's defaults under the
 * tokens set, and each value that isn't a short, plain CSS value left at its default.
 */
export function resolveTheme (input: unknown): Theme {
  const given = (typeof input === 'object' && input !== null ? input : {}) as { scheme?: unknown, tokens?: unknown };
  const scheme: ThemeScheme = given.scheme === 'dark' ? 'dark' : 'light';
  const tokens: ThemeTokens = { ...DEFAULT_TOKENS[scheme] };
  const set = (typeof given.tokens === 'object' && given.tokens !== null ? given.tokens : {}) as Record<string, unknown>;
  for (const name of THEME_TOKENS) {
    const value = set[name];
    if (typeof value === 'string' && value.trim() !== '' && value.length <= 200 && !UNSAFE_VALUE.test(value)) tokens[name] = value.trim();
  }
  return Object.freeze({ scheme, tokens: Object.freeze(tokens) });
}

/** Set a theme's custom properties, and its `color-scheme`, on an element's style. */
export function applyTheme (style: CSSStyleDeclaration, theme: Theme): void {
  for (const name of THEME_TOKENS) style.setProperty(`--anfra-${name}`, theme.tokens[name]);
  style.setProperty('color-scheme', theme.scheme);
}
