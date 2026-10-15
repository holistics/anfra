import { TransportError } from '../common/errors';
import type { components } from './schema';
import { toSdkError } from './errors';

type StreamEnd = components['schemas']['StreamEnd'];

/**
 * Reads a core op's streamed answer (`Accept: application/x-ndjson`): one JSON event a line, handed
 * to `onEvent` as each arrives, until `end`. A failed `end` rejects with the SDK error its error is,
 * as an error response would; a stream that stops without `end` was cut, and rejects too. `end` and
 * `ping` are this reader's; every other event, `start` and `status` included, is the caller's.
 */
export async function readEvents<E extends { type: string }> (
  body: ReadableStream<Uint8Array>,
  onEvent: (event: E) => void,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffered = '';
  const line = (text: string): boolean => {
    if (!text.trim()) return false;
    const event = JSON.parse(text) as E | StreamEnd;
    if (event.type === 'ping') return false;
    if (event.type === 'end') {
      const end = event as StreamEnd;
      if (end.status === 'failed') throw toSdkError(end.error?.status ?? 500, end.error);
      return true;
    }
    onEvent(event as E);
    return false;
  };
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buffered += decoder.decode(value, { stream: true });
      let start = 0;
      for (let newline = buffered.indexOf('\n'); newline >= 0; newline = buffered.indexOf('\n', start)) {
        if (line(buffered.slice(start, newline))) return;
        start = newline + 1;
      }
      buffered = buffered.slice(start);
    }
    if (line(buffered + decoder.decode())) return;
  } finally {
    reader.releaseLock();
  }
  throw new TransportError('The connection to the server was cut before the answer ended.');
}
