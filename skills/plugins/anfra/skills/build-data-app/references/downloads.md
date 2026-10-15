# Anfra SDK: downloads

Two ways a reader takes something away from a Data App: a query's **data** as a file
(`query.export()`), or a **picture** of the app or one of its parts (`Anfra.render()`). Add either
only where the agreed design lists it. The rest of the API is in [api.md](api.md).

## Where downloads go

- **At most one download control per block**, in its header. Every format the design agreed for
  that block is an item of it, never a button of its own.
- **A picture of the whole app** is one control in the page's header, never repeated per block.
- **Pictures are the host's first.** `anfra serve` already downloads a PNG or PDF of any block, and
  of the whole page, from its toolbar and Inspect panel. Add a picture item in the app only when
  the user asks for one (a reader on a host without that). The usual block control is one item:
  its data.

## The control

One `↓` button per block. With one item it downloads on click; with more, it opens a menu of them.
Its feedback stays out of the layout, since text added next to it ("Exported 198 rows.") pushes the
card's content around:

- **Shown** only when the host can do it and the reader may: items whose plugin is installed
  (`Anfra.plugins.has('export')` or `Anfra.plugins.has('render')`) and
  `Anfra.user.permissions.canExportData` is true; no items, no button. Where a plugin isn't
  installed its API isn't there at all: `query.export` and `Anfra.render` are `undefined`.
- **Progress is on the button**: `aria-busy`, its label unchanged. **Success needs no message**:
  the browser's own download says it. **Failure goes in a toast** (`position: fixed`, out of the
  flow), gone after a few seconds.
- **Marked `data-anfra-render="skip"`**, so a picture of its block doesn't show it.

```html
<section data-anfra-block="trend" data-anfra-label="Revenue over time">
  <header>
    <h3>Revenue over time</h3>
    <div class="dl" data-anfra-render="skip">
      <button type="button" aria-label="Download" hidden>↓</button>
      <div class="dl-menu" role="menu" hidden></div>
    </div>
  </header>
  …
</section>
<div id="toast" role="status" hidden></div>
<style>
  .dl { position: relative; }
  .dl > button[aria-busy="true"] { cursor: progress; opacity: .6; }
  .dl-menu { position: absolute; right: 0; top: 100%; z-index: 10; display: flex; flex-direction: column;
             padding: 4px; border: 1px solid #ccc; border-radius: 8px; background: #fff; }
  .dl-menu[hidden] { display: none; } /* display: flex above would override [hidden] */
  .dl-menu button { border: 0; background: none; padding: 6px 10px; text-align: left; white-space: nowrap; }
  #toast { position: fixed; bottom: 16px; right: 16px; padding: 8px 12px; border-radius: 6px;
           background: #b42318; color: #fff; }
</style>
```

```js
const toast = document.querySelector('#toast');
const showToast = (text) => {
  toast.textContent = text;
  toast.hidden = false;
  setTimeout(() => { toast.hidden = true; }, 6000);
};

// A block's one download control: `items` are the formats the design agreed for it.
function downloadControl (root, items) {
  const button = root.querySelector('button');
  const menu = root.querySelector('.dl-menu');
  const allowed = items.filter((item) => Anfra.plugins.has(item.feature) && Anfra.user.permissions.canExportData);
  button.hidden = allowed.length === 0;

  const run = async (item) => {
    menu.hidden = true;
    if (button.getAttribute('aria-busy') === 'true') return;
    button.setAttribute('aria-busy', 'true');
    try {
      await item.action();
    } catch (err) {
      showToast(`Download failed: ${err.message}`);
    } finally {
      button.removeAttribute('aria-busy');
    }
  };

  if (allowed.length === 1) {
    button.title = allowed[0].label;
    button.addEventListener('click', () => run(allowed[0]));
    return;
  }
  for (const item of allowed) {
    const entry = document.createElement('button');
    entry.type = 'button';
    entry.setAttribute('role', 'menuitem');
    entry.textContent = item.label;
    entry.addEventListener('click', () => run(item));
    menu.append(entry);
  }
  button.addEventListener('click', (event) => {
    event.stopPropagation();
    menu.hidden = !menu.hidden;
  });
  document.addEventListener('click', () => { menu.hidden = true; });
}

// A picture, telling the reader when an image had to be left blank.
async function picture (request) {
  const { omitted } = await Anfra.render(request);
  if (omitted.length) showToast(`Saved; ${omitted.length} image(s) left blank (no CORS).`);
}

// The usual block: its data only.
downloadControl(document.querySelector('[data-anfra-block="trend"] .dl'), [
  { label: 'Data (CSV)', feature: 'export', action: () => trend.export() },
]);
// Only when the user asked for a picture of this block too:
//   { label: 'Image (PNG)', feature: 'render', action: () => picture({ block: 'trend', format: 'png' }) },
```

## Exporting a query's data

`query.export()` has the host make a file of the query's **whole result** and save it: every row,
not only the pages loaded, with what the result on screen was last run with (the mapped controls,
the selection, the sort, the timezone). A control the reader changed but has not applied is not in
it; `execute()` first if it should be. It changes nothing on the query (`state`, `result` and
`error` stay as they are). Don't build a CSV from `query.result.rows` instead: a paged result holds
only the pages loaded.

- Resolves once the file is made and its download has started, with
  `{ url, filename, format, rowCount, expiresAt }`.
- Options, all optional: `filename` (default: the app's title and the query's name, `Sales -
  byRegion.csv`; the extension is added), `format` (`'csv'`), `formatOptions`
  (`{ header: 'labels' | 'names' | 'none', bom }`; `bom` defaults to `true`, which Excel needs for
  non-ASCII text), `signal` (an `AbortSignal` to cancel it; `query.abort()` does not).
- Values are written raw: numbers with every digit, dates in ISO 8601, `null` as an empty cell.
- A large result takes as long as the query does to run in full.
- Rejects with the query's error classes, naming it in `entity`: a `QueryError` for a bad option,
  a `TransportError` from a host that cannot make exports.

## Pictures of the app

`Anfra.render()` draws the whole app, or one block or container of the
[structure](api.md#structure), as the reader sees it now, and has the host save it as a PNG or a
PDF. The Inspect panel in `anfra serve` offers the same for any block or container, and for the
whole page, with nothing written in the app.

```js
await Anfra.render({ block: 'trend', format: 'png' });       // a block, by its data-anfra-block id
await Anfra.render({ container: 'kpis', format: 'pdf' });    // a container
await Anfra.render({ format: 'pdf' });                       // the whole app
```

- Options: `filename` (default: the app's title and the node's label or id, `Sales - Revenue.png`;
  the extension is added), `pixelRatio` (default: the screen's, at least 2).
- Resolves once the file is saved, with `{ filename, format, width, height, blob, omitted }`.
  `omitted` lists the images left blank, by URL; tell the reader when it isn't empty.
- Rejects with a `ValidationError` naming the node: no element marked with that id, both a block
  and a container, or a picture too large for a canvas.
- It draws at once, what is on the page: `await app.execute()` first if results may be stale.
- A PDF is one page holding the picture, not a printed document: its text isn't selectable.

What comes out as it looks on the page, and what doesn't:

- Charts on a canvas (ECharts' default) or SVG, HTML, CSS, images and fonts served with CORS.
- An entry animation caught mid-way: turn it off (`animation: false`) in an app that may be rendered.
- WebGL is blank unless the chart sets `preserveDrawingBuffer: true`.
- An image whose server sends no CORS headers is left blank, its space kept, and listed in
  `omitted`; a web font without CORS falls back to another font. Serve both from servers that send
  CORS headers (jsDelivr, Google Fonts), or mark such an image `data-anfra-render="skip"`.
- Nothing inside an iframe of the app's own.
