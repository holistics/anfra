# Annotations

Read this only when the page draws annotations: the agreed design asks for them, or the host
installs a plugin that sets them (an anomaly detector, a forecast, targets).

An **annotation** is something a plugin says about a query's rows, for the reader to see on the
chart that draws the query: a flagged month, a target line, a forecast. The plugin sets it. Your app
draws it with the chart's own marks, so it shows in pictures of the page too. The SDK draws nothing
on your chart. When an element doesn't draw an annotation's kind, the SDK shows a small badge in
the element's top-right corner instead, with a count. A click on the badge lists the annotations.

## What `query.annotations` holds

`query.annotations` lists every plugin's annotations on the query's current result. It changes
through `query.subscribe` like the result does, so a render function that subscribes already redraws
on a change. The list is empty until a plugin sets some. It empties when the result is replaced, and as soon as
your code calls `query.update`.

| Field | Means |
| --- | --- |
| `kind` | `mark`, `focus`, `line`, `band` or `series`. See the table below. |
| `anchor` | Where the annotation is, in the query's own column aliases. Its shape depends on the kind. |
| `label`, `detail` | Optional text: `label` is short, for the chart. `detail` is longer, for the tooltip. |
| `severity` | Optional: `info`, `warning` or `critical`. Take its colour from your theme. |
| `plugin`, `key` | The plugin that set it, and a key that stays the same across reads. |

| Kind | Anchor | Draw it as |
| --- | --- | --- |
| `mark` | `{ rows, measure?, index }` | The marked rows' points, in the severity's colour. |
| `focus` | `{ rows, measure?, index }` | The other rows dimmed. |
| `line` | `{ column, value }` | A line across the chart at `value` on `column` (a dimension or a measure). |
| `band` | `{ column, from, to }` | A shaded range on `column`. |
| `series` | `{ measure, points: [{ at, value?, lower?, upper? }] }` | An extra line through `value`, and a band from `lower` to `upper`. A point's `at` names dimension values, which can be missing from the result, such as a future month. |

- `index` lists the rows' positions in `query.result.rows`. Restyle by it, as you would for
  `query.selectedRows`. `measure` names the series the rows belong to, when the chart draws several.
- Values are strings, as in the rows. Convert them with `Number()` for a chart.

## Declare what the chart draws

Put `data-anfra-annotations` on the element marked `data-anfra-query`, listing the kinds the chart
draws:

```html
<div id="trend" data-anfra-query="trend" data-anfra-annotations="mark focus line band series"></div>
```

The badge counts every kind that the attribute leaves out. With no attribute, it counts them all.
`data-anfra-annotations="off"` removes the badge. Use `off` only for an element that shows
annotations some other way, such as a table that lists them. **List exactly the kinds your helper
draws.** If a kind is listed but not drawn, the reader never sees it.

## The ECharts helper

Write `drawAnnotations` once per app, and call it in each chart's render, right after that chart's
`setOption`. It expects one entry per result row, in row order (a category on the axis, or a pie's
slice), and series whose `id` is the measure's column name. It draws on the chart's own series and
adds series of its own whose ids start with `annotation:`, so a click handler skips those. What it
draws depends on the chart:

| Chart | Draws | Declare |
| --- | --- | --- |
| Line, or vertical bars (columns) | Every kind. A `series` is a dashed line and a band past the last row, over columns too. | `mark focus line band series` |
| Horizontal bars (`yAxis` is the category axis) | Every kind. A `line` or `band` on the measure runs across the value axis (x); on the dimension, across the category axis (y). | `mark focus line band series` |
| Pie | `mark` (the slice in the severity's colour, pulled out) and `focus` (the other slices dimmed). A pie has no axis for a line, a band or a series, so the badge shows those. | `mark focus` |

Declare a kind only when the chart's result can carry it: a `series` needs periods to extend, so a
chart by category leaves it out, and the badge counts one if a plugin ever sets it. It colours each
annotation from three CSS variables. Define them in the app's styles, in hex, for light and dark,
because ECharts can't parse `oklch`:

```css
:root { --annotation-info: #6d28d9; --annotation-warning: #b45309; --annotation-critical: #c2410c; }
.dark { --annotation-info: #a78bfa; --annotation-warning: #fbbf24; --annotation-critical: #fb923c; }
```

```js
function drawAnnotations (chart, query, { label = String } = {}) {
  const result = query.result;
  const option = chart.getOption();
  if (!result || !option?.series?.length) return;
  const css = getComputedStyle(document.documentElement);
  const color = (severity) => css.getPropertyValue(`--annotation-${severity ?? 'info'}`).trim();
  const esc = echarts.format.encodeHTML;
  const cartesian = Boolean(option.xAxis?.length); // a pie has no axes: only mark and focus
  const horizontal = option.yAxis?.[0]?.type === 'category';
  const categoryAxis = horizontal ? 'yAxis' : 'xAxis';
  const valueAxis = horizontal ? 'xAxis' : 'yAxis';
  const at = (index, value) => (horizontal ? [value, index] : [index, value]);
  const dimension = result.columns.find((column) => !column.isMeasure)?.name;
  const measures = new Set(result.columns.filter((column) => column.isMeasure).map((column) => column.name));
  const positionOf = (value) => result.rows.findIndex((row) => row[dimension] === value);

  // The chart's own series, with what the last draw added taken off.
  const own = option.series.filter((series) => !String(series.id).startsWith('annotation:')).map((series) => ({
    ...series, ...series.anfraOwnSeries, anfraOwnSeries: null,
    data: (series.data ?? []).map((item) => (item && typeof item === 'object' && 'anfraOwn' in item ? item.anfraOwn : item)),
  }));
  const seriesFor = (measure) => {
    const matched = own.filter((series) => (measure ? series.id === measure : measures.has(series.id)));
    return matched.length ? matched : own.slice(0, 1);
  };
  const valueAt = (series, index) => {
    const item = series.data[index];
    const value = Array.isArray(item) ? item[horizontal ? 0 : 1] : (item && typeof item === 'object' ? item.value : item);
    return Number(Array.isArray(value) ? value[horizontal ? 0 : 1] : value);
  };
  const items = new Map(); // series -> index -> what the annotations change on that item
  const changed = new Map(); // series -> what the annotations change on the series itself
  const item = (series, index, change) => {
    if (!items.has(series)) items.set(series, new Map());
    const was = items.get(series).get(index) ?? {};
    items.get(series).set(index, { ...was, ...change, itemStyle: { ...was.itemStyle, ...change.itemStyle } });
  };
  const change = (series, patch) => changed.set(series, { ...changed.get(series), ...patch });
  const marks = { markLine: [], markArea: [], markPoint: [] };
  const extra = [];
  const tips = new Map(); // row index -> its annotations, for the tooltip
  const categories = [...(option[categoryAxis]?.[0]?.data ?? [])];
  const shown = categories.length;

  for (const annotation of query.annotations) {
    const c = color(annotation.severity);
    const name = annotation.label ?? '';
    const { anchor } = annotation;
    if (annotation.kind === 'mark' || annotation.kind === 'focus') {
      anchor.index.forEach((index) => tips.set(index, [...(tips.get(index) ?? []), annotation]));
      for (const series of seriesFor(anchor.measure)) {
        if (annotation.kind === 'focus') {
          // Dim every other row: a bar, a slice, or a line's point (its line fades too).
          if (series.type === 'line') change(series, { showSymbol: true, lineStyle: { ...series.lineStyle, opacity: 0.4 } });
          series.data.forEach((_, index) => { if (!anchor.index.includes(index)) item(series, index, { itemStyle: { opacity: 0.25 } }); });
        } else if (series.type === 'pie') {
          // Colour the slice, and pull it out (below) unless the app selects slices itself.
          if (!series.selectedMode) change(series, { selectedMode: 'multiple' });
          anchor.index.forEach((index) => item(series, index, { itemStyle: { color: c }, anfraPulled: true }));
        } else if (series.type === 'bar') {
          anchor.index.forEach((index) => item(series, index, { itemStyle: { color: c } }));
        } else {
          anchor.index.forEach((index) => marks.markPoint.push({
            coord: at(index, valueAt(series, index)), symbol: 'circle', symbolSize: 11, itemStyle: { color: c, borderColor: '#ffffff', borderWidth: 2 },
          }));
        }
      }
    } else if (!cartesian) {
      continue;
    } else if (annotation.kind === 'line') {
      const onDimension = anchor.column === dimension;
      const value = onDimension ? positionOf(anchor.value) : Number(anchor.value);
      if (onDimension && value < 0) continue;
      marks.markLine.push({
        [onDimension ? categoryAxis : valueAxis]: value, lineStyle: { color: c, type: 'dashed', width: 1.5 },
        label: { formatter: name, color: c, position: 'insideEndTop' }, // inside the plot, so it never overflows
      });
    } else if (annotation.kind === 'band') {
      const onDimension = anchor.column === dimension;
      const [from, to] = onDimension ? [positionOf(anchor.from), positionOf(anchor.to)] : [Number(anchor.from), Number(anchor.to)];
      if (onDimension && (from < 0 || to < 0)) continue;
      const axis = onDimension ? categoryAxis : valueAxis;
      // A band standing upright is labelled just above the plot, where no bar hides it.
      const upright = onDimension !== horizontal;
      marks.markArea.push([
        { [axis]: from, name, itemStyle: { color: c, opacity: 0.12 }, label: { color: c, position: upright ? 'top' : 'insideTop' } },
        { [axis]: to },
      ]);
    } else if (annotation.kind === 'series') {
      const [series] = seriesFor(anchor.measure);
      const last = result.rows.length - 1;
      const points = anchor.points.map((point) => {
        let index = positionOf(point.at[dimension]);
        if (index < 0) {
          categories.push(label(point.at[dimension]));
          index = categories.length - 1;
        }
        return { index, ...point };
      });
      // From the last row, so the new line joins the chart's own.
      const start = last >= 0 && series ? [{ index: last, value: valueAt(series, last), lower: valueAt(series, last), upper: valueAt(series, last) }] : [];
      const line = (pick) => [...start, ...points].filter((point) => pick(point) !== undefined).map((point) => at(point.index, Number(pick(point))));
      const common = { type: 'line', showSymbol: false, silent: true, tooltip: { show: false } };
      if (points.some((point) => point.value !== undefined)) {
        extra.push({ ...common, id: `annotation:${annotation.key}`, name, data: line((point) => point.value), lineStyle: { color: c, type: 'dashed', width: 2 }, itemStyle: { color: c } });
      }
      if (points.some((point) => point.lower !== undefined)) {
        const stack = `annotation:${annotation.key}:band`;
        extra.push({ ...common, id: `${stack}:lower`, stack, data: line((point) => point.lower), lineStyle: { opacity: 0 } });
        extra.push({
          ...common, id: `${stack}:range`, stack, lineStyle: { opacity: 0 }, areaStyle: { color: c, opacity: 0.15 },
          data: [...start, ...points].filter((point) => point.lower !== undefined).map((point) => at(point.index, Number(point.upper) - Number(point.lower))),
        });
      }
    }
  }

  // The tooltip lists a row's annotations under the chart's own lines.
  const update = {};
  const tooltip = option.tooltip?.[0];
  if (tooltip && typeof tooltip.formatter === 'function') {
    const base = tooltip.formatter.anfraOwn ?? tooltip.formatter;
    const formatter = (params, ...rest) => {
      const first = Array.isArray(params) ? params[0] : params;
      const notes = tips.get(first?.dataIndex) ?? [];
      return base(params, ...rest) + notes.map((note) => `<div class="tt-note" style="color:${color(note.severity)}">`
        + `${esc(note.label ?? note.kind)}${note.detail ? `: ${esc(note.detail)}` : ''}</div>`).join('');
    };
    formatter.anfraOwn = base;
    update.tooltip = { formatter };
  }
  if (categories.length !== shown) update[categoryAxis] = [{ data: categories }];
  chart.setOption({
    ...update,
    series: [
      ...own.map((series) => {
        const patch = changed.get(series);
        const kept = { showSymbol: series.showSymbol, lineStyle: { ...series.lineStyle, opacity: series.lineStyle?.opacity ?? 1 }, selectedMode: series.selectedMode ?? false };
        return {
          ...series,
          ...(patch ? { ...patch, anfraOwnSeries: Object.fromEntries(Object.keys(patch).map((key) => [key, kept[key]])) } : {}),
          data: series.data.map((value, index) => {
            const patched = items.get(series)?.get(index);
            if (!patched) return value;
            const base = value && typeof value === 'object' && !Array.isArray(value) ? value : { value };
            return { ...base, ...patched, itemStyle: { ...base.itemStyle, ...patched.itemStyle }, anfraOwn: value };
          }),
        };
      }),
      ...(cartesian ? [{
        id: 'annotation:marks', type: 'line', data: [], silent: true,
        markLine: { symbol: 'none', data: marks.markLine },
        markArea: { data: marks.markArea },
        markPoint: { data: marks.markPoint },
      }] : []),
      ...extra,
    ],
  }, { replaceMerge: ['series'] });
  // Pull a pie's marked slices out by selecting them, unless the app selects slices itself.
  for (const series of own.filter((series) => series.type === 'pie' && !series.selectedMode)) {
    const pulled = [...(items.get(series) ?? [])].filter(([, patch]) => patch.anfraPulled).map(([index]) => index);
    chart.dispatchAction({ type: 'unselect', seriesId: series.id, dataIndex: series.data.map((_, index) => index) });
    if (pulled.length) chart.dispatchAction({ type: 'select', seriesId: series.id, dataIndex: pulled });
  }
}
```

Call it after the chart's own options. Pass `label` when the axis shows formatted labels, so a
forecast's new months are formatted the same way:

```js
trendChart.setOption({ /* the chart's own options */ }, { replaceMerge: ['series'] });
drawAnnotations(trendChart, trend, { label: (period) => periodLabel(period, grain) });
```

- It adds a row's annotations to the chart's tooltip when the tooltip has a `formatter` function.
  Give the tooltip one, as the example's charts have.
- It keeps a line's label inside the plot, so it never runs past the chart's edge. An upright band
  (over a few months, or a value range on horizontal bars) is labelled just above the plot, where
  no bar hides it: leave the grid 24px of `top` for it. A band is narrow, so plugins keep labels
  short.
- On a pie, a mark pulls its slice out by selecting it. When the app sets the pie's `selectedMode`
  itself, the helper only colours the slice.
- A right-click on a point should open the menu on its row (`query.menu`, see
  [api.md](api.md#the-menu-on-the-page)). The menu lists the row's annotations first, followed by
  the plugins' actions on them. Skip the helper's own series there: `if
  (String(p.seriesId).startsWith('annotation:')) return;`.

## Other chart libraries, SVG and tables

For a chart drawn by another library, write the same single helper for that library: one function,
called after each render, that draws the kinds above with that library's own marks (its mark lines,
areas and point styles). For hand-drawn SVG, restyle the elements at each `index`, as you would for
`selectedRows`. Then list in `data-anfra-annotations` exactly the kinds the helper draws, and let the
badge show the rest.

A table draws `mark` and `focus` by restyling its rows, as it does for `selectedRows`, and gives a
marked row its label as a `title`. It has no axis for the other kinds, so it declares only those two,
and the badge shows the rest. A right-click on a row opens the menu on it, as on a chart's point:

```js
const marked = new Map(); // row index -> its mark
const focused = new Set();
for (const annotation of byProduct.annotations) {
  if (annotation.kind === 'mark') annotation.anchor.index.forEach((index) => marked.set(index, annotation));
  if (annotation.kind === 'focus') annotation.anchor.index.forEach((index) => focused.add(index));
}
tbody.querySelectorAll('tr').forEach((tr, index) => {
  const mark = marked.get(index);
  tr.dataset.mark = mark ? mark.severity ?? 'info' : ''; // styled by tr[data-mark="warning"] etc.
  tr.title = mark?.label ?? '';
  tr.classList.toggle('dimmed', focused.size > 0 && !focused.has(index));
  tr.oncontextmenu = (event) => byProduct.menu(event, byProduct.result.rows[index]);
});
```

```html
<table data-anfra-query="byProduct" data-anfra-annotations="mark focus">…</table>
```
