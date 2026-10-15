# Anfra SDK: downloads

Two ways a reader takes something away from a Data App: a query's **data** as a file
(`query.export()`, the [export plugin](plugins/export.md)), or a **picture** of the app or one of
its parts (`Anfra.render()`, the [render plugin](plugins/render.md)). Add either only where the
agreed design lists it. This file is where their control goes and how it behaves; each plugin's
reference has its API. The rest of the SDK is in [api.md](api.md).

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
