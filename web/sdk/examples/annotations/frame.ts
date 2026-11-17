// The annotations example's frame half: it sets one annotation of each kind on every query's result,
// from trivial rules, so a definition's drawing of them can be tried. A real plugin (an anomaly
// detector, a forecast) computes them the same way and annotates each new result. It imports only
// types: everything it reaches, it reaches through the kernel.
import type {
  Annotation, AnnotationInput, FrameKernel, MenuTarget, Query, QueryResult, Row,
} from '@holistics/anfra-sdk/app';

/** A number as rows carry it: a string with no float noise (`55`, not `55.00000000000001`). */
const text = (value: number): string => String(Number(value.toPrecision(12)));

/** A date, or a timestamp at its day's start (`2026-03-01T00:00:00Z`), as a date column's rows carry it. */
const DAY = /^(\d{4})-(\d{2})-(\d{2})(T00:00:00(?:\.0+)?Z?)?$/;

/**
 * The next `count` periods after the last two, when they are dates, written as the last one is: by
 * whole months when both are on the 1st, else by the days between them. None when they aren't dates.
 */
function nextPeriods (before: string, last: string, count: number): string[] {
  const a = DAY.exec(before);
  const b = DAY.exec(last);
  if (!a || !b) return [];
  const pad = (value: number) => String(value).padStart(2, '0');
  const format = (date: Date) => `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())}${b[4] ?? ''}`;
  const end = Date.UTC(+b[1], +b[2] - 1, +b[3]);
  if (a[3] === '01' && b[3] === '01') {
    const months = (+b[1] * 12 + +b[2]) - (+a[1] * 12 + +a[2]);
    if (months <= 0) return [];
    return Array.from({ length: count }, (_, at) => format(new Date(Date.UTC(+b[1], +b[2] - 1 + months * (at + 1), 1))));
  }
  const step = end - Date.UTC(+a[1], +a[2] - 1, +a[3]);
  if (step <= 0) return [];
  return Array.from({ length: count }, (_, at) => format(new Date(end + step * (at + 1))));
}

/**
 * One annotation of each kind, on the result's first dimension and first measure. When the
 * dimension holds dates, the band covers the middle periods and the series forecasts three more;
 * else the band is a target zone on the measure, and there is no series.
 */
export function annotationsFor (result: QueryResult): AnnotationInput[] {
  const period = result.columns.find((column) => !column.isMeasure)?.name;
  const measure = result.columns.find((column) => column.isMeasure)?.name;
  const rows = result.rows.filter((row) => Number.isFinite(Number(row[measure ?? ''])));
  if (!period || !measure || !rows.length) return [];
  const value = (row: Row) => Number(row[measure]);
  const name = (row: Row): Row => ({ [period]: row[period] });
  const ranked = [...rows].sort((a, b) => value(b) - value(a));
  const mean = rows.reduce((sum, row) => sum + value(row), 0) / rows.length;
  const annotations: AnnotationInput[] = [
    {
      id: 'highest', kind: 'mark', anchor: { rows: [name(ranked[0])], measure }, label: 'Highest', detail: `${measure} peaks at ${String(ranked[0][measure])}.`, severity: 'warning',
    },
    {
      id: 'top-3', kind: 'focus', anchor: { rows: ranked.slice(0, 3).map(name), measure }, label: 'Top 3',
    },
    {
      id: 'mean', kind: 'line', anchor: { column: measure, value: text(mean) }, label: 'Mean', detail: `The mean of ${measure}.`,
    },
  ];
  if (rows.every((row) => DAY.test(String(row[period])))) {
    // Periods: a made-up campaign over the middle two or three, and a forecast after the last.
    const middle = Math.floor(rows.length / 2);
    const [from, to] = [rows[Math.max(0, middle - 1)], rows[Math.min(rows.length - 1, middle + 1)]];
    annotations.push({
      id: 'campaign', kind: 'band', anchor: { column: period, from: String(from[period]), to: String(to[period]) }, label: 'Campaign', detail: 'A made-up campaign over the middle periods.', severity: 'info',
    });
    if (rows.length >= 2) {
      const [before, last] = rows.slice(-2);
      const flat = value(last);
      const points = nextPeriods(String(before[period]), String(last[period]), 3).map((at) => ({
        at: { [period]: at }, value: text(flat), lower: text(flat * 0.9), upper: text(flat * 1.1),
      }));
      if (points.length) {
        annotations.push({
          id: 'flat', kind: 'series', anchor: { measure, points }, label: 'Flat forecast', detail: 'The last value, held for three periods, ±10%.', severity: 'critical',
        });
      }
    }
  } else {
    // Categories have no range to shade or periods to forecast: a target zone on the measure instead.
    annotations.push({
      id: 'target', kind: 'band', anchor: { column: measure, from: text(mean * 0.9), to: text(mean * 1.1) }, label: 'Target', detail: `Within 10% of the mean of ${measure}.`, severity: 'info',
    });
  }
  return annotations;
}

export default function setup (kernel: FrameKernel): void {
  // The result each query was last annotated for: annotating notifies, and must not loop.
  const annotated = new WeakMap<Query, QueryResult>();

  kernel.onApp((app) => {
    const annotateAll = () => {
      for (const query of Object.values(app.queries)) {
        const { result } = query;
        if (!result || annotated.get(query) === result) continue;
        annotated.set(query, result);
        kernel.annotate(query, annotationsFor(result), { result });
      }
    };
    app.subscribe(annotateAll);
    annotateAll();
  });

  // An action on its annotations, offered on their rows: in the rows menu and the badge's popover.
  const ours = (target: MenuTarget) => (target.kind === 'rows' ? target.annotations ?? [] : []).filter((annotation) => annotation.plugin === kernel.plugin);
  kernel.menuItem({
    id: 'why',
    label: 'Why is this marked?',
    target: 'rows',
    when: (target) => ours(target).length > 0,
    run: (target) => {
      if (target.kind !== 'rows') return undefined;
      return kernel.popover({
        // Where the reader opened it: the right-click, or the badge; the block when neither says.
        anchor: target.at ?? target.node ?? { x: 16, y: 16 },
        about: { app: target.app, query: target.query },
        dismiss: 'outside',
        mount: (root) => drawReasons(root, ours(target)),
      });
    },
  });
}

/** One line per annotation: its label, and its detail under it. The popover's root is bare: it draws its own padding. */
function drawReasons (root: ShadowRoot, annotations: readonly Annotation[]): void {
  const doc = root.ownerDocument;
  const style = doc.createElement('style');
  style.textContent = `
    ul { margin: 0; padding: 12px 14px; list-style: none; display: grid; gap: 8px; max-width: 280px; }
    .label { font-weight: 500; }
    .detail { color: var(--anfra-color-fg-weak, oklch(0.392 0.021 304.3)); }
  `;
  const list = doc.createElement('ul');
  for (const annotation of annotations) {
    const item = doc.createElement('li');
    const label = doc.createElement('div');
    label.className = 'label';
    label.textContent = annotation.label ?? annotation.kind;
    item.append(label);
    if (annotation.detail) {
      const detail = doc.createElement('div');
      detail.className = 'detail';
      detail.textContent = annotation.detail;
      item.append(detail);
    }
    list.append(item);
  }
  root.append(style, list);
}
