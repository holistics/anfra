/**
 * Trust: how far an app's numbers trace back to the semantic layer. Each query gets a level, and
 * the app's is its lowest query's. A query that declares a dataset goes through the semantic
 * layer. The fourth level, Endorsed, is defined but not reached until the SDK reports endorsement.
 */
import type { InspectedApp, InspectedQuery, QueryDeclaration } from '../../../common/types';
import { plural } from './format';

export type TrustLevel = 1 | 2 | 3 | 4;
export type Tone = 'error' | 'warn' | 'ok';

export interface Trust {
  level: TrustLevel;
  tone: Tone;
  /** Why, in a sentence. */
  text: string;
}

/** Each level's name, tone and what it means, for the meter's popover. */
export const TRUST_LEVELS: Record<TrustLevel, { tone: Tone, name: string, points: string[] }> = {
  1: {
    tone: 'error',
    name: 'Unverified',
    points: ['A query has no dataset, or there are no queries', "Numbers can't be traced to the semantic layer"],
  },
  2: {
    tone: 'warn',
    name: 'Verified, incomplete',
    points: ['All queries use the semantic layer', "Some failed or haven't returned yet, so the app may be incomplete"],
  },
  3: {
    tone: 'ok',
    name: 'Semantic layer verified',
    points: ['All queries use the semantic layer and succeeded', 'Some fields are still ad hoc expressions'],
  },
  4: {
    tone: 'ok',
    name: 'Endorsed',
    points: ['All queries use the semantic layer and succeeded', 'Every field is an endorsed object from the semantic layer, none ad hoc'],
  },
};

/** The levels, lowest first. */
export const LEVELS = Object.keys(TRUST_LEVELS).map(Number) as TrustLevel[];

const at = (level: TrustLevel, text: string): Trust => ({ level, tone: TRUST_LEVELS[level].tone, text });

const adhocCount = (query: InspectedQuery) => query.columns.filter((column) => column.adhoc).length;

/** The SDK doesn't report endorsement yet, so nothing is endorsed: wiring it up is this function alone. */
const isEndorsed = (_query: InspectedQuery) => false;

/** One query's level, with why. */
export function queryTrust (declaration: QueryDeclaration | undefined, query: InspectedQuery): Trust {
  if (!declaration?.dataset) return at(1, 'Not on the semantic layer: declares no dataset');
  if (query.state === 'error') return at(2, 'On the semantic layer, but failed');
  // Verified means it came back: a query that hasn't yet can't be counted as verified.
  if (query.state !== 'success') return at(2, "On the semantic layer, but hasn't returned yet");
  if (isEndorsed(query)) return at(4, 'Endorsed');
  const adhoc = adhocCount(query);
  return at(3, adhoc ? `Semantic layer verified · ${plural(adhoc, 'ad hoc field')}` : 'Semantic layer verified');
}

/** The app's level: its lowest query's, or 1 with no queries. */
export function appTrust (app: InspectedApp): Trust {
  const queries = Object.entries(app.queries);
  if (queries.length === 0) return at(1, 'No queries found');
  const levels = queries.map(([name, query]) => queryTrust(app.declaration.queries[name], query).level);
  const level = Math.min(...levels) as TrustLevel;
  const n = levels.filter((l) => l === level).length;
  const total = queries.length;
  if (level === 1) return at(1, `${n} of ${total} queries aren't on the semantic layer`);
  if (level === 2) {
    const failed = queries.filter(([, query]) => query.state === 'error').length;
    const waiting = n - failed;
    const parts = [failed && `${failed} of ${total} queries failed`, waiting && `${waiting} of ${total} haven't returned yet`];
    return at(2, parts.filter(Boolean).join(' · '));
  }
  if (level === 4) return at(4, 'All queries are endorsed');
  const adhoc = queries.filter(([, query]) => adhocCount(query) > 0).length;
  return at(3, adhoc ? `All queries are semantic layer verified · ${adhoc} with ad hoc fields` : 'All queries are semantic layer verified');
}
