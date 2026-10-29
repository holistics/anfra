---
version: alpha
name: anfra
description: anfra's product UI, in anfra core (anfra serve, the SDK's plugin UI) and anfra Cloud. Paper and ink with one violet accent, dense and quiet, on the anfra brand.
colors:
  primary: 'oklch(0.565 0.194 300.3)'
  primary-hover: 'oklch(0.517 0.192 299.1)'
  on-primary: 'oklch(1 0 0)'
  surface: 'oklch(1 0 0)'
  surface-chrome: 'oklch(0.955 0.007 88.6)'
  surface-sunken: 'oklch(0.973 0.005 95.1)'
  on-surface: 'oklch(0.201 0.022 301.1)'
  on-surface-weak: 'oklch(0.392 0.021 304.3)'
  placeholder: 'oklch(0.485 0.022 306.3)'
  disabled-surface: 'oklch(0.201 0.022 301.1 / 0.04)'
  disabled-text: 'oklch(0.723 0.017 306.3)'
  link: 'oklch(0.484 0.181 298.1)'
  selected: 'oklch(0.946 0.027 304.8)'
  inverse-surface: 'oklch(0.201 0.022 301.1)'
  inverse-on-surface: 'oklch(0.961 0.007 304.2)'
  error: 'oklch(0.549 0.199 27.1)'
  error-text: 'oklch(0.501 0.17 27.2)'
  error-container: 'oklch(0.971 0.014 22.8)'
  warning-text: 'oklch(0.501 0.1 70.3)'
  warning-container: 'oklch(0.973 0.028 84.6)'
  success-text: 'oklch(0.479 0.1 151.8)'
  success-container: 'oklch(0.972 0.021 152.2)'
typography:
  title:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: 600
    lineHeight: 28px
  heading:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 600
    lineHeight: 20px
  body:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 400
    lineHeight: 20px
  label:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: 500
    lineHeight: 20px
  caption:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: 400
    lineHeight: 16px
  code:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: 400
    lineHeight: 28px
    letterSpacing: 0.5em
    fontFeature: '"tnum"'
  mono-label:
    fontFamily: Geist Mono
    fontSize: 12px
    fontWeight: 400
    lineHeight: 16px
    letterSpacing: 0.025em
rounded:
  md: 6px
  lg: 10px
  xl: 14px
  pill: 9999px
spacing:
  unit: 4px
  control-height: 32px
  field-gap: 8px
  form-gap: 24px
  panel-inset: 8px
  sheet-padding: 24px
  sheet-width: 384px
  logomark-sheet: 48px
components:
  button-primary:
    backgroundColor: '{colors.primary}'
    textColor: '{colors.on-primary}'
    typography: '{typography.label}'
    rounded: '{rounded.lg}'
    height: '{spacing.control-height}'
    padding: 10px
  button-primary-hover:
    backgroundColor: '{colors.primary-hover}'
    textColor: '{colors.on-primary}'
  button-outline:
    backgroundColor: '{colors.surface}'
    textColor: '{colors.on-surface}'
    typography: '{typography.label}'
    rounded: '{rounded.lg}'
    height: '{spacing.control-height}'
  button-ghost:
    textColor: '{colors.on-surface-weak}'
    typography: '{typography.label}'
    rounded: '{rounded.lg}'
    height: '{spacing.control-height}'
  button-link:
    textColor: '{colors.link}'
    typography: '{typography.label}'
    height: '{spacing.control-height}'
  button-disabled:
    backgroundColor: '{colors.disabled-surface}'
    textColor: '{colors.disabled-text}'
  input:
    backgroundColor: '{colors.surface}'
    textColor: '{colors.on-surface}'
    typography: '{typography.body}'
    rounded: '{rounded.lg}'
    height: '{spacing.control-height}'
    padding: 10px
  input-placeholder:
    backgroundColor: '{colors.surface}'
    textColor: '{colors.placeholder}'
  input-code:
    typography: '{typography.code}'
    height: '{spacing.control-height}'
  panel:
    backgroundColor: '{colors.surface}'
    textColor: '{colors.on-surface}'
    rounded: '{rounded.xl}'
  chrome:
    backgroundColor: '{colors.surface-chrome}'
    textColor: '{colors.on-surface}'
  well:
    backgroundColor: '{colors.surface-sunken}'
    textColor: '{colors.on-surface-weak}'
  sheet:
    backgroundColor: '{colors.surface}'
    textColor: '{colors.on-surface}'
    rounded: '{rounded.xl}'
    padding: '{spacing.sheet-padding}'
    width: '{spacing.sheet-width}'
  row-selected:
    backgroundColor: '{colors.selected}'
    textColor: '{colors.on-surface}'
  tooltip:
    backgroundColor: '{colors.inverse-surface}'
    textColor: '{colors.inverse-on-surface}'
    typography: '{typography.caption}'
    rounded: '{rounded.md}'
  alert-danger:
    backgroundColor: '{colors.error-container}'
    textColor: '{colors.on-surface}'
    rounded: '{rounded.xl}'
  field-error:
    textColor: '{colors.error-text}'
    typography: '{typography.body}'
  button-destructive:
    backgroundColor: '{colors.error}'
    textColor: '{colors.on-primary}'
    typography: '{typography.label}'
    rounded: '{rounded.lg}'
  status-warning:
    backgroundColor: '{colors.warning-container}'
    textColor: '{colors.warning-text}'
    typography: '{typography.caption}'
    rounded: '{rounded.pill}'
  status-success:
    backgroundColor: '{colors.success-container}'
    textColor: '{colors.success-text}'
    typography: '{typography.caption}'
    rounded: '{rounded.pill}'
  mono-label:
    textColor: '{colors.placeholder}'
    typography: '{typography.mono-label}'
---

# anfra design

## Overview

anfra's product UI is a working notebook on warm paper: ink for everything, one violet pen for the one thing the reader follows. It is a tool for analysts who read models, metrics and query results all day, so it is dense, calm and exact. Nothing glows, nothing floats without a reason, and no screen tries to impress.

- **Paper and ink first.** Paper chrome frames a white work panel. Text, rules and icons are ink.
- **One violet.** Violet marks the one primary action of a view, links, focus and selection. It is never decoration.
- **Drawn, not decorated.** Hairlines carry structure. Shadows only lift what floats.
- **One density.** Every screen uses the same control height, type scale and spacing.

**One language, two apps.** This file holds the rules every anfra screen follows, in anfra core (`anfra serve`'s pages in `web/appserve`, and the plugin UI the SDK draws in any host) and in anfra Cloud. It is public, and it is the source: anfra Cloud's `web/DESIGN.md` extends it with Cloud-only rules (its signed-out pages, its forms) and its Tailwind and shadcn-vue notes, and never restates or overrides a rule here. The brand's principles, logo and marketing pages live in the brand file (`holistics/anfra-landing`, `DESIGN.md` on `main`).

The front matter holds the light theme's values. The implementation is `tokens.css`, `fonts.css` and `type.css` beside this file, published as `@holistics/anfra-design`; it holds both themes. A change goes into both, here first.

## Colors

Colors are in `tokens.css`, in two tiers, after the Holistics Design System. A host adds its own names on top (a third tier), and never a color of its own.

| Tier | Names | Used by |
| --- | --- | --- |
| 1 Primitives | `--anfra-{ink,paper,violet,red,amber,green,white,black}-{step}`, OKLCH | Tier 2 only |
| 2 Semantic | `--anfra-color-{scope}-{semantic}-{state}`, scope = `background`, `text`, `icon`, `border`, `focus-ring` | Every component, in every host |
| 3 Host aliases | Cloud: Tailwind's `text-weak`, `bg-hover`, shadcn's `--primary`. appserve: its stylesheet | That host's components |

The same word is a different color for each scope: `--anfra-color-text-weak`, `--anfra-color-background-hover`, `--anfra-color-border-strong`, `--anfra-color-icon-weak`. Below, a token is named without its `--anfra-color-` prefix.

- **Paper** `background-base-default` {colors.surface-chrome} is the chrome: the sidebar and the frame around the panel. **White** `background-default` {colors.surface} is the work panel, cards and dialogs. **Sunken paper** `background-secondary-default` {colors.surface-sunken} is a well inside the panel: code, read-only fields, card footers.
- **Ink** `text-default` {colors.on-surface} carries titles, body text and values. **Weak ink** `text-weak` {colors.on-surface-weak} is secondary text and quiet controls. **Muted ink** `text-placeholder` {colors.placeholder} is the one muted color: descriptions, captions, muted labels and placeholders. There is no second muted color.
- **Violet** {colors.primary} is the one accent: `background-highlight-default` for the one filled button of a view, `text-highlight-default` or `text-link-default` {colors.link} for violet text. The violet wash `background-interactive-highlight-default` {colors.selected} marks a selected row or option, always with a checked control and `aria-selected`.
- **Status** is restrained red, amber and green; info is neutral ink, never blue or violet. A status always has a label or an icon, and never fills a large surface: `background-{danger,warning,success,info}-secondary-default` behind `text-{s}-default`.
- **Disabled** is `background-disabled` and `text-disabled`, plus `border-disabled` on a control with a border. Never lower the opacity instead.
- **Neutral states** are `background-hover` and `background-active`; they work on paper and on white.
- **Errors**: `text-danger-default` {colors.error-text} for error text; `background-danger-default` {colors.error} is the solid fill, never text.
- **Dark** redefines Tier 2 only, under `.dark` or `[data-theme="dark"]` on `<html>`. A host follows the system by default; light and dark are choices, stored per device.

## Typography

Inter for all text; Geist Mono for code, revisions, mono labels and names the user writes in code. Both are bundled (`fonts.css`, `--anfra-font-sans`, `--anfra-font-mono`), never loaded from a CDN: an on-prem install, or `anfra serve` on a laptop, may be offline.

A screen uses three sizes, 20, 14 and 12px: to add emphasis, change weight or color, not size. `type.css` has each role as a `font` shorthand (`font: var(--anfra-type-body)`).

| Role | Token | Size / leading | Weight | Use |
| --- | --- | --- | --- | --- |
| title | {typography.title} | 20 / 28px | 600 | The one heading of a page, a step or a dialog |
| heading | {typography.heading} | 14 / 20px | 600 | Section headings, card titles |
| body | {typography.body} | 14 / 20px | 400, 450 in dark | Descriptions, field text, alerts, list rows, menu items |
| label | {typography.label} | 14 / 20px | 500 | Buttons, links, field labels, the current page, names |
| caption | {typography.caption} | 12 / 16px | 400 | Dividers, notes under a button, roles, timestamps, tooltips, shortcuts |
| mono-label | {typography.mono-label} | 12 / 16px | 400 | Uppercase kinds and revisions |

- **No other weights**, except: caption at 500 for badges, status pills, group labels and initial tiles; 500 on a name inside a sentence; 500 on the current item of a nav.
- **A new role gets a new name**, never a size suffix (`heading`, not `body-lg`).
- **A name in a sentence** (an organization slug, an address) is Inter 500, never mono.
- Tabular numbers wherever numbers change or line up.

## Layout

One density for every screen, on a 4px unit {spacing.unit}.

- **Work screens**: the paper chrome holds the sidebar; the white panel is inset {spacing.panel-inset} on the paper, with a 14px radius, a 1px `border-default` and no shadow. The header bar is navigation (a breadcrumb); the page's subject is its own heading.
- **One control height**: {spacing.control-height} for every field and button.
- **A panel docked beside the work** (the plugins' panel) sits inside the white panel, behind a 1px `border-default`, with its tabs in a 48px header.

## Elevation & Depth

Depth comes from tone and hairlines: paper behind, white in front, 1px rules between. Shadows are neutral ink, never colored, and only two exist: `--anfra-e1` for a resting card, `--anfra-e2` for what floats (menus, popovers, dialogs). On dark, both become a border.

## Shapes

Soft, not round. Controls and surfaces nest by radius: the smaller the thing, the smaller the corner.

- `--anfra-radius-md` {rounded.md}: menu items, nav items, chips, tooltips.
- `--anfra-radius-lg` {rounded.lg}: buttons, fields, alerts.
- `--anfra-radius-xl` {rounded.xl}: cards, popovers, dialogs, the panel.
- `--anfra-radius-pill` {rounded.pill}: status pills and counters only, never buttons.

## Components

Cloud builds these with shadcn-vue. Core (appserve and the SDK's plugin UI) has no component library, so it uses the class layer, `components.css`: each class draws what the shadcn component of its name draws in Cloud, from the same Tier 2 tokens. Variants and sizes are data attributes, as in shadcn.

| Class | Cloud's component | Variants |
| --- | --- | --- |
| `.a-button` | Button | `data-variant` default, outline, secondary, ghost, destructive, link; `data-size` xs, sm, default, lg, icon, icon-xs, icon-sm, icon-lg |
| `.a-input` | Input | `aria-invalid="true"` for an error |
| `.a-badge` | Badge | `data-variant` default, secondary, outline, destructive, warning, success |
| `.a-pill` | StatusPill | `data-tone` neutral, highlight, success, warning, danger |
| `.a-tabs`, `.a-tab` | TabsList, TabsTrigger | `data-variant="line"` on the list; the open tab is `aria-selected="true"` |
| `.a-menu`, `.a-menu-item`, `.a-menu-label`, `.a-menu-separator`, `.a-menu-shortcut` | DropdownMenu | `data-variant="destructive"` on an item; `data-highlighted` for a keyboard highlight |
| `.a-tooltip` | TooltipContent | |
| `.a-kbd` | Kbd | |
| `.a-nav-label`, `.a-nav-item`, `.a-tree`, `.a-tree-row` | SidebarGroupLabel, SidebarMenuButton, SidebarMenuSub, SidebarMenuSubButton | `data-size="sm"` on a nav item; the current page is `aria-current="page"` |

A change to a Cloud component's look is a change to its class here, in the same release.

### Buttons

| Variant | Look | Use |
| --- | --- | --- |
| default | Violet fill {colors.primary}, white label | The one primary action of a view |
| outline | White, `border-strong`, ink label | Every other action that needs weight |
| ghost | No border, weak ink, `background-hover` on hover | Quiet actions: toolbar buttons, "Back", "Sign out" |
| link | Violet text, underline on hover | Task actions that do the job another way |
| destructive | Red fill {colors.error}, white label | A destructive action |

- Labels are the label role. Buttons are {spacing.control-height} high; a smaller button never carries an action in a form or a list row.
- A toggled toolbar button (a panel open, a mode on) shows `background-active` and `aria-pressed`.
- Every pressable control scales to 0.97 on press.

### Fields

- **Rest**: white, `border-field`, the body role, `text-placeholder` for the placeholder.
- **Focus**: no outline; the border turns violet (`border-highlight-strong`) and doubles to 2px.
- **Error**: the border is red (`border-danger-strong`) at rest and on focus, never violet. The message is `text-danger-default` under the field and clears as soon as the user types again.

### Surfaces and status

- **Current nav item**: on the paper, a white chip with a violet icon; in a nav inside the panel (tabs, settings), `background-active` and weight 500. Both carry `aria-current="page"` or `aria-selected`.
- **Alerts**: a tinted container with an icon and the body role.
- **Badges and status pills**: caption at 500, a label or an icon always beside the color.
- **Tooltips**: inverse ink (`background-inverted-default`, `text-inverted-default`), the caption role, `--anfra-radius-md`.

### Icons

Lucide, at 16px in controls and 14px in dense rows, `currentColor`, a 2px round stroke, never filled. Core draws the same icons from their path data, so it carries no icon library: `@holistics/anfra-design/icons` exports `ICONS`, each icon's path data on a 24px grid, by the name it has in anfra (`sidebar`, `lineage`, `type-date`), generated from lucide by `scripts/icons.mjs`. To add one, add its name and lucide's to that script and run `pnpm --filter @holistics/anfra-design icons`.

### Logo

The product shows the logomark only: violet on light, the inverse on dark (`--anfra-color-icon-logo`), at the top of the sidebar. Never set the mark beside typed "anfra".

## Do's and Don'ts

- **Do** use one filled violet button per view; every other action is outline, ghost or link.
- **Do** pick a type role for every text, and keep three sizes on a screen: 20, 14 and 12px.
- **Do** give every status a label or an icon; **don't** use red and green for good and bad in charts.
- **Do** keep every control {spacing.control-height} high.
- **Don't** lower opacity for disabled, or use a fill color as text.
- **Don't** add glows, gradients, glass, colored shadows or textures.
- **Don't** animate more than the properties that change, or for longer than 300ms on a control.
- **Don't** set the logomark beside typed "anfra".
- **Don't** add a color to a host: add a token here first.

## Motion

`--anfra-ease-out` for UI, under 300ms: 90ms press (scale to 0.97), 160ms hover, 220ms panels (`--anfra-t-press`, `--anfra-t-hover`, `--anfra-t-ui`). Menus and tooltips scale from 0.97, never slide. `prefers-reduced-motion` removes movement and keeps short fades.

## Content

- "anfra" is lowercase, even first in a sentence.
- Text of one sentence has no end period; two or more sentences keep all of theirs.
- Users read "organization", never "org", "tenant" or "workspace". Code and URLs keep `org`.

## Accessibility

- Contrast follows WCAG 2.2 AA: text 4.5:1; icons, field borders and the focus outline 3:1. A new token is measured in both themes before it ships.
- Focus is a 2px violet outline (`focus-ring-default`) offset 3px on everything focusable; a control keeps its own border. Text fields show focus on their border instead.
- Color never carries meaning alone: a status, an error or a met rule also has text or an icon.

## Implementation

- **Package**: `@holistics/anfra-design`, released with anfra at its version. `index.css` imports `fonts.css`, `tokens.css`, `type.css` and `components.css`; `icons` is a module.
- **anfra Cloud** imports it in `web/src/main.css` and maps Tier 2 onto Tailwind and shadcn's aliases there.
- **appserve** imports it in `web/appserve/src/style.css`; its stylesheet uses Tier 2 tokens only.
- **The SDK's plugin UI** reads the theme a host passes (`--anfra-color-bg` and the rest, `web/sdk/src/common/theme.ts`); a host takes those from Tier 2.
- **To change a rule or a token**: a PR here, with `tokens.css` and this file in step; then each host takes the release. A rule for one page goes in that page's spec, not here.
- **Checks**: `pnpm --filter @holistics/anfra-design test` (every reference resolves, dark redefines only Tier 2, only Tier 1 holds raw colors, the class layer reads only Tier 2, `icons.js` is what the script writes); `npx @google/design.md lint web/design/DESIGN.md`.
