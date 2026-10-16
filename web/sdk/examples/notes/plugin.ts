// The notes plugin's host half: it answers the frame's `notes.list` and `notes.add` from a store
// the host gives it, and passes on the frame's `notes.viewed` once checked.
import { ValidationError } from '@holistics/anfra-sdk/common';
import { definePlugin, type FrameHalf, type Plugin } from '@holistics/anfra-sdk/host';
import type { Note } from './types';

/** Where notes live: the host's to decide (its server, the browser), never the SDK's. */
export interface NotesStore {
  list: (about: string, signal: AbortSignal) => Promise<Note[]>;
  add: (about: string, text: string, signal: AbortSignal) => Promise<Note>;
}

const MAX_TEXT = 2000;

/** The frame sends what the definition made up: check it before the store sees it. */
function about (request: unknown): string {
  const value = (request as { about?: unknown } | null)?.about;
  if (typeof value !== 'string' || !value.trim() || value.length > 200) {
    throw new ValidationError('A note is about something: name it, in at most 200 characters.');
  }
  return value;
}

function text (request: unknown): string {
  const value = (request as { text?: unknown } | null)?.text;
  if (typeof value !== 'string' || !value.trim() || value.length > MAX_TEXT) {
    throw new ValidationError(`A note's text is between 1 and ${MAX_TEXT} characters.`);
  }
  return value.trim();
}

/** `frame` is the frame half as built (`buildFrameHalf`); the package's entry passes it in. */
export function createNotesPlugin (store: NotesStore, frame: FrameHalf): Plugin {
  return definePlugin({
    name: 'notes',
    version: '1.0.0',
    host: {
      calls: {
        list: { frame: true, handler: (request, call) => store.list(about(request), call.signal) },
        add: { frame: true, handler: (request, call) => store.add(about(request), text(request), call.signal) },
      },
      events: {
        /** The reader opened the notes about something. */
        viewed: (payload) => {
          const { about: subject, count } = (payload ?? {}) as { about?: unknown, count?: unknown };
          return typeof subject === 'string' && Number.isInteger(count) ? { about: subject, count } : undefined;
        },
      },
    },
    frame,
  });
}
