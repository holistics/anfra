/**
 * The annotations badge, the kernel's side: which marked elements get one, and for which
 * annotations. An element marked for a query (`data-anfra-query`) declares the kinds its definition
 * draws (`data-anfra-annotations="mark line"`, or `off` for no badge); the badge counts the rest.
 * What draws it, and its popover's list, is the kernel's UI body, fetched once a badge is needed, in
 * the plugin layer, so renderings leave it out. See docs/designs/sdk-plugins.md, Annotations.
 */
import type { Annotation, AnnotationKind, AnnotationSeverity } from '../common/types';
import type { App } from './app';
import type { FrameKernel } from './kernel';
import type { Query } from './query';
import { annotationNote, type ContentUi, type FrameMenuEntry } from './menu';
import { layerRoot } from './layer';
import { reportError } from './reporting';
import { ATTR, markersFor } from './structure';
import { ANNOTATION_KINDS, SEVERITIES } from './annotations';
import type { Badge, Rect, UiBody } from './ui/body';

/** Where an element declares the kinds its definition draws. */
export const ANNOTATIONS_ATTRIBUTE = 'data-anfra-annotations';

/** The kinds an element draws itself, or `off` for no badge at all. */
function drawnBy (element: Element): ReadonlySet<AnnotationKind> | 'off' {
  const value = element.getAttribute(ANNOTATIONS_ATTRIBUTE);
  if (value?.trim() === 'off') return 'off';
  return new Set((value ?? '').split(/\s+/).filter((kind): kind is AnnotationKind => ANNOTATION_KINDS.includes(kind as AnnotationKind)));
}

/** The annotations a badge on this element covers: those of the kinds it doesn't draw. */
export function badgedAnnotations (element: Element, annotations: readonly Annotation[]): Annotation[] {
  const drawn = drawnBy(element);
  return drawn === 'off' ? [] : annotations.filter((annotation) => !drawn.has(annotation.kind));
}

/** The highest severity among these, if any has one. */
function highest (annotations: readonly Annotation[]): AnnotationSeverity | undefined {
  let top = -1;
  for (const { severity } of annotations) top = Math.max(top, severity ? SEVERITIES.indexOf(severity) : -1);
  return top < 0 ? undefined : SEVERITIES[top];
}

const describe = (count: number, severity?: AnnotationSeverity): string => `${count} annotation${count === 1 ? '' : 's'}${severity ? `, highest: ${severity}` : ''}`;

/** Start the badges in a frame; `kernel` is the kernel's own. */
export function installAnnotationBadges (kernel: FrameKernel, contentUi: ContentUi): void {
  const doc = kernel.window.document;
  const realm = kernel.window as Window & typeof globalThis;
  const apps: App[] = [];
  let scheduled = false;
  let drawnAny = false;
  let observer: MutationObserver | undefined;
  /**
   * A number per `query.annotations` list, which stays the same list until a set or the result
   * changes: a redraw compares them, as an open popover's badge must not be redrawn for nothing.
   */
  const lists = new WeakMap<readonly Annotation[], number>();
  let listed = 0;
  const listId = (list: readonly Annotation[]) => {
    if (!lists.has(list)) lists.set(list, (listed += 1));
    return lists.get(list)!;
  };

  const ui = () => kernel.body<UiBody>();

  /** One annotation's actions: the rows menu entries of the plugin that set it, for its rows. */
  function actionsFor (app: App, query: Query, annotation: Annotation): FrameMenuEntry[] {
    const rows = query.result?.rows;
    if ((annotation.kind !== 'mark' && annotation.kind !== 'focus') || !rows) return [];
    const target = app.rowsTarget(query, annotation.anchor.index.map((at) => rows[at]), undefined, annotation.anchor.measure);
    return contentUi.rowsEntries(annotation.plugin, { ...target, annotations: [annotation] });
  }

  /** The badge's popover: each annotation it covers, with its actions; picking one closes it. */
  function open (app: App, query: Query, covered: Annotation[], anchor: () => Rect | undefined): void {
    const opening = contentUi.popoverAt(anchor, {
      about: { app: app.index, query: query.name },
      side: 'bottom',
      dismiss: 'outside',
      mount: (root) => {
        void ui().then((body) => body.annotationList(
          root,
          covered.map((annotation) => ({ ...annotationNote(annotation), actions: actionsFor(app, query, annotation) })),
          (action) => {
            void opening.then((opened) => opened.close());
            Promise.resolve().then(action.run).catch((err: unknown) => {
              realm.console?.error('anfra: an annotation\'s action failed:', err);
              reportError('plugin.uncaught', err);
            });
          },
        ));
      },
    });
  }

  /** What the badges drawn last showed, so a page that changes nothing about them redraws none. */
  let shown: { signature: string, elements: Element[] } = { signature: '', elements: [] };

  async function draw (): Promise<void> {
    scheduled = false;
    const list: Badge[] = [];
    const parts: string[] = [];
    let annotated = false;
    for (const app of apps) {
      for (const query of Object.values(app.queries)) {
        const { annotations } = query;
        if (!annotations.length) continue;
        annotated = true;
        markersFor(doc, 'query', app.index, query.name).forEach((element, position) => {
          const covered = badgedAnnotations(element, annotations);
          if (!covered.length) return;
          const severity = highest(covered);
          const id = `${app.index}/${query.name}#${position}`;
          parts.push(`${id}:${listId(annotations)}:${covered.length}`);
          list.push({
            id,
            element,
            count: covered.length,
            ...(severity ? { severity } : {}),
            label: describe(covered.length, severity),
            onOpen: (anchor) => open(app, query, covered, anchor),
          });
        });
      }
    }
    watchPage(annotated);
    const elements = list.map((badge) => badge.element);
    const signature = parts.join('|');
    if (signature === shown.signature && elements.every((element, at) => element === shown.elements[at])) return;
    shown = { signature, elements };
    // No badge, and none drawn yet: the body isn't fetched.
    if (!list.length && !drawnAny) return;
    drawnAny = true;
    const body = await ui();
    body.badges(layerRoot(doc, 'anfra-badges'), list);
  }

  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(() => {
      draw().catch((err: unknown) => {
        realm.console?.error('anfra: the annotations badge failed to draw:', err);
        reportError('plugin.uncaught', err);
      });
    });
  };

  /** While some query has annotations, the page's marks are watched: an element marked later gets its badge. */
  function watchPage (on: boolean): void {
    if (on && !observer && typeof realm.MutationObserver === 'function') {
      observer = new realm.MutationObserver(schedule);
      observer.observe(doc.documentElement, {
        subtree: true, childList: true, attributes: true, attributeFilter: [ATTR.query, ANNOTATIONS_ATTRIBUTE],
      });
    } else if (!on && observer) {
      observer.disconnect();
      observer = undefined;
    }
  }

  kernel.onApp((app) => {
    apps.push(app);
    app.subscribe(schedule);
    app.onRemoved(schedule);
  });
}
