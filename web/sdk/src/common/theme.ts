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
  'color-bg-sunken', 'color-fg-weak', 'color-border-strong', 'color-hover', 'color-active', 'color-selected',
  'color-warning', 'color-success',
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

const FONT = "'Inter Variable', ui-sans-serif, system-ui, sans-serif";
const FONT_MONO = "'Geist Mono Variable', ui-monospace, monospace";

/**
 * Each scheme's tokens where the host sets none: anfra's own (`@holistics/anfra-design`'s
 * `tokens.css`, resolved), so plugin UI looks like anfra in a host that maps nothing. A test
 * holds them to that file.
 */
export const DEFAULT_TOKENS: Readonly<Record<ThemeScheme, Readonly<ThemeTokens>>> = {
  light: {
    'color-bg': 'oklch(1 0 0)',
    'color-bg-sunken': 'oklch(0.973 0.005 95.1)',
    'color-fg': 'oklch(0.201 0.022 301.1)',
    'color-fg-weak': 'oklch(0.392 0.021 304.3)',
    'color-muted': 'oklch(0.485 0.022 306.3)',
    'color-border': 'oklch(0.201 0.022 301.1 / 0.12)',
    'color-border-strong': 'oklch(0.201 0.022 301.1 / 0.24)',
    'color-accent': 'oklch(0.484 0.181 298.1)',
    'color-danger': 'oklch(0.501 0.17 27.2)',
    'color-warning': 'oklch(0.501 0.1 70.3)',
    'color-success': 'oklch(0.479 0.1 151.8)',
    'color-hover': 'oklch(0.201 0.022 301.1 / 0.05)',
    'color-active': 'oklch(0.201 0.022 301.1 / 0.09)',
    'color-selected': 'oklch(0.946 0.027 304.8)',
    radius: '10px',
    font: FONT,
    'font-mono': FONT_MONO,
  },
  dark: {
    'color-bg': 'oklch(0.19 0.012 300)',
    'color-bg-sunken': 'oklch(0.157 0.009 296.9)',
    'color-fg': 'oklch(0.961 0.007 304.2)',
    'color-fg-weak': 'oklch(0.879 0.016 302.5)',
    'color-muted': 'oklch(0.835 0.018 300)',
    'color-border': 'oklch(0.343 0.025 298.2)',
    'color-border-strong': 'oklch(0.392 0.021 304.3)',
    'color-accent': 'oklch(0.845 0.085 300)',
    'color-danger': 'oklch(0.82 0.09 27)',
    'color-warning': 'oklch(0.84 0.12 69.6)',
    'color-success': 'oklch(0.8 0.12 151.9)',
    'color-hover': 'oklch(1 0 0 / 0.06)',
    'color-active': 'oklch(1 0 0 / 0.1)',
    'color-selected': 'oklch(0.565 0.194 300.3 / 0.18)',
    radius: '10px',
    font: FONT,
    'font-mono': FONT_MONO,
  },
};

/**
 * Where each token comes from in `@holistics/anfra-design`: the custom property a host that uses
 * it has on its page. One mapping, so every such host hands plugin UI the same theme.
 */
export const DESIGN_TOKENS: Readonly<Record<ThemeToken, string>> = {
  'color-bg': '--anfra-color-background-default',
  'color-bg-sunken': '--anfra-color-background-secondary-default',
  'color-fg': '--anfra-color-text-default',
  'color-fg-weak': '--anfra-color-text-weak',
  'color-muted': '--anfra-color-text-placeholder',
  'color-border': '--anfra-color-border-default',
  'color-border-strong': '--anfra-color-border-strong',
  'color-accent': '--anfra-color-text-highlight-default',
  'color-danger': '--anfra-color-text-danger-default',
  'color-warning': '--anfra-color-text-warning-default',
  'color-success': '--anfra-color-text-success-default',
  'color-hover': '--anfra-color-background-hover',
  'color-active': '--anfra-color-background-active',
  'color-selected': '--anfra-color-background-interactive-highlight-default',
  radius: '--anfra-radius-lg',
  font: '--anfra-font-sans',
  'font-mono': '--anfra-font-mono',
};

/**
 * A theme from a page that wears `@holistics/anfra-design`: each token read from its custom
 * property (`DESIGN_TOKENS`), as `getComputedStyle(document.documentElement)` has it. A host calls
 * it again when its scheme changes.
 */
export function designTheme (scheme: ThemeScheme, style: Pick<CSSStyleDeclaration, 'getPropertyValue'>): ThemeInput {
  const tokens: Partial<ThemeTokens> = {};
  for (const name of THEME_TOKENS) {
    const value = style.getPropertyValue(DESIGN_TOKENS[name]).trim();
    if (value) tokens[name] = value;
  }
  return { scheme, tokens };
}

// A token's value goes into a style attribute: no way out of its declaration, nor a URL to fetch.
const UNSAFE_VALUE = /[;{}<>]|url\(|expression\(|@import|\\/i;

/**
 * A theme with every token, from what a host or the bridge gives: the scheme's defaults under the
 * tokens set, and each value that isn't a short, plain CSS value left at its default. A newer token
 * the host leaves out is mixed from the older ones it sets (`DERIVED`), so it follows the host.
 */
export function resolveTheme (input: unknown): Theme {
  const given = (typeof input === 'object' && input !== null ? input : {}) as { scheme?: unknown, tokens?: unknown };
  const scheme: ThemeScheme = given.scheme === 'dark' ? 'dark' : 'light';
  const tokens: ThemeTokens = { ...DEFAULT_TOKENS[scheme] };
  const set = (typeof given.tokens === 'object' && given.tokens !== null ? given.tokens : {}) as Record<string, unknown>;
  const taken = new Set<ThemeToken>();
  for (const name of THEME_TOKENS) {
    const value = set[name];
    if (typeof value === 'string' && value.trim() !== '' && value.length <= 200 && !UNSAFE_VALUE.test(value)) {
      tokens[name] = value.trim();
      taken.add(name);
    }
  }
  for (const [name, { from, value }] of Object.entries(DERIVED) as [ThemeToken, Derived][]) {
    if (taken.has(name) || !from.some((source) => taken.has(source))) continue;
    const derived = value(tokens);
    if (derived.length <= 200) tokens[name] = derived;
  }
  return Object.freeze({ scheme, tokens: Object.freeze(tokens) });
}

interface Derived { from: ThemeToken[], value: (tokens: ThemeTokens) => string }

/**
 * The tokens added after the first nine, from those nine, for a host that maps the first nine and
 * not these: its own ink and accent, mixed, rather than anfra's paper and violet beside its colours.
 * The status colours keep anfra's.
 */
const DERIVED: Partial<Record<ThemeToken, Derived>> = {
  'color-bg-sunken': { from: ['color-fg', 'color-bg'], value: (t) => `color-mix(in srgb, ${t['color-fg']} 4%, ${t['color-bg']})` },
  'color-fg-weak': { from: ['color-fg', 'color-bg'], value: (t) => `color-mix(in srgb, ${t['color-fg']} 80%, ${t['color-bg']})` },
  'color-border-strong': { from: ['color-fg', 'color-border'], value: (t) => `color-mix(in srgb, ${t['color-fg']} 10%, ${t['color-border']})` },
  'color-hover': { from: ['color-fg'], value: (t) => `color-mix(in srgb, ${t['color-fg']} 7%, transparent)` },
  'color-active': { from: ['color-fg'], value: (t) => `color-mix(in srgb, ${t['color-fg']} 12%, transparent)` },
  'color-selected': { from: ['color-accent'], value: (t) => `color-mix(in srgb, ${t['color-accent']} 14%, transparent)` },
};

/** Set a theme's custom properties, and its `color-scheme`, on an element's style. */
export function applyTheme (style: CSSStyleDeclaration, theme: Theme): void {
  for (const name of THEME_TOKENS) style.setProperty(`--anfra-${name}`, theme.tokens[name]);
  style.setProperty('color-scheme', theme.scheme);
}
