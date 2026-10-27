// How the canvas and the app in each of its frames talk. Frames are same-origin (both come from
// the app's dev server), so they call each other directly: the frame's agent registers with the
// canvas on load, the canvas sets its mode and asks it where things are, and the agent reports
// what the pointer does. Nothing here is a security boundary: it is a dev tool on the dev server.
import type { ElementRef, Rect } from '../model/board.ts';

/** What the pointer does in a frame: pick elements (`inspect`, `comment`), use the app, or nothing. */
export type FrameMode = 'inspect' | 'comment' | 'interact' | 'idle';

/** An element as the canvas shows it: how to find it again, and what it looks like now. */
export interface ElementInfo {
  ref: ElementRef;
  /** In the frame's viewport, in CSS pixels. */
  rect: Rect;
  /** Its own text only, with no elements inside: the text can be edited in place. */
  textOnly: boolean;
  classes: string;
  style: Record<string, string>;
}

export type FrameEvent =
  | { type: 'hover', info?: ElementInfo }
  | { type: 'pick', info: ElementInfo, shift: boolean, point: { x: number, y: number } }
  | { type: 'activate', info: ElementInfo }
  | { type: 'layout' }
  | { type: 'route', route: string }
  | { type: 'wheel', deltaX: number, deltaY: number, zoom: boolean, x: number, y: number }
  | { type: 'key', key: string, meta: boolean, shift: boolean };

export interface FrameAgent {
  setMode: (mode: FrameMode) => void;
  /** Where an element is now, or undefined when the screen no longer shows it. */
  locate: (ref: ElementRef) => ElementInfo | undefined;
  /** The element a person would pick at a point of the frame's viewport. */
  pickAt: (x: number, y: number) => ElementInfo | undefined;
  /** Edit an element's text in place; answers the text before and after, or undefined if cancelled. */
  editText: (ref: ElementRef) => Promise<{ from: string, to: string } | undefined>;
  route: () => string;
}

export interface CanvasHost {
  attach: (frame: string, agent: FrameAgent) => void;
  detach: (frame: string, agent: FrameAgent) => void;
  event: (frame: string, event: FrameEvent) => void;
}

declare global {
  interface Window {
    __anfraCanvasHost?: CanvasHost;
  }
}

/** A frame's window name carries its id and scenario, which survive the app's own navigation. */
export const FRAME_NAME_PREFIX = 'anfra-canvas';

export const frameName = (id: string, scenario?: string) => [FRAME_NAME_PREFIX, id, scenario ?? ''].join('|');

export function parseFrameName (name: string): { id: string, scenario?: string } | undefined {
  const [prefix, id, scenario] = name.split('|');
  return prefix === FRAME_NAME_PREFIX && id ? { id, scenario: scenario || undefined } : undefined;
}

/** The attributes the Vite plugin writes on every element: where it is written, and its component. */
export const SOURCE_ATTR = 'data-canvas-src';
export const COMPONENT_ATTR = 'data-canvas-cmp';
