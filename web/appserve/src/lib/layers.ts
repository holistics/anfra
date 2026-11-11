// Whether a layer (a menu, a dialog, a popover) is open: it takes Escape for itself, so the shell's
// own use of Escape (ending a pick, closing the panel) waits. reka marks each open layer with
// data-dismissable-layer, and appserve's own popovers wear the same mark. reka handles Escape on
// window after the shell does, without marking the event, so the event alone can't tell. A
// tooltip is a layer too, but only names a control: one shown on a focused button must not
// swallow the reader's Escape.
export const layerOpen = (doc: Document = document): boolean => doc.querySelector('[data-dismissable-layer]:not([data-slot="tooltip-content"])') !== null;
