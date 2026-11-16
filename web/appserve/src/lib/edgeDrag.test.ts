import { beforeAll, describe, expect, it, vi } from 'vitest';
import { startEdgeDrag } from './edgeDrag';

beforeAll(() => {
  // jsdom has no pointer capture.
  HTMLElement.prototype.setPointerCapture ??= () => {};
});

const pointer = (type: string, init: MouseEventInit = {}) => new MouseEvent(type, { button: 0, ...init }) as PointerEvent;

/** Presses an edge, starting a drag with spies for its moves and its end. */
function press (init: MouseEventInit = {}) {
  const edge = document.createElement('button');
  const move = vi.fn();
  const end = vi.fn();
  let started = false;
  edge.addEventListener('pointerdown', (event) => { started = startEdgeDrag(event as PointerEvent, { move, end }); });
  edge.dispatchEvent(pointer('pointerdown', { clientX: 100, ...init }));
  return { edge, move, end, started };
}

describe('dragging an edge', () => {
  it('follows the pointer once it moves past the threshold, and ends on the release', () => {
    const { edge, move, end } = press();
    edge.dispatchEvent(pointer('pointermove', { clientX: 102 }));
    expect(move).not.toHaveBeenCalled();
    edge.dispatchEvent(pointer('pointermove', { clientX: 140 }));
    expect(move).toHaveBeenCalledWith(40, expect.anything());
    edge.dispatchEvent(pointer('pointerup'));
    expect(end).toHaveBeenCalledExactlyOnceWith(true);
  });

  it('is a click when the pointer barely moves', () => {
    const { edge, end } = press();
    edge.dispatchEvent(pointer('pointermove', { clientX: 101 }));
    edge.dispatchEvent(pointer('pointerup'));
    expect(end).toHaveBeenCalledWith(false);
  });

  it('ends when the capture is lost some other way, and only once', () => {
    const { edge, move, end } = press();
    edge.dispatchEvent(pointer('pointermove', { clientX: 140 }));
    edge.dispatchEvent(pointer('lostpointercapture'));
    edge.dispatchEvent(pointer('pointerup'));
    edge.dispatchEvent(pointer('pointermove', { clientX: 200 }));
    expect(end).toHaveBeenCalledOnce();
    expect(move).toHaveBeenCalledOnce();
  });

  it('ends on a cancel', () => {
    const { edge, end } = press();
    edge.dispatchEvent(pointer('pointercancel'));
    expect(end).toHaveBeenCalledOnce();
  });

  it('starts nothing for another button than the main one', () => {
    const { edge, move, end, started } = press({ button: 2 });
    edge.dispatchEvent(pointer('pointermove', { clientX: 200 }));
    edge.dispatchEvent(pointer('pointerup'));
    expect(started).toBe(false);
    expect(move).not.toHaveBeenCalled();
    expect(end).not.toHaveBeenCalled();
  });
});
