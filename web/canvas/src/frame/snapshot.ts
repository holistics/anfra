// A still of the screen, for when its frame is far from the view and its live app is let go: the
// page as it is drawn now, with its styles, as an SVG image the canvas shows in the frame's place.
// An SVG image loads nothing, so the app's web fonts and images are left out; at the size a far
// frame is seen at, that reads the same.

const xml = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

export function snapshot (doc: Document): string {
  const win = doc.defaultView!;
  const css = [...doc.styleSheets].map((sheet) => {
    try {
      return [...sheet.cssRules].map((r) => r.cssText).join('\n');
    } catch {
      return ''; // another origin's sheet
    }
  }).join('\n');

  // Form fields show what is typed in them, which is not in the markup.
  const body = doc.body.cloneNode(true) as HTMLElement;
  const live = doc.body.querySelectorAll('input, textarea, select');
  body.querySelectorAll('input, textarea, select').forEach((el, i) => {
    const from = live[i] as HTMLInputElement;
    if (el instanceof win.HTMLTextAreaElement) el.textContent = from.value;
    else if (el instanceof win.HTMLInputElement && (from.type === 'checkbox' || from.type === 'radio')) el.toggleAttribute('checked', from.checked);
    else el.setAttribute('value', from.value);
  });
  body.querySelectorAll('script, iframe, noscript').forEach((el) => el.remove());

  const root = doc.documentElement;
  const w = win.innerWidth;
  const h = win.innerHeight;
  const html = new win.XMLSerializer().serializeToString(body);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">`
    + `<foreignObject x="0" y="${-win.scrollY}" width="${w}" height="${Math.max(h, root.scrollHeight)}">`
    + `<html xmlns="http://www.w3.org/1999/xhtml" class="${xml(root.className)}" style="${xml(root.getAttribute('style') ?? '')}">`
    + `<head><style>${xml(css)}</style></head>${html}</html></foreignObject></svg>`;
}
