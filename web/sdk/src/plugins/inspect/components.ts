/**
 * anfra's class layer (`@holistics/anfra-design`'s `components.css`) for plugin tabs: the buttons,
 * badges, tabs, fields and tooltips anfra Cloud draws, as one stylesheet a tab's shadow root
 * carries. It reads anfra-design's Tier 2 tokens, which a page that wears anfra-design has and
 * which reach a shadow root; any other host gives only the theme's tokens, so each Tier 2 token
 * falls back to its nearest theme token.
 */
import COMPONENTS from '@holistics/anfra-design/components.css?raw';

/**
 * Each token the class layer reads, as the theme's tokens give it where the page has none; but
 * `--anfra-color-border-strong`, which is a theme token too.
 */
export const FALLBACKS: Readonly<Record<string, string>> = {
  '--anfra-color-background-default': 'var(--anfra-color-bg)',
  '--anfra-color-background-raised-default': 'var(--anfra-color-bg)',
  '--anfra-color-background-current-default': 'var(--anfra-color-bg)',
  '--anfra-color-background-secondary-default': 'var(--anfra-color-bg-sunken)',
  '--anfra-color-background-hover': 'var(--anfra-color-hover)',
  '--anfra-color-background-active': 'var(--anfra-color-active)',
  '--anfra-color-background-fill-default': 'color-mix(in oklch, var(--anfra-color-fg) 5%, transparent)',
  '--anfra-color-background-fill-hover': 'color-mix(in oklch, var(--anfra-color-fg) 9%, transparent)',
  '--anfra-color-background-fill-selected': 'var(--anfra-color-bg)',
  '--anfra-color-background-disabled': 'var(--anfra-color-hover)',
  '--anfra-color-background-highlight-default': 'var(--anfra-color-accent)',
  '--anfra-color-background-highlight-hover': 'color-mix(in oklch, var(--anfra-color-accent) 88%, var(--anfra-color-fg))',
  '--anfra-color-background-highlight-active': 'color-mix(in oklch, var(--anfra-color-accent) 76%, var(--anfra-color-fg))',
  '--anfra-color-background-highlight-secondary-default': 'var(--anfra-color-selected)',
  '--anfra-color-background-info-secondary-default': 'var(--anfra-color-hover)',
  '--anfra-color-background-danger-default': 'var(--anfra-color-danger)',
  '--anfra-color-background-danger-hover': 'color-mix(in oklch, var(--anfra-color-danger) 88%, var(--anfra-color-fg))',
  '--anfra-color-background-danger-secondary-default': 'color-mix(in srgb, var(--anfra-color-danger) 14%, transparent)',
  '--anfra-color-background-warning-secondary-default': 'color-mix(in srgb, var(--anfra-color-warning) 16%, transparent)',
  '--anfra-color-background-success-secondary-default': 'color-mix(in srgb, var(--anfra-color-success) 16%, transparent)',
  '--anfra-color-background-inverted-default': 'var(--anfra-color-fg)',
  '--anfra-color-text-default': 'var(--anfra-color-fg)',
  '--anfra-color-text-weak': 'var(--anfra-color-fg-weak)',
  '--anfra-color-text-placeholder': 'var(--anfra-color-muted)',
  '--anfra-color-text-disabled': 'color-mix(in srgb, var(--anfra-color-muted) 60%, transparent)',
  '--anfra-color-text-highlight-default': 'var(--anfra-color-accent)',
  '--anfra-color-text-link-default': 'var(--anfra-color-accent)',
  '--anfra-color-text-link-hover': 'var(--anfra-color-accent)',
  '--anfra-color-text-on-fill': 'var(--anfra-color-bg)',
  '--anfra-color-text-inverted-default': 'var(--anfra-color-bg)',
  '--anfra-color-text-info-default': 'var(--anfra-color-fg-weak)',
  '--anfra-color-text-danger-default': 'var(--anfra-color-danger)',
  '--anfra-color-text-warning-default': 'var(--anfra-color-warning)',
  '--anfra-color-text-success-default': 'var(--anfra-color-success)',
  '--anfra-color-border-default': 'var(--anfra-color-border)',
  '--anfra-color-border-weak': 'var(--anfra-color-border)',
  '--anfra-color-border-hover': 'var(--anfra-color-muted)',
  '--anfra-color-border-disabled': 'var(--anfra-color-border)',
  '--anfra-color-border-highlight-strong': 'var(--anfra-color-accent)',
  '--anfra-color-border-danger-default': 'var(--anfra-color-danger)',
  '--anfra-color-border-danger-strong': 'var(--anfra-color-danger)',
  '--anfra-color-border-warning-default': 'var(--anfra-color-warning)',
  '--anfra-color-border-success-default': 'var(--anfra-color-success)',
  '--anfra-color-focus-ring-default': 'var(--anfra-color-accent)',
  '--anfra-color-icon-weak': 'var(--anfra-color-muted)',
  '--anfra-color-icon-disabled': 'var(--anfra-color-muted)',
  '--anfra-color-icon-highlight-default': 'var(--anfra-color-accent)',
  '--anfra-e1': '0 0 0 1px var(--anfra-color-border), 0 1px 2px oklch(0 0 0 / 0.06)',
  '--anfra-e2': '0 0 0 1px var(--anfra-color-border), 0 12px 32px -12px oklch(0 0 0 / 0.3)',
  '--anfra-ease-out': 'cubic-bezier(0.22, 1, 0.36, 1)',
  '--anfra-radius-md': '6px',
  '--anfra-radius-lg': 'var(--anfra-radius)',
  '--anfra-radius-xl': '14px',
  '--anfra-radius-pill': '9999px',
  '--anfra-type-body': '400 0.875rem/1.25rem var(--anfra-font)',
  '--anfra-type-label': '500 0.875rem/1.25rem var(--anfra-font)',
  '--anfra-type-caption': '400 0.75rem/1rem var(--anfra-font)',
};

/** The tokens `css` reads, each once. */
export function tokensRead (css: string): string[] {
  return [...new Set([...css.matchAll(/var\((--anfra-[\w-]+)\)/g)].map((match) => match[1]!))];
}

/** The class layer, each token it reads falling back as `FALLBACKS` says. */
export const COMPONENT_STYLES = COMPONENTS.replace(/var\((--anfra-[\w-]+)\)/g, (whole, name: string) => (FALLBACKS[name] ? `var(${name}, ${FALLBACKS[name]})` : whole));

export { COMPONENTS };
