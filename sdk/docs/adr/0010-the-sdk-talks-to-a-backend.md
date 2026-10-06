# The SDK talks to a provisioned Backend, not to Holistics' web API

The SDK used to call Holistics' HTTP endpoints by path through its own fetch client (`submit_execute_aql`, `get_execute_aql_result`, `submit_fetch_field_suggestions`) and poll asynchronous jobs itself. It now takes a required `backend` from the provisioner, with two methods, `submitQuery` and `fieldSuggestions`. Each takes an `AbortSignal` and returns a promise, and the SDK ships no implementation of either. That keeps the SDK a pure client-side library with no knowledge of any server, so it runs the same against anfra, against Holistics, or across an iframe bridge, wherever the provisioner puts it.

A query request carries the dataset name and the AQL verbatim. It also carries the **Query Input** (`input: { filters, conditions, sorts, dateDrills }`) and the **Execution Options** (`page`, `pageSize`, `timezone`, `bustCache`; `page` and `pageSize` only for a query that declares a `pageSize`), and the backend decides how to apply them; anfra rewrites the AQL with them. A date drill travels as `input.dateDrills`, not as a `transform_date_drill` filter. The control still spells its state that way internally, because selections size their ranges by it.

## Consequences

- No job polling in the SDK. A backend that runs queries asynchronously polls inside its own `submitQuery`; the SDK only awaits a promise. ADR 0002's "cancelling in-flight jobs" is now "aborting the earlier call's signal".
- Errors: a backend that rejects with a `DataAppError` (`PermissionError`, `QueryError`, …) chooses the error an author sees. Anything else becomes a `QueryError`. `TransportError` remains for backends that want to report a network failure as one.
- Supersede and `abort()` abort the earlier call's signal. A backend that cannot cancel may still resolve, and the SDK ignores the stale result.
- Paging is opt-in. A query that declares no `pageSize` sends no `page` or `pageSize` and gets every row; the SDK sets no default and no cap, and any row limit is the backend's to apply. A pivot query (an explore with `rows { }` / `columns { }`) must not declare one: anfra rejects paging on it as a `QueryError`, "Pagination for pivot queries isn't supported yet.", because LIMIT/OFFSET would split a pivot's grid rows and drop its header rows.
- Result columns are classified by the backend (`fieldName`, `modelId`, `isMeasure`, …). Cross-filtering depends on that metadata, so a backend must supply it even when its engine returns only column names.
