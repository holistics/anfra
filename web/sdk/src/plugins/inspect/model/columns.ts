import type { ColumnMeta } from '../../../common/types';

/** What a result column is, in the words the tabs show: "Metric · sum", "Dimension · date". */
export function columnKind (column: Pick<ColumnMeta, 'isMeasure' | 'aggregation' | 'semantic_type'>): string {
  return column.isMeasure ? `Metric${column.aggregation ? ` · ${column.aggregation}` : ''}` : `Dimension · ${column.semantic_type}`;
}
