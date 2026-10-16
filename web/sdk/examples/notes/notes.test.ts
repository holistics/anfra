// The notes plugin, tested as the usage guide says (docs/usage/sdk-plugins.md, Testing): the host
// half through a host runtime, the frame half in a test frame.
import { describe, expect, it } from 'vitest';
import { ValidationError } from '@holistics/anfra-sdk/common';
import { createHost } from '@holistics/anfra-sdk/host';
import { corePlugin } from '@holistics/anfra-sdk/plugins';
import { testFrame, flush } from '@holistics/anfra-sdk/testing';
import { createNotesPlugin, type NotesStore } from './plugin';
import setup from './frame';
import type { Note } from './types';
import './types';

function memoryStore (): NotesStore & { notes: Note[] } {
  const notes: Note[] = [];
  return {
    notes,
    list: async (about) => notes.filter((note) => note.about === about),
    add: async (about, text) => {
      const note = { id: String(notes.length + 1), about, text, at: '2026-10-16T00:00:00Z' };
      notes.push(note);
      return note;
    },
  };
}

const noBackend = { submitQuery: async () => ({ columns: [], values: [] }), fieldSuggestions: async () => [] };

describe('the host half', () => {
  it('keeps notes in the host\'s store, and refuses what isn\'t one', async () => {
    const store = memoryStore();
    const host = createHost({ plugins: [corePlugin(noBackend), createNotesPlugin(store, { setup: '' })] });

    await host.call('notes.add', { about: 'revenue', text: '  Q3 includes the refund.  ' });
    await expect(host.call('notes.list', { about: 'revenue' })).resolves.toMatchObject([{ text: 'Q3 includes the refund.' }]);
    await expect(host.call('notes.add', { about: 'revenue', text: '' })).rejects.toBeInstanceOf(ValidationError);
    await expect(host.call('notes.add', { about: 42, text: 'x' })).rejects.toBeInstanceOf(ValidationError);
  });

  it('passes on only a `viewed` event of its shape', () => {
    const { viewed } = createNotesPlugin(memoryStore(), { setup: '' }).host!.events!;
    const mount = {} as never;
    expect(viewed({ about: 'revenue', count: 2 }, mount)).toEqual({ about: 'revenue', count: 2 });
    expect(viewed({ about: 'revenue', count: 'many' }, mount)).toBeUndefined();
  });
});

describe('the frame half', () => {
  const notes: Note[] = [{
    id: '1', about: 'revenue', text: '<b>Q3</b> includes the refund.', at: '2026-10-16T00:00:00Z',
  }];

  it('adds Anfra.notes and query.notes, which make the plugin\'s calls', async () => {
    const frame = testFrame({
      setups: [['notes', setup]],
      datasets: { sales: { name: 'sales', label: 'Sales', data_models: [], metrics: [] } as never },
      answers: {
        'notes.list': () => notes,
        'notes.add': (request: { about: string, text: string }) => ({ ...request, id: '2', at: '' }),
      },
    });
    const query = frame.anfra.createApp().createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });

    await expect(frame.anfra.notes!.list('revenue')).resolves.toEqual(notes);
    await query.notes!.add('Checked with finance.');
    expect(frame.calls.at(-1)).toEqual({ name: 'notes.add', request: { about: 'revenue', text: 'Checked with finance.' } });
  });

  it('shows notes as text in its part of the plugin layer, and says so', async () => {
    const frame = testFrame({ setups: [['notes', setup]], answers: { 'notes.list': () => notes } });

    await frame.anfra.notes!.show('revenue');
    await flush();

    const panel = document.querySelector('[data-anfra-layer] [data-part="notes"]')!.shadowRoot!;
    expect(panel.querySelector('li')!.textContent).toBe('<b>Q3</b> includes the refund.');
    expect(panel.querySelector('b')).toBeNull();
    expect(frame.events).toEqual([{ name: 'notes.viewed', payload: { about: 'revenue', count: 1 } }]);
  });
});
