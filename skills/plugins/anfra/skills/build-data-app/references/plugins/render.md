# Plugin: render

`Anfra.render()`, from the `render` plugin. It is there only where the host installed the plugin:
check `Anfra.plugins.has('render')`, and `Anfra.user.permissions.canExportData`, before offering it
(see [api.md](../api.md#plugins)). Its button follows [downloads.md](../downloads.md).


`Anfra.render()` draws the whole app, or one block or container of the
[structure](../api.md#structure), as the reader sees it now, and has the host save it as a PNG or a
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
