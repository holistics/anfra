// The annotations example's frame half: it sets one annotation of each kind on every query's result,
// from trivial rules, so a definition's drawing of them can be tried. A real plugin (an anomaly
// detector, a forecast) computes them the same way and annotates each new result. It imports only
// types: everything it reaches, it reaches through the kernel.
import type {
  AnnotationInput, FrameKernel, MenuTarget, Query, QueryResult, Row,
} from '@holistics/anfra-sdk/app';

/** A number as rows carry it: a string with no float noise (`55`, not `55.00000000000001`). */
const text = (value: number): string => String(Number(value.toPrecision(12)));

const DAY = /^(\d{4})-(\d{2})-(\d{2})$/;

/**
 * The next `count` periods after the last two, when they are dates: by whole months when both are
 * on the 1st, else by the days between them. None when they aren't dates.
 */
function nextPeriods (before: string, last: string, count: number): string[] {
  const a = DAY.exec(before);
  const b = DAY.exec(last);
  if (!a || !b) return [];
  const pad = (value: number) => String(value).padStart(2, '0');
  const format = (date: Date) => `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())}`;
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

/** One annotation of each kind, on the result's first dimension and first measure. */
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
  if (rows.length >= 2) {
    const [before, last] = rows.slice(-2);
    annotations.push({
      id: 'latest', kind: 'band', anchor: { column: period, from: String(before[period]), to: String(last[period]) }, label: 'Latest two periods', severity: 'info',
    });
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
    run: (target) => kernel.popover({
      anchor: target.kind === 'rows' && target.node ? target.node : { x: 16, y: 16 },
      dismiss: 'outside',
      mount: (root) => {
        root.textContent = ours(target).map((annotation) => annotation.detail ?? annotation.label ?? annotation.kind).join(' ');
      },
    }),
  });
}
