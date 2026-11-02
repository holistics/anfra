---
version: alpha
name: anfra app
description: The anfra web app (anfra-bi). Paper and ink with one violet accent, dense and quiet, on the anfra brand.
colors:
  primary: "oklch(0.565 0.194 300.3)"
  primary-hover: "oklch(0.517 0.192 299.1)"
  on-primary: "oklch(1 0 0)"
  surface: "oklch(1 0 0)"
  surface-chrome: "oklch(0.955 0.007 88.6)"
  surface-sunken: "oklch(0.973 0.005 95.1)"
  on-surface: "oklch(0.201 0.022 301.1)"
  on-surface-weak: "oklch(0.392 0.021 304.3)"
  placeholder: "oklch(0.485 0.022 306.3)"
  disabled-surface: "oklch(0.201 0.022 301.1 / 0.04)"
  disabled-text: "oklch(0.723 0.017 306.3)"
  link: "oklch(0.484 0.181 298.1)"
  selected: "oklch(0.946 0.027 304.8)"
  inverse-surface: "oklch(0.201 0.022 301.1)"
  inverse-on-surface: "oklch(0.961 0.007 304.2)"
  error: "oklch(0.549 0.199 27.1)"
  error-text: "oklch(0.501 0.17 27.2)"
  error-container: "oklch(0.971 0.014 22.8)"
  warning-text: "oklch(0.501 0.1 70.3)"
  warning-container: "oklch(0.973 0.028 84.6)"
  success-text: "oklch(0.479 0.1 151.8)"
  success-container: "oklch(0.972 0.021 152.2)"
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
    fontFeature: "\"tnum\""
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
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.label}"
    rounded: "{rounded.lg}"
    height: "{spacing.control-height}"
    padding: 10px
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
    textColor: "{colors.on-primary}"
  button-outline:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    typography: "{typography.label}"
    rounded: "{rounded.lg}"
    height: "{spacing.control-height}"
  button-ghost:
    textColor: "{colors.on-surface-weak}"
    typography: "{typography.label}"
    rounded: "{rounded.lg}"
    height: "{spacing.control-height}"
  button-link:
    textColor: "{colors.link}"
    typography: "{typography.label}"
    height: "{spacing.control-height}"
  button-disabled:
    backgroundColor: "{colors.disabled-surface}"
    textColor: "{colors.disabled-text}"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    typography: "{typography.body}"
    rounded: "{rounded.lg}"
    height: "{spacing.control-height}"
    padding: 10px
  input-placeholder:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.placeholder}"
  input-code:
    typography: "{typography.code}"
    height: "{spacing.control-height}"
  panel:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.xl}"
  chrome:
    backgroundColor: "{colors.surface-chrome}"
    textColor: "{colors.on-surface}"
  well:
    backgroundColor: "{colors.surface-sunken}"
    textColor: "{colors.on-surface-weak}"
  sheet:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.xl}"
    padding: "{spacing.sheet-padding}"
    width: "{spacing.sheet-width}"
  row-selected:
    backgroundColor: "{colors.selected}"
    textColor: "{colors.on-surface}"
  tooltip:
    backgroundColor: "{colors.inverse-surface}"
    textColor: "{colors.inverse-on-surface}"
    typography: "{typography.caption}"
    rounded: "{rounded.md}"
  alert-danger:
    backgroundColor: "{colors.error-container}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.xl}"
  field-error:
    textColor: "{colors.error-text}"
    typography: "{typography.body}"
  button-destructive:
    backgroundColor: "{colors.error}"
    textColor: "{colors.on-primary}"
    typography: "{typography.label}"
    rounded: "{rounded.lg}"
  status-warning:
    backgroundColor: "{colors.warning-container}"
    textColor: "{colors.warning-text}"
    typography: "{typography.caption}"
    rounded: "{rounded.pill}"
  status-success:
    backgroundColor: "{colors.success-container}"
    textColor: "{colors.success-text}"
    typography: "{typography.caption}"
    rounded: "{rounded.pill}"
  mono-label:
    textColor: "{colors.placeholder}"
    typography: "{typography.mono-label}"
---

> Synced copy of `holistics/anfra-cloud` `web/DESIGN.md` at `bc5b72e` (my/req-29-signin-ui). Do not edit: change it in anfra-cloud. It refreshes by itself; by hand: `pnpm sync:app-design`.

# anfra app design

## Overview

The anfra app is a working notebook on warm paper: ink for everything, one violet pen for the one thing the reader follows. It is a tool for analysts who read models, metrics and query results all day, so it is dense, calm and exact. Nothing glows, nothing floats without a reason, and no screen tries to impress.

- **Paper and ink first.** Paper chrome frames a white work panel. Text, rules and icons are ink.
- **One violet.** Violet marks the one primary action of a view, links, focus and selection. It is never decoration.
- **Drawn, not decorated.** Hairlines carry structure. Shadows only lift what floats. Graph paper is the one texture, and only behind the signed-out pages.
- **One density.** Every screen uses the same control height, type scale and spacing.

The brand's principles, palette, logo system and motion live in the anfra brand file (`holistics/anfra-landing`, `DESIGN.md` on `main`). The anfra-landing `prototype` branch keeps a read-only copy of this file to test app screens; change the rules here. This file holds the rules for every app screen: which brand rules the app takes, where it departs from them, and which token to use for what. Rules for one page or one flow live in its feature spec and its code, not here. The front matter holds the light theme's values; `src/main.css` is the implementation and holds both themes. A change goes into both.

## Colors

Color tokens are in `src/main.css`, in three tiers, after the Holistics Design System:

| Tier | Names | Used by |
| --- | --- | --- |
| 1 Primitives | `--anfra-{ink,paper,violet,red,amber,green}-{step}`, OKLCH | Tier 2 only |
| 2 Semantic | `--anfra-color-{background,text,icon,border,focus-ring}-{semantic}-{state}` | Components, through Tailwind |
| 3 shadcn aliases | `--background`, `--primary`, `--sidebar-*`, ... | shadcn-vue components, unchanged |

Tailwind exposes Tier 2 per property: the same word is a different color for each property (`text-weak`, `bg-hover`, `border-strong`, `icon-weak`).

- **Paper** {colors.surface-chrome} is the chrome: the sidebar and the frame around the panel. **White** {colors.surface} is the work panel, cards, dialogs and the signed-out sheet. **Sunken paper** {colors.surface-sunken} is a well inside the panel: code, read-only fields, card footers. The app's raised surfaces are white, not the brand's sheet tone.
- **Ink** {colors.on-surface} carries titles, body text and values. **Weak ink** {colors.on-surface-weak} is secondary text and quiet controls. **Muted ink** {colors.placeholder} (`text-placeholder`) is the one muted color: descriptions, captions, muted labels and input placeholders. There is no second muted color.
- **Violet** {colors.primary} is the one accent. A filled violet button (`variant="default"`) appears once per view. Violet text is `text-highlight-default` or `text-link-default` {colors.link}, never `text-primary`: that is a fill color and fails as text on paper. The violet wash {colors.selected} (`bg-interactive-highlight-default`) marks a selected row or option, always with a checked control and `aria-selected`.
- **Status** is restrained red, amber and green; info is neutral ink, never blue or violet. A status always has a label or an icon, and never fills a large surface.
- **Disabled** is `disabled:bg-disabled disabled:text-disabled`, plus `disabled:border-disabled` on a control that has a border. A disabled `ghost` or `link` button has no border and no fill.
- **Neutral states** are `hover:bg-hover` and `active:bg-active`; they work on paper and on white. `bg-muted` is the sunken paper tone and does not show on the chrome.
- **Errors**: `text-danger-default` {colors.error-text} for error text; `text-destructive` is the solid fill {colors.error}, not text.
- **Dark theme** redefines Tier 2 only. System is the default theme (it follows the OS); light and dark are choices, stored per device.

| Need | Class |
| --- | --- |
| Text: body, secondary, muted | `text-default`, `text-weak`, `text-placeholder` |
| Violet text, links | `text-highlight-default`, `text-link-default hover:text-link-hover` |
| Field border, hover, invalid | `border-input`, `hover:border-hover`, `aria-invalid:border-danger-strong`, and `in-data-invalid:` for a field inside `<Field data-invalid>` |
| Floating surface | `bg-popover ring-1 ring-strong shadow-e2 rounded-xl` |
| Icons | `icon-default`, `icon-weak`, `icon-highlight-default`, `icon-{danger,warning,success,info}-default` |
| Status | `bg-{danger,warning,success,info}-secondary-default`, `text-{s}-default`, `border-{s}-default`, or `<StatusPill tone>` |

### Charts

`--chart-cat-1..8` is the categorical palette, in a fixed order, violet first. Light fills are lighter than 3:1 on white, so each slot has `--chart-cat-N-strong` for lines, points and outlines, and every filled mark shows its value as a label, a tooltip or in a table. Labels on fills are ink, never white. `--chart-seq-1..9` is the violet ramp for magnitude; `--chart-div-neg-4..pos-4` runs orange, gray, violet for change around a baseline. When one series carries the message, use `--chart-highlight` and `--chart-context`. Never use red and green for good and bad.

## Typography

Inter for all text; Geist Mono for code, revisions and mono labels. Both are bundled (`@fontsource-variable/*`), never loaded from a CDN: an on-prem install may be offline.

Sizes are role tokens (`--text-{role}` in `src/main.css`). Each class sets size, line height and the role's weight. Tailwind's own sizes (`text-xs` to `text-9xl`) are off: they draw nothing, and `pnpm lint` refuses them. A screen uses three sizes, 20, 14 and 12px: to add emphasis, change weight or color, not size.

| Class | Token | Size / leading | Weight | Use |
| --- | --- | --- | --- | --- |
| `text-title` | {typography.title} | 20 / 28px | 600 | The one heading of a page, a step or a dialog |
| `text-heading` | {typography.heading} | 14 / 20px | 600 | Section headings, card titles |
| `text-body` | {typography.body} | 14 / 20px | inherited: 400, 450 in dark | Descriptions, field text, alerts, list rows, menu items |
| `text-label` | {typography.label} | 14 / 20px | 500 | Buttons, links, field labels, the current page, names |
| `text-caption` | {typography.caption} | 12 / 16px | inherited | Dividers ("OR"), notes under a button, roles, timestamps, tooltips, shortcuts |
| `text-code` | {typography.code} | 20 / 28px | inherited | The 6-digit code field only |

- **No weight class beside a role**, except: `text-caption font-medium` for badges, status pills, group labels and initial tiles; `font-medium` on a name inside a sentence; `data-active:font-medium` on the current sidebar item; `dark:font-[550]` on uppercase mono labels {typography.mono-label}, which read too thin on dark at 400.
- **A new role gets a new name**, never a size suffix (`text-heading`, not `text-body-lg`).
- **Fields are 14px at every width.** shadcn sets 16px on phones to stop iOS Safari from zooming into a field; the app accepts the zoom.
- **A name in a sentence** (an organization slug, an address) is Inter 500, never mono.
- `tabular-nums` wherever numbers change or line up. Headings wrap with `text-balance`, paragraphs and alerts with `text-pretty`; never balance an alert, it breaks a short error in odd places.

## Layout

One density for every screen, on a 4px unit {spacing.unit}.

- **Work screens**: the paper chrome holds the sidebar; the white panel is inset {spacing.panel-inset} on the paper, with a 14px radius, a 1px `border-default` and no shadow. The header bar is navigation (a breadcrumb); the page's subject is its own `h1`.
- **Signed-out pages**: the logomark at {spacing.logomark-sheet} above one sheet, {spacing.sheet-width} wide with {spacing.sheet-padding} padding, centered on graph paper.
- **One control height**: {spacing.control-height} for every field and button, the code field included. Never `h-9` or `h-10` on one control.
- **Forms**: {spacing.field-gap} between a label, its field and its message; {spacing.form-gap} between groups. The main action is full width on the sheet; quieter actions sit centered under it.

## Elevation & Depth

Depth comes from tone and hairlines: paper behind, white in front, 1px rules between. Shadows are neutral ink, never colored, and only two exist: `shadow-e1` for a resting card, `shadow-e2` for what floats (menus, popovers, dialogs, the signed-out sheet). On dark, both become a border. Graph paper (`bg-graph-paper`) is the one texture, and only behind the signed-out sheet; work screens never carry a texture.

## Shapes

Soft, not round. Controls and surfaces nest by radius: the smaller the thing, the smaller the corner.

- `rounded-md` {rounded.md}: menu items, nav items, chips, tooltips.
- `rounded-lg` {rounded.lg}: buttons, fields, alerts.
- `rounded-xl` {rounded.xl}: cards, popovers, dialogs, the panel, the sheet.
- `rounded-pill` {rounded.pill}: status pills and counters only, never buttons.

## Components

shadcn-vue components live in `src/components/ui/` and are ours: edit them, never wrap them to fight them.

### Buttons

| Variant | Look | Use |
| --- | --- | --- |
| `default` | Violet fill {colors.primary}, white label | The one primary action of a view |
| `outline` | White, `border-strong`, ink label | Every other action that needs weight |
| `ghost` | No border, weak ink, `bg-hover` on hover | Leave actions: "Back", "Sign out" |
| `link` | Violet text, underline on hover | Task actions that do the job another way: "Resend code", "Sign in with a password", "Forgot your password, or new here?", "Not you? Switch account", "Sign in with another account" |
| `destructive` | Red fill {colors.error}, white label | A destructive action |

- Labels are `text-label`. Buttons are {spacing.control-height} high; `size="xs"` and `"sm"` never carry an action in a form or a list row.
- A link-style action sits centered under the main button.
- Every pressable control scales to 0.97 on press.

### Fields

- **Rest**: white, `border-input`, `text-body`, `text-placeholder` for the placeholder.
- **Focus**: no outline; the border turns violet (`border-highlight-strong`) and doubles to 2px.
- **Error**: the border is red (`border-danger-strong`) at rest and on focus, never violet. The message is `text-danger-default` under the field and clears as soon as the user types again.
- **Code errors** use the same words on every code step, from one place (`src/lib/signInErrors.ts`): a wrong guess under the field, an expired code in an alert.
- **Input group** (`ui/input-group`): a fixed prefix inside the frame, such as the organization address, in `text-placeholder` and not editable. The frame carries hover, focus and error; the input inside has no border.
- **Code field** (`<CodeInput>`): centered, `text-code`, digits only, no placeholder. The set-password step keeps a labeled, left-aligned code field, because it shares the step with the new password.
- **Password rule** (`<PasswordRule>`): one row per rule under every new-password field, always shown. Gray with a circle while unmet, green with a check once met, red with a cross once the user stops typing for 1 second with the rule unmet or after a refused submit; the field turns red with it. An empty field stays gray. A refused submit for this rule adds no second message; other password errors show under the row.

### Signed-out sheet

- **Step header** (`<StepHeader>`): a centered title, an optional description, and an optional icon in a 40px tile above the title.
- **Back** (`<BackLink>`): a ghost button fixed in the page's top-left corner, outside the sheet, so it does not compete with the centered logomark and header.
- **Resend code** (`<ResendCode>`): a link under the main button that counts down the server's wait ("Resend code in 42s"). Once the code has expired, it becomes the step's main button and the step's own button hides. A code step that needs a bot check renders its own Turnstile widget.

### Surfaces and status

- **Current nav item**: a white chip on the paper, violet icon, `aria-current="page"`.
- **Alerts**: a tinted container with an icon and `text-body`; `<FormAlert>` for a form's refusal that is not about one field.
- **Badges and status pills**: `text-caption font-medium`, a label or an icon always beside the color.
- **Tooltips**: inverse ink, `text-caption`, `rounded-md`.

### Logo

The app shows the logomark only, through `<Logo>`: violet on light, the inverse on dark, at the top of the sidebar and above the signed-out sheet. Never set the mark beside typed "anfra": where the name must sit next to the mark, use the brand's official `logo` file (mark and wordmark). A page that needs the name in text puts it in an ordinary heading.

## Do's and Don'ts

- **Do** use one filled violet button per view; every other action is outline, ghost or link.
- **Do** pick a type role for every text; **don't** use Tailwind sizes or pair a role with a weight class outside the listed exceptions.
- **Do** keep three sizes on a screen: 20, 14 and 12px.
- **Do** give every status a label or an icon; **don't** use red and green for good and bad in charts.
- **Do** keep every control {spacing.control-height} high.
- **Don't** use `opacity-50` for disabled, `text-primary` for violet text, or `text-destructive` for error text.
- **Don't** put a focus ring in a component; focus is set once in `src/main.css`.
- **Don't** add glows, gradients, glass, colored shadows or textures; graph paper belongs to the signed-out pages only.
- **Don't** use `transition-all`, or motion longer than 300ms on a control.
- **Don't** set the logomark beside typed "anfra".

## Motion

`ease-anfra-out` for UI, under 300ms: 90ms press (scale to 0.97), 160ms hover, 220ms panels. Menus and tooltips scale from 0.97, never slide. `prefers-reduced-motion` removes movement and keeps short fades. Hover styles exist only where hovering exists; Tailwind v4's `hover:` already does this.

## Content

- "anfra" is lowercase, even first in a sentence.
- Text of one sentence has no end period; two or more sentences keep all of theirs.
- Users read "organization", never "org", "tenant" or "workspace". Code and URLs keep `org` (`/o/<slug>`).
- A form that ends the account's other sessions says so in a `text-caption text-placeholder` note under its button, or in the dialog's description.

## Accessibility

- Contrast follows WCAG 2.2 AA: text 4.5:1; icons, field borders and the focus outline 3:1. A new token is measured in both themes before it ships.
- Focus is a 2px violet outline offset 3px on everything focusable; a control keeps its own border. Text fields show focus on their border instead (see Fields).
- Every page starts with a "Skip to content" link to `<main id="main">`.
- Color never carries meaning alone: a status, an error or a met rule also has text or an icon.

## Implementation

- **Stack**: shadcn-vue (style `reka-nova`), reka-ui, lucide icons, Tailwind v4. (Use basecoatui if this is a pure html app)
- **New shadcn component**: `pnpm dlx shadcn-vue@<version> add <name>`, then strip its `focus-visible:ring-*` and `focus-visible:border-*` classes and swap its colors and sizes to the tokens above.
- **New type role**: add it to `src/main.css`, to `TYPE_ROLES` in `src/lib/utils.ts` (without it, `cn()` drops the role beside a text color), and to Typography here.
- **Theme**: `index.html` sets `.dark` on `<html>` before first paint; `useTheme()` owns it after, stored in `localStorage` (`anfra.theme`). The class stays on `<html>`: the shadcn aliases resolve on `:root`.
- **Checks**: `pnpm lint` refuses Tailwind sizes; `npx @google/design.md lint web/DESIGN.md` checks this file's tokens and contrast.
