# Anfra SDK: downloads

Two ways a reader takes something away from a Data App: a query's **data** as a file
(`query.export()`), or a **picture** of the app or one of its parts (`Anfra.render()`). Add either
only where the agreed design lists it. The rest of the API is in [api.md](api.md).

## The button

Offer a download only where it can work, and keep its feedback out of the layout. A download
button usually sits in a card or a chart's header, and text added next to it ("Exported 198
rows.") pushes the card's content around. So:

- **Shown** only when the host can do it and the reader may: `Anfra.features.export` or
  `Anfra.features.render`, and `Anfra.user.permissions.canExportData`.
- **Progress is on the button**: disabled, with `aria-busy`, its label keeping its width.
- **Success needs no message**: the browser's own download is the confirmation.
- **Failure goes in a toast** (`position: fixed`, out of the flow), gone after a few seconds.
- **Marked `data-anfra-render="skip"`**, so a picture of its card doesn't show it.

```html
<button id="dl-trend" class="dl" data-anfra-render="skip" hidden>↓ CSV</button>
<button id="png-trend" class="dl" data-anfra-render="skip" hidden>↓ PNG</button>
<div id="toast" role="status" hidden></div>
<style>
  .dl[aria-busy="true"] { cursor: progress; opacity: .6; }
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

// One button, one download: `action` is the export or the render.
function downloadButton (button, feature, action) {
  button.hidden = !(Anfra.features[feature] && Anfra.user.permissions.canExportData);
  button.addEventListener('click', async () => {
    button.disabled = true;
    button.setAttribute('aria-busy', 'true');
    try {
      await action();
    } catch (err) {
      showToast(`Download failed: ${err.message}`);
    } finally {
      button.disabled = false;
      button.removeAttribute('aria-busy');
    }
  });
}

downloadButton(document.querySelector('#dl-trend'), 'export', () => trend.export());
downloadButton(document.querySelector('#png-trend'), 'render', async () => {
  const { omitted } = await Anfra.render({ block: 'trend', format: 'png' });
  if (omitted.length) showToast(`Saved; ${omitted.length} image(s) left blank (no CORS).`);
});
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
