# Data App URLs are bare paths, and Data App serving reserves `/_anfra/`

A Data App is addressed as `/sales/overview` (its path under `apps/`, no extension), not as `/#/sales/overview.html`, so a URL reads like the thing it opens and can be shared as it is. To keep Data Apps from colliding with Data App serving's own routes, everything the server owns (`api`, `data-apps`, the Shell's `assets`) lives under one reserved prefix, `/_anfra/`. A Data App that would land inside it is left out of the catalog, with a warning at startup.

## Considered Options

- **A fixed prefix for Data Apps (`/app/sales/overview`).** No clash is possible, but the URL is longer and not the bare form that was asked for. Rejected.
- **Bare URLs and keep `/api`, `/data-apps`, `/assets`.** Rejected: those are plausible folder names in a Repo's `apps/`, and each would silently shadow a Data App.

## Consequences

- Folders have no URL, and neither do old `/#/…` or `….html` URLs: they show the Shell's not-found view.
- The server answers every other GET with the Shell, so a refresh on a Data App URL works.
- Moving the internal routes later means rebuilding the Shell, since its bundle is built for the `/_anfra/` base.
