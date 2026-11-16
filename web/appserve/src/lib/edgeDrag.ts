// Dragging an edge that resizes something: the sidebar's, the Inspect panel's. The edge captures
// the pointer from the press, a move past DRAG_PX is a drag (less is a click), and the drag ends
// once, however it ends: the release, a cancel, or the capture lost some other way (the window
// losing focus, a dialog taking the pointer). Without that last one a drag could be left running,
// its listeners on the edge and the Data App's frame kept from the pointer.

/** A drag moves at least this far; less is a click. */
export const DRAG_PX = 3;

export interface EdgeDrag {
  /** The pointer moved, `dx` from where it was pressed. */
  move: (dx: number, event: PointerEvent) => void;
  /** The drag ended; `dragged` is false for a click. */
  end: (dragged: boolean) => void;
}

/** Starts a drag on the edge the pointer pressed (`event.currentTarget`); a press with another button than the main one starts nothing. */
export function startEdgeDrag (event: PointerEvent, drag: EdgeDrag): boolean {
  if (event.button !== 0) return false;
  const edge = event.currentTarget as HTMLElement;
  edge.setPointerCapture(event.pointerId);
  const startX = event.clientX;
  let dragged = false;

  const move = (moved: PointerEvent): void => {
    if (!dragged && Math.abs(moved.clientX - startX) < DRAG_PX) return;
    dragged = true;
    drag.move(moved.clientX - startX, moved);
  };
  const end = (): void => {
    edge.removeEventListener('pointermove', move);
    edge.removeEventListener('pointerup', end);
    edge.removeEventListener('pointercancel', end);
    edge.removeEventListener('lostpointercapture', end);
    drag.end(dragged);
  };
  edge.addEventListener('pointermove', move);
  edge.addEventListener('pointerup', end);
  edge.addEventListener('pointercancel', end);
  edge.addEventListener('lostpointercapture', end);
  return true;
}
