// The notes plugin's frame half: `Anfra.notes` and `query.notes`, and a panel in the plugin layer.
// It imports only types: everything it reaches, it reaches through the kernel.
import type { FrameKernel, Query } from '@holistics/anfra-sdk/app';
import type { Note, NotesApi, QueryNotes } from './types';

const STYLE = `
  aside { position: fixed; top: 16px; right: 16px; width: 280px; max-height: 60vh; overflow: auto;
          padding: 12px; border-radius: 8px; background: #fff; color: #111; font: 13px/1.4 system-ui, sans-serif;
          box-shadow: 0 8px 24px rgb(0 0 0 / 20%); }
  h2 { margin: 0 0 8px; font-size: 14px; }
  li { margin: 4px 0; }
  button { float: right; border: 0; background: none; cursor: pointer; }
`;

export default function setup (kernel: FrameKernel): void {
  const doc = kernel.window.document;
  const list = (about: string) => kernel.call<Note[]>('notes.list', { about });
  const add = (about: string, text: string) => kernel.call<Note>('notes.add', { about, text });

  async function show (about: string): Promise<void> {
    const notes = await list(about);
    const root = kernel.layer();
    const style = doc.createElement('style');
    style.textContent = STYLE;
    const panel = doc.createElement('aside');
    const close = doc.createElement('button');
    close.textContent = '×';
    close.setAttribute('aria-label', 'Close');
    close.onclick = () => root.replaceChildren();
    const title = doc.createElement('h2');
    title.textContent = `Notes on ${about}`;
    const items = doc.createElement('ul');
    for (const note of notes) {
      const item = doc.createElement('li');
      item.textContent = note.text; // Text, never HTML: a note is anyone's words.
      items.append(item);
    }
    panel.append(close, title, items);
    root.replaceChildren(style, panel);
    kernel.emit('viewed', { about, count: notes.length });
  }

  const api: NotesApi = { list, add, show };
  kernel.provide(api);
  kernel.provideOnQuery((query: Query): QueryNotes => ({
    list: () => list(query.name),
    add: (text) => add(query.name, text),
    show: () => show(query.name),
  }));
}
