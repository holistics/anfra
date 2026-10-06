// What the Anfra SDK's `App.toInspectJSON()` posts out of the frame (mirrors its `Inspected*` types).
// Rows are never part of it.
export interface Condition { operator: string, values?: (string | number | boolean)[], modifier?: string }

export interface InspectedError { name: string, message: string, entity?: string, status?: number }

export interface InspectedQuery {
  state: 'idle' | 'executing' | 'success' | 'error';
  isDirty: boolean;
  page: number;
  hasMore: boolean;
  rowCount: number;
  selectedRowCount: number;
  signature: string;
  error?: InspectedError;
  debug?: { executedAql?: string, executedSql?: string, fromCache: boolean, executedAt?: Date | string };
}

export interface InspectedControl {
  kind: 'query' | 'filter' | 'dateDrill';
  condition: Condition;
  appliedCondition?: Condition;
  isDirty: boolean;
  options?: (string | number | boolean)[];
}

export interface InspectedSelection {
  source: string;
  fields: string[];
  rowCount: number;
  conditions: { field: string, operator: string, values: (string | number | boolean)[] }[];
  expression?: string;
  lossy: boolean;
}

export interface InspectedApp {
  title?: string;
  timezone?: string;
  hasChanges: boolean;
  queries: Record<string, InspectedQuery>;
  controls: Record<string, InspectedControl>;
  selection?: InspectedSelection;
  appliedSelection?: InspectedSelection;
}

export function formatCondition (condition: Condition | undefined): string {
  if (!condition) return 'not applied yet';
  const parts = [condition.operator, condition.modifier, ...(condition.values ?? []).map(String)];
  return parts.filter((part) => part !== undefined && part !== '').join(' ');
}

export function formatTime (value: Date | string | undefined): string {
  if (!value) return '';
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleTimeString();
}
