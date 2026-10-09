// Formatting for the inspect panel. What it shows is what the Anfra SDK's `App.toInspectJSON()`
// posts out of the frame: its types are the SDK's. Rows are never part of it.
import type { Condition } from '@holistics/anfra-sdk/common';

export type {
  InspectedApp, InspectedControl, InspectedError, InspectedQuery, InspectedSelection,
} from '@holistics/anfra-sdk/common';

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
