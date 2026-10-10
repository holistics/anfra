/**
 * Pick mode: while on, pointing at the page outlines the nearest marked element, a click chooses it
 * for the host's inspect panel and reaches nothing else, and Escape leaves the mode.
 */
import { MARKED } from './structure';

export interface PickHandlers {
  /** The marked element under the pointer changed; null when the pointer is on nothing marked. */
  onHover: (element: Element | null) => void;
  /** The reader clicked a marked element. */
  onPick: (element: Element) => void;
  /** The reader pressed Escape. */
  onCancel: () => void;
}

/** Start pick mode on `doc`; returns the function that stops it. */
export function startPick (doc: Document, handlers: PickHandlers): () => void {
  let current: Element | null = null;
  const under = (event: Event): Element | null => (event.target instanceof Element ? event.target.closest(MARKED) : null);
  const onMove = (event: Event) => {
    const element = under(event);
    if (element === current) return;
    current = element;
    handlers.onHover(element);
  };
  const onClick = (event: Event) => {
    event.preventDefault();
    event.stopPropagation();
    const element = under(event);
    if (element) handlers.onPick(element);
  };
  const onKey = (event: KeyboardEvent) => {
    if (event.key === 'Escape') handlers.onCancel();
  };
  const options = { capture: true } as const;
  doc.addEventListener('mousemove', onMove, options);
  doc.addEventListener('click', onClick, options);
  doc.addEventListener('keydown', onKey, options);
  return () => {
    doc.removeEventListener('mousemove', onMove, options);
    doc.removeEventListener('click', onClick, options);
    doc.removeEventListener('keydown', onKey, options);
  };
}
