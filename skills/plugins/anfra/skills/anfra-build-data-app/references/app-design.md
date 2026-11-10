# Designing a Data App

Read this at step 2 in the default anfra look. Creative and own-design builds skip it. They follow
the user's design or the direction in the plan, and the floors in SKILL.md.

It holds the calls the example can't make for you: the layout for the kind of page, the form each
block takes, which control fits, how numbers read. The look itself (tokens, type, controls, cards,
tooltips, states) is already built in [example.html](example.html), so keep its parts when you adapt
it instead of restyling them.

Everything below is a default with its reason. When the content needs something else, depart from it
and say why in a comment. Only the floors and the "In every look" rules in SKILL.md are not yours to
change.

The frame `anfra serve` gives the app is about 1166 × 834px on a 1440 laptop, and it scrolls. The
reader decides from that first view whether to read on. Left to its defaults, an agent lays a page
out by inventory, one equal card per measure, and the reader can't tell what matters.

## 1. Layout

Top to bottom: the header (the `h1`, one caption line with the scope and basis, page controls on
the right of the same row), the body, then the footer. Leave out a part the plan has nothing for.

**A KPI or monitoring page** leads with its numbers:

- **A KPI row**, the lead tile first. When one number decides and a query can say why, use a hero
  instead, which puts that figure beside its computed sentence. Four tiles is about what a reader
  compares at a glance, so past four the extra measures usually read better in a table.
- **A KPI tile** is a label, the figure in ink, then a delta pill (arrow and signed change, tinted
  green or red by whether that direction is good for this measure, gray when neither) and its
  comparison: "[↑ +6.2%] vs last year". Pills stay in KPI tiles. In tables and charts a change is
  signed text, because a pill on every number marks none of them.
- Then the chart that explains the lead, then detail.
- **A footer of definitions:** one entry per measure under its KPI's column (the term over one
  short clause from its AML description, which fits one line under a tile), then a line with the
  comparison, the date basis and the as-of date. A list rather than a paragraph, so the reader
  finds one measure without reading them all.

**Other kinds** (an explorer, a catalog, a lineage view, a detail lookup, an ops list) have no
lead, so open with the control or view the reader came for (the search, the filter, the list). Add
a definitions footer only when the page shows measures a reader could misread.

**Grid:** every row is a `.row` of spanned cards on one 12-column grid, as in the example, so edges
between rows line up or differ by a whole column, never by a few pixels. Spans follow the content.
As a start, a time series takes 6 columns or more and labeled bars 4 or more, so their labels fit,
and a table 6 or more. The chart that explains the lead takes the wider span (8 + 4), or 6 + 6
when its companion needs the room. Tiles in one row share one span, and cards in a row one height.

**Controls:** page-wide ones go in the header row. Past about three they wrap at laptop width, so
the rest go in a bare row under the header. A control that changes one chart goes in that chart's
card header: a segmented control, or a dropdown only with `overflow: visible` on that card, because
`.card` clips any dropdown inside it, header included (SKILL.md).

## 2. Charts

Walk the chooser top to bottom, and take the first branch that fits.

```
What does the block answer?
├── One number vs a comparison ──────────────────────▶ KPI tile, no chart
├── How it changed over time
│   ├── series more than 5× apart, or in different ──▶ small multiples: a panel per series,
│   │   units (one axis flattens the smaller one)        stacked, sharing one x-axis
│   ├── this period vs the same period last year ────▶ two lines, current in violet,
│   │                                                    last year in the context gray
│   ├── 1 to 3 series ───────────────────────────────▶ lines, labeled at their ends
│   └── 4 or more ───────────────────────────────────▶ highlight the one that matters, the
│                                                        rest in context gray
├── How categories compare
│   ├── ordered categories (months, buckets) ────────▶ columns in that order
│   ├── up to 12 ────────────────────────────────────▶ horizontal bars, sorted, value at the end
│   └── more than 12 ────────────────────────────────▶ top 10 + "Other", or a table
├── Two measures in different units, per category ───▶ two aligned bar charts, or a table
├── Category × time, or category × category ─────────▶ heatmap in one hue, values in the cells
├── Parts of one whole: 2 or 3 parts ────────────────▶ one 100% bar or a donut; more: sorted bars
└── Exact values to look up ─────────────────────────▶ table
```

- The card heading is the chart's title, so the canvas has none. One series needs no legend, and 2
  or 3 lines get labels at their ends instead of one.
- Every series is named without hover (an end label, a data label or a table column). Tooltips go
  through the example's `tooltipStyle()` and `tooltipHtml()`, so every chart's tooltip looks alike
  and follows dark mode.
- **Violet first.** A chart with one series draws it in violet (`chartTheme().cat[0]`), and several
  series take the palette in order from violet, because violet is what makes the page read as anfra.
  A main chart in another color reads as another product.
- A selection dims the other marks rather than recoloring them. Red and green never mean good and
  bad in a chart, and status colors never color a series.
- A period still filling is labeled "to date" (the example's `isPartial()`), or the trend ends at
  the last full period, because a partial month's lower total reads as a drop.
- A mark that filters the page on click looks clickable (pointer, hover emphasis, "Click to filter
  the page" in its tooltip), and a control does the same for keyboard readers.
- Design for the worst row: the longest name (truncated with an ellipsis), a null, a zero, a
  single category. Find each in the data and check the chart holds.

Tables: numbers right-aligned in `tabular-nums`, text left-aligned, one line per cell with the
full value in `title`, and `pageSize` once a table outgrows a screen (about 50 rows).

## 3. Controls

Pick each control by what it sets:

```
Views that swap which blocks show ─────────▶ Basecoat .tabs, above the blocks they swap
2 to 5 options, one pick (period, grain) ──▶ the example's segmented control
More than 5 options, one pick ─────────────▶ Basecoat div.select; a combobox once the list is
                                              too long to scan (about 15 values)
Several picks ─────────────────────────────▶ the example's multi-select: one 32px line,
                                              removable chips, then "+N"
A date range ──────────────────────────────▶ presets; two date inputs only if the user asks
```

For several picks from a long list (a country filter with 200 values), keep the one-line chip field
and add a search input at the top of its menu that hides the options that don't match, because
Basecoat's select has no search of its own and the combobox's chips wrap and grow the field. When
the list has a natural parent (country, then city), a parent filter can narrow it instead.

Never use a native `<select>`. The browser draws it, so it reads as a second library beside
Basecoat. Controls side by side share the example's 32px height and white surface. Basecoat's own
default is 36px, so restyling one kind alone leaves a row that looks off without anyone knowing why.
Every control has a visible label. "Clear selection" shows only while there is a selection.

## 4. Words and numbers

- **Title:** what the page is about, in a few of the reader's words, sentence case: "Subscription
  growth". Never "Dashboard", "Overview" or "Analytics". Keep `<title>`, `h1` and
  `createApp({ title })` the same.
- **Card heading:** what is plotted and against what: "New signups by week, vs the previous 13".
- **One name per concept** in tile, chart, table, tooltip and footer, taken from the field's label
  in sentence case. Never a field name, query name or alias.
- **A sentence on the page is computed** from a query result, and claims only what a query
  measured: no "strong", no "driven by". A finding typed as static text is wrong the day the data
  changes.
- Format with `Intl.NumberFormat` and `Intl.DateTimeFormat`, from the result's strings, one
  formatter per kind. The SDK doesn't carry a measure's `format`, so read currency and precision
  from its AML when you write the app, and keep one map from alias to formatter with a comment
  naming the AML. Never guess a currency from a field name.

| Kind | Format |
| --- | --- |
| Money | compact past 10,000 in figures and axes ($1.2M), full in tables and tooltips |
| Count | grouped (12,480) |
| Rate | percent, 1 decimal (4.7%), and its change in points (+1.2 pts) |
| Change | signed, with its comparison ("+2.3% vs last year"). Write minus as − (U+2212), never "−0.0" |
| Date | short month, never ISO (Oct 19, 2026; Oct 2026; Week of Oct 13) |
| `null` | "No data", or an en dash in a table, never 0, which means something else |

One precision per measure across the page. No "please", "Oops" or exclamation marks, because the
page is a tool.

## 5. States

Each block shows four states in its loaded size, so nothing jumps (the example's
`renderStatus()` and `renderKpis()`):

- **Loading:** a skeleton shaped like the content.
- **Updating:** the old result stays, dimmed. An old figure never shows undimmed under the new
  scope's label.
- **Empty:** the scope that is empty and how to widen it: "No signups in Brazil in this period.
  Choose a longer period."
- **Error:** by who must act, as in the example: "no access" or "sign in again" for the reader's
  rights, what failed and the message for a bad input, and "couldn't load" with a Retry when the
  server failed.

Nothing moves on load: no fade-in, count-up or chart entry. A page read every day wears out a
flourish by the second visit.

## 6. Unslop

Agents add by default, and each addition dilutes what the page is for. Before handing over, delete
every element whose absence breaks nothing, then remove these tells. Each is defensible alone, but
together they read as a generated page:

- gradients, glass, glows, colored shadows, a different tint per card
- uppercase eyebrows, Title Case headings, a fourth font size, weights above 600
- an emoji or icon before every label, a pill or badge on every number
- an "Insights" or "Key takeaways" card no query produced, "Welcome back", a caption that repeats
  its heading
- puffery (powerful, seamless, robust) and judgments no query measured
- cards that lift on hover but do nothing on click.

## 7. Self-check

At step 5, after the floors in SKILL.md, read the file against these and fix every "no". First
drafts miss the same few, and each one the user catches costs a round of feedback:

1. Does the first view hold the header and what the reader came for: the lead and the top of its
   chart on a KPI page, the control and the first results on other kinds?
2. On a KPI page, is there one lead, first, with its comparison, and do the defaults show the
   finding?
3. Does each span fit its content, on one 12-column grid, with no control in a card body?
4. Is each block the chooser's form, with no legend on one series and no unsorted bars?
5. Is each control the right kind for what it sets?
6. Are titles, headings, names and numbers written as section 4 says, and is every tell in
   section 6 gone?
7. Does every departure from a default in this file carry a comment with its reason?
