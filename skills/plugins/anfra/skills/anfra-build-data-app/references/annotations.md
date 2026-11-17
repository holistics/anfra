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
on a change. The list is empty until a plugin sets some, and it empties when the result is replaced.

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
`setOption`. It expects a category axis that holds one entry per result row, in row order, and
series whose `id` is the measure's column name. It draws on the chart's own series and adds series
of its own whose ids start with `annotation:`, so a click handler skips those. It colours each
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
  const horizontal = option.yAxis?.[0]?.type === 'category';
  const categoryAxis = horizontal ? 'yAxis' : 'xAxis';
  const valueAxis = horizontal ? 'xAxis' : 'yAxis';
  const at = (index, value) => (horizontal ? [value, index] : [index, value]);
  const dimension = result.columns.find((column) => !column.isMeasure)?.name;
  const measures = new Set(result.columns.filter((column) => column.isMeasure).map((column) => column.name));
  const positionOf = (value) => result.rows.findIndex((row) => row[dimension] === value);

  // The chart's own series, with what the last draw added taken off.
  const own = option.series.filter((series) => !String(series.id).startsWith('annotation:')).map((series) => ({
    ...series, data: (series.data ?? []).map((item) => (item && typeof item === 'object' && 'anfraOwn' in item ? item.anfraOwn : item)),
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
  const styles = new Map(); // series -> index -> itemStyle
  const style = (series, index, itemStyle) => {
    if (!styles.has(series)) styles.set(series, new Map());
    styles.get(series).set(index, { ...styles.get(series).get(index), ...itemStyle });
  };
  const marks = { markLine: [], markArea: [], markPoint: [] };
  const extra = [];
  const tips = new Map(); // row index -> its annotations, for the tooltip
  const categories = [...(option[categoryAxis]?.[0]?.data ?? [])];
  const shown = categories.length;

  // Marks last, so a mark's point sits over a focus's on the same row.
  const ordered = [...query.annotations].sort((a, b) => (a.kind === 'mark') - (b.kind === 'mark'));
  for (const annotation of ordered) {
    const c = color(annotation.severity);
    const name = annotation.label ?? '';
    const { anchor } = annotation;
    if (annotation.kind === 'mark' || annotation.kind === 'focus') {
      anchor.index.forEach((index) => tips.set(index, [...(tips.get(index) ?? []), annotation]));
      for (const series of seriesFor(anchor.measure)) {
        if (annotation.kind === 'focus') {
          const focused = new Set(anchor.index);
          if (series.type === 'bar') series.data.forEach((_, index) => { if (!focused.has(index)) style(series, index, { opacity: 0.35 }); });
          else anchor.index.forEach((index) => marks.markPoint.push({ coord: at(index, valueAt(series, index)), symbol: 'circle', symbolSize: 7, itemStyle: { color: c } }));
        } else if (series.type === 'bar') {
          anchor.index.forEach((index) => style(series, index, { color: c }));
        } else {
          anchor.index.forEach((index) => marks.markPoint.push({
            coord: at(index, valueAt(series, index)), symbol: 'circle', symbolSize: 11, itemStyle: { color: c, borderColor: '#ffffff', borderWidth: 2 },
          }));
        }
      }
    } else if (annotation.kind === 'line') {
      const onDimension = anchor.column === dimension;
      const value = onDimension ? positionOf(anchor.value) : Number(anchor.value);
      if (onDimension && value < 0) continue;
      marks.markLine.push({
        [onDimension ? categoryAxis : valueAxis]: value, lineStyle: { color: c, type: 'dashed', width: 1.5 }, label: { formatter: name, color: c, position: 'insideEndTop' },
      });
    } else if (annotation.kind === 'band') {
      const onDimension = anchor.column === dimension;
      const [from, to] = onDimension ? [positionOf(anchor.from), positionOf(anchor.to)] : [Number(anchor.from), Number(anchor.to)];
      if (onDimension && (from < 0 || to < 0)) continue;
      const axis = onDimension ? categoryAxis : valueAxis;
      marks.markArea.push([
        { [axis]: from, name, itemStyle: { color: c, opacity: 0.12 }, label: { color: c, position: 'insideTop' } },
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
      ...own.map((series) => ({
        ...series,
        data: series.data.map((item, index) => {
          const itemStyle = styles.get(series)?.get(index);
          if (!itemStyle) return item;
          const base = item && typeof item === 'object' && !Array.isArray(item) ? item : { value: item };
          return { ...base, itemStyle: { ...base.itemStyle, ...itemStyle }, anfraOwn: item };
        }),
      })),
      {
        id: 'annotation:marks', type: 'line', data: [], silent: true,
        markLine: { symbol: 'none', data: marks.markLine },
        markArea: { data: marks.markArea },
        markPoint: { data: marks.markPoint },
      },
      ...extra,
    ],
  }, { replaceMerge: ['series'] });
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
- A right-click on a point should open the menu on its row (`query.menu`, see
  [api.md](api.md#the-menu-on-the-page)). The menu lists the row's annotations first, followed by
  the plugins' actions on them. Skip the helper's own series there: `if
  (String(p.seriesId).startsWith('annotation:')) return;`.

## Other chart libraries, SVG and tables

For a chart drawn by another library, write the same single helper for that library: one function,
called after each render, that draws the kinds above with that library's own marks (its mark lines,
areas and point styles). For hand-drawn SVG or a DOM table, restyle the elements at each `index`, as
you would for `selectedRows`. Then list in `data-anfra-annotations` exactly the kinds the helper
draws, and let the badge show the rest.
